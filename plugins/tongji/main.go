// Command provider is the Tongji University source: recorded lectures from
// the university's lecture platform and coursework from its Canvas, both
// signed in through the university's identity provider.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/walkerkiller/coursive-plugins/internal/canvaslms"
	"github.com/walkerkiller/coursive-plugins/internal/courseapi"
	"github.com/walkerkiller/coursive-plugins/internal/school"
	"github.com/walkerkiller/coursive-plugins/sdk/provider"
)

func main() {
	if err := provider.Serve(func(h *provider.Host) provider.Handler {
		return school.Handler(newSource(h))
	}); err != nil {
		log.Fatal(err)
	}
}

type challenge struct {
	id, username, password, image string
}

type source struct {
	s      *school.Session
	plat   *courseapi.Client
	canvas *canvaslms.Client

	mu       sync.Mutex
	token    string
	exp      time.Time
	expired  bool
	pending  *challenge
	renewing chan struct{}
	renewErr error
}

func newSource(h *provider.Host) *source {
	src := &source{s: school.NewSession(h)}
	src.plat = &courseapi.Client{Base: platformBase, Tenant: tenantCode, Do: src.do,
		Address: func(raw string) provider.Address {
			return provider.Address{URL: raw, Headers: map[string]string{"User-Agent": courseapi.UserAgent, "Referer": platformBase + "/"}}
		}}
	src.canvas = &canvaslms.Client{Base: canvasBase, HC: src.s.HC}
	return src
}

func (src *source) Platform() *courseapi.Client { return src.plat }
func (src *source) Canvas() *canvaslms.Client   { return src.canvas }

func (src *source) Status(ctx context.Context) provider.Auth {
	saved := src.s.Load(ctx)
	src.mu.Lock()
	defer src.mu.Unlock()
	if c := src.pending; c != nil {
		return provider.Auth{State: provider.NeedsInput, Challenge: &provider.Challenge{ID: c.id, Kind: "image", Prompt: "输入图中的验证码", Image: c.image}}
	}
	if src.token == "" && saved.Token != "" && !src.expired {
		src.token, src.exp = saved.Token, jwtExpiry(saved.Token)
	}
	display := saved.Display
	if display == "" {
		display = saved.Username
	}
	switch {
	case src.token != "" && !src.expired:
		return provider.Auth{State: provider.SignedIn, Account: &provider.Account{Display: display}}
	case saved.Username != "":
		return provider.Auth{State: provider.Expired, Account: &provider.Account{Display: display}}
	}
	return provider.Auth{State: provider.SignedOut}
}

func (src *source) SignIn(ctx context.Context, username, password string) (provider.Auth, error) {
	src.s.Jar.Reset()
	src.plat.Reset()
	return src.finish(ctx, username, password, "")
}

func (src *source) Answer(ctx context.Context, id, answer string) (provider.Auth, error) {
	src.mu.Lock()
	c := src.pending
	src.mu.Unlock()
	if c == nil || c.id != id {
		return provider.Auth{}, provider.Errorf(provider.KindInvalid, "无法提交验证码：验证码已过期，请重新登录")
	}
	return src.finish(ctx, c.username, c.password, answer)
}

// finish runs the password sign-in; a captcha turns into a challenge.
func (src *source) finish(ctx context.Context, username, password, captcha string) (provider.Auth, error) {
	tok, err := login(ctx, src.s.HC, username, password, captcha)
	if errors.Is(err, errCaptchaRequired) {
		img, ierr := captchaImage(ctx, src.s.HC)
		if ierr != nil {
			return provider.Auth{}, provider.Errorf(provider.KindUnavailable, "无法获取验证码：%v", ierr)
		}
		c := &challenge{id: newID(), username: username, password: password, image: img}
		src.mu.Lock()
		src.pending = c
		src.mu.Unlock()
		prompt := "输入图中的验证码"
		if captcha != "" {
			prompt = "验证码不正确，请重新输入"
		}
		return provider.Auth{State: provider.NeedsInput, Challenge: &provider.Challenge{ID: c.id, Kind: "image", Prompt: prompt, Image: img}}, nil
	}
	src.mu.Lock()
	src.pending = nil
	src.mu.Unlock()
	if errors.Is(err, errBadCredentials) {
		return provider.Auth{}, provider.Errorf(provider.KindInvalid, "无法登录：%v", err)
	}
	if err != nil {
		return provider.Auth{}, provider.Errorf(provider.KindUnavailable, "无法登录：%v", err)
	}
	src.adopt(tok)
	display := username
	if u, err := src.plat.UserInfo(ctx); err == nil && u.Realname != "" {
		display = u.Realname
	}
	if err := src.s.Update(ctx, func(s *school.Saved) {
		*s = school.Saved{Username: username, Password: password, Display: display, Token: tok}
	}); err != nil {
		return provider.Auth{}, err
	}
	return provider.Auth{State: provider.SignedIn, Account: &provider.Account{Display: display}}, nil
}

