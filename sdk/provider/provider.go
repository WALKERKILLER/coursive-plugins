// Package provider runs a Coursive source provider: JSON-RPC 2.0 over stdin
// and stdout, one message per line, as the source-provider contract defines.
// Requests from the daemon are served concurrently; the provider reaches the
// daemon's vault through Host.
package provider

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
)

// Protocol is the provider protocol this SDK speaks.
const Protocol = "1"

// MaxMessage is the largest protocol message, in bytes, either way.
const MaxMessage = 1 << 20

// Error kinds the daemon understands.
const (
	KindAuthRequired = "auth_required"
	KindRateLimited  = "rate_limited"
	KindUnavailable  = "unavailable"
	KindInvalid      = "invalid"
)

// Error is a failure reported to the daemon. Message is shown to the user.
type Error struct {
	Kind          string
	Message       string
	RetryAfterSec int
}

func (e *Error) Error() string { return e.Message }

// Errorf builds an Error of the given kind.
func Errorf(kind, format string, args ...any) *Error {
	return &Error{Kind: kind, Message: fmt.Sprintf(format, args...)}
}

// Handler serves one daemon request. A returned *Error keeps its kind; any
// other error is reported as unavailable.
type Handler func(ctx context.Context, method string, params json.RawMessage) (any, error)

// ErrMethodNotFound is returned by a Handler for a method it does not serve.
var ErrMethodNotFound = errors.New("method not found")

type message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int64          `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    *struct {
		Kind          string `json:"kind"`
		RetryAfterSec int    `json:"retryAfterSec,omitempty"`
	} `json:"data,omitempty"`
}

// Host is the daemon side of the session.
type Host struct {
	w   io.Writer
	wmu sync.Mutex

	mu      sync.Mutex
	next    int64
	pending map[int64]chan message
	closed  chan struct{}
}

func (h *Host) write(m message) error {
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	if len(b) > MaxMessage {
		return Errorf(KindInvalid, "回应超过 1 MiB")
	}
	h.wmu.Lock()
	defer h.wmu.Unlock()
	_, err = h.w.Write(append(b, '\n'))
	return err
}

// call sends a request to the daemon and decodes its result into out.
func (h *Host) call(ctx context.Context, method string, params, out any) error {
	raw, err := json.Marshal(params)
	if err != nil {
		return err
	}
	ch := make(chan message, 1)
	h.mu.Lock()
	h.next++
	id := h.next
	h.pending[id] = ch
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		delete(h.pending, id)
		h.mu.Unlock()
	}()
	if err := h.write(message{JSONRPC: "2.0", ID: &id, Method: method, Params: raw}); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-h.closed:
		return errors.New("daemon closed the session")
	case m := <-ch:
		if m.Error != nil {
			return fmt.Errorf("%s: %s", method, m.Error.Message)
		}
		if out == nil || len(m.Result) == 0 {
			return nil
		}
		return json.Unmarshal(m.Result, out)
	}
}

// VaultGet reads a secret the provider stored earlier.
func (h *Host) VaultGet(ctx context.Context, key string) (string, bool, error) {
	var out struct {
		Value *string `json:"value"`
	}
	if err := h.call(ctx, "vault.get", map[string]string{"key": key}, &out); err != nil {
		return "", false, err
	}
	if out.Value == nil {
		return "", false, nil
	}
	return *out.Value, true, nil
}

// VaultPut stores a secret in the daemon's vault.
func (h *Host) VaultPut(ctx context.Context, key, value string) error {
	return h.call(ctx, "vault.put", map[string]string{"key": key, "value": value}, nil)
}

// VaultDelete removes a secret.
func (h *Host) VaultDelete(ctx context.Context, key string) error {
	return h.call(ctx, "vault.delete", map[string]string{"key": key}, nil)
}

// Notify tells the daemon the sign-in state changed; it asks auth.status again.
func (h *Host) Notify() {
	h.write(message{JSONRPC: "2.0", Method: "notify", Params: json.RawMessage(`{}`)})
}

// Serve runs the session on stdin and stdout until the daemon closes it.
func Serve(newHandler func(*Host) Handler) error {
	return serve(os.Stdin, os.Stdout, newHandler)
}

func serve(r io.Reader, w io.Writer, newHandler func(*Host) Handler) error {
	h := &Host{w: w, pending: map[int64]chan message{}, closed: make(chan struct{})}
	defer close(h.closed)
	handle := newHandler(h)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), MaxMessage)
	for sc.Scan() {
		var m message
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil || m.JSONRPC != "2.0" {
			continue
		}
		switch {
		case m.Method != "":
			go h.serveOne(ctx, handle, m)
		case m.ID != nil:
			h.mu.Lock()
			ch := h.pending[*m.ID]
			h.mu.Unlock()
			if ch != nil {
				ch <- m
			}
		}
	}
	return sc.Err()
}

func (h *Host) serveOne(ctx context.Context, handle Handler, m message) {
	result, err := handle(ctx, m.Method, m.Params)
	if m.ID == nil {
		return
	}
	out := message{JSONRPC: "2.0", ID: m.ID}
	if err != nil {
		var pe *Error
		if !errors.As(err, &pe) {
			pe = &Error{Kind: KindUnavailable, Message: err.Error()}
			if errors.Is(err, ErrMethodNotFound) {
				pe.Kind = KindInvalid
			}
		}
		out.Error = &rpcError{Code: -32000, Message: pe.Message}
		out.Error.Data = &struct {
			Kind          string `json:"kind"`
			RetryAfterSec int    `json:"retryAfterSec,omitempty"`
		}{pe.Kind, pe.RetryAfterSec}
	} else {
		if result == nil {
			result = struct{}{}
		}
		out.Result, _ = json.Marshal(result)
	}
	if h.write(out) != nil && out.Error == nil {
		// An oversized result becomes an error the daemon can show.
		h.write(message{JSONRPC: "2.0", ID: m.ID, Error: &rpcError{Code: -32000, Message: "无法回应：结果超过 1 MiB"}})
	}
}

// Decode unmarshals request params, reporting malformed ones as invalid.
func Decode(params json.RawMessage, v any) error {
	if len(params) == 0 {
		return nil
	}
	if err := json.Unmarshal(params, v); err != nil {
		return Errorf(KindInvalid, "参数无效：%v", err)
	}
	return nil
}
