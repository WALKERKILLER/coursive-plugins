package provider

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"
	"time"
)

// TestServe plays the daemon: it calls the provider, answers its vault
// request and checks results and error kinds.
func TestServe(t *testing.T) {
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	done := make(chan error, 1)
	go func() {
		done <- serve(inR, outW, func(h *Host) Handler {
			return func(ctx context.Context, method string, params json.RawMessage) (any, error) {
				switch method {
				case "echo":
					v, ok, err := h.VaultGet(ctx, "k")
					return map[string]any{"value": v, "ok": ok}, err
				case "fail":
					return nil, Errorf(KindAuthRequired, "sign in")
				case "plain":
					return nil, errors.New("down")
				case "empty":
					return nil, nil
				}
				return nil, ErrMethodNotFound
			}
		})
		outW.Close()
	}()
	enc := json.NewEncoder(inW)
	sc := bufio.NewScanner(outR)
	read := func() map[string]any {
		t.Helper()
		if !sc.Scan() {
			t.Fatal("provider closed")
		}
		var m map[string]any
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			t.Fatal(err)
		}
		return m
	}

	enc.Encode(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "echo"})
	req := read()
	if req["method"] != "vault.get" {
		t.Fatalf("want vault.get, got %v", req)
	}
	enc.Encode(map[string]any{"jsonrpc": "2.0", "id": req["id"], "result": map[string]any{"value": "v"}})
	if res := read(); res["id"] != 1.0 || res["result"].(map[string]any)["value"] != "v" {
		t.Fatalf("echo = %v", res)
	}

	for method, kind := range map[string]string{"fail": KindAuthRequired, "plain": KindUnavailable, "nope": KindInvalid} {
		enc.Encode(map[string]any{"jsonrpc": "2.0", "id": 2, "method": method})
		res := read()
		e, _ := res["error"].(map[string]any)
		if e == nil || e["code"] != -32000.0 || e["data"].(map[string]any)["kind"] != kind {
			t.Fatalf("%s = %v", method, res)
		}
	}

	enc.Encode(map[string]any{"jsonrpc": "2.0", "id": 3, "method": "empty"})
	if res := read(); res["result"] == nil {
		t.Fatalf("empty = %v", res)
	}

	inW.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("serve did not return after stdin closed")
	}
}