func (src *source) adopt(tok string) {
	src.mu.Lock()
	src.token, src.exp, src.expired = tok, jwtExpiry(tok), false
	src.mu.Unlock()
}

func (src *source) SignOut(ctx context.Context) error {
	src.mu.Lock()
	src.token, src.exp, src.expired, src.pending = "", time.Time{}, false, nil
	src.mu.Unlock()
	return src.s.Clear(ctx)
}

// currentToken returns a valid JWT, renewing it when it is close to expiry.
func (src *source) currentToken(ctx context.Context) (string, error) {
	src.Status(ctx)
	src.mu.Lock()
	tok, exp := src.token, src.exp
	src.mu.Unlock()
	if tok == "" {
		return "", school.AuthRequired("无法读取课程：请先在「来源」中登录")
	}
	if exp.IsZero() || time.Until(exp) > 2*time.Minute {
		return tok, nil
	}
	if err := src.renew(ctx); err != nil {
		return "", err
	}
	src.mu.Lock()
	defer src.mu.Unlock()
	return src.token, nil
}

// renew mints a new JWT through single sign-on, then with the saved
// password; concurrent callers share one renewal.
func (src *source) renew(ctx context.Context) error {
	src.mu.Lock()
	if ch := src.renewing; ch != nil {
		src.mu.Unlock()
		select {
		case <-ch:
		case <-ctx.Done():
			return ctx.Err()
		}
		src.mu.Lock()
		defer src.mu.Unlock()
		return src.renewErr
	}
	ch := make(chan struct{})
	src.renewing = ch
	src.mu.Unlock()

	saved := src.s.Load(ctx)
	tok, err := silentRenew(ctx, src.s.HC)
	if err != nil && saved.Username != "" && saved.Password != "" {
		tok, err = login(ctx, src.s.HC, saved.Username, saved.Password, "")
	}
	if err == nil {
		src.adopt(tok)
		src.s.Update(ctx, func(s *school.Saved) { s.Token = tok })
	} else {
		src.mu.Lock()
		src.expired = true
		src.mu.Unlock()
		src.s.Host.Notify()
		err = school.AuthRequired(fmt.Sprintf("无法续期登录：%v。请在「来源」中重新登录", err))
	}
	src.mu.Lock()
	src.renewing, src.renewErr = nil, err
	src.mu.Unlock()
	close(ch)
	return err
}

// do sends a platform API request with the JWT, renewing once on 401 or 403.
func (src *source) do(ctx context.Context, req *http.Request) (*http.Response, error) {
	for attempt := 0; ; attempt++ {
		tok, err := src.currentToken(ctx)
		if err != nil {
			return nil, err
		}
		r := req.Clone(ctx)
		r.Header.Set("Authorization", "Bearer "+tok)
		resp, err := src.s.API.Do(r)
		if err != nil {
			return nil, fmt.Errorf("无法连接课程平台：%w", err)
		}
		if (resp.StatusCode != http.StatusUnauthorized && resp.StatusCode != http.StatusForbidden) || attempt == 1 {
			return resp, nil
		}
		resp.Body.Close()
		if err := src.renew(ctx); err != nil {
			return nil, err
		}
	}
}

// CanvasSignIn opens Canvas through OpenID Connect with the identity
// provider's cookies, signing in to the provider again when they expired.
func (src *source) CanvasSignIn(ctx context.Context) error {
	err := canvaslms.OIDCSignIn(ctx, src.s.HC, canvasBase)
	if errors.Is(err, canvaslms.ErrIdP) {
		saved := src.s.Load(ctx)
		if saved.Username == "" || saved.Password == "" {
			return school.AuthRequired("无法打开 Canvas：请先在「来源」中登录")
		}
		tok, lerr := login(ctx, src.s.HC, saved.Username, saved.Password, "")
		if lerr != nil {
			return school.AuthRequired(fmt.Sprintf("无法打开 Canvas：%v。请在「来源」中重新登录", lerr))
		}
		src.adopt(tok)
		err = canvaslms.OIDCSignIn(ctx, src.s.HC, canvasBase)
	}
	if err != nil {
		return provider.Errorf(provider.KindUnavailable, "无法打开 Canvas：%v", err)
	}
	return src.s.Update(ctx, func(*school.Saved) {})
}

func newID() string {
	var b [8]byte
	rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
