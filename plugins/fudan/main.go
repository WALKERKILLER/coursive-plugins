// Command provider is the Fudan University source: recorded lectures from
// iCourse, reached through the university's WebVPN, and coursework from its
// Canvas, both signed in through UIS.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sync"

	"github.com/walkerkiller/coursive-plugins/internal/canvaslms"
	"github.com/walkerkiller/coursive-plugins/internal/courseapi"
	"github.com/walkerkiller/coursive-plugins/internal/school"
	"github.com/walkerkiller/coursive-plugins/sdk/provider"
)

const (
	icourseBase = "https://icourse.fudan.edu.cn"
	canvasBase  = "https://elearning.fudan.edu.cn"
	tenantCode  = "222"
)

func main() {
	if err := provider.Serve(func(h *provider.Host) provider.Handler {
		return school.Handler(newSource(h))
	}); err != nil {
		log.Fatal(err)
	}
}

type source struct {
	s      *school.Session
	plat   *courseapi.Client
	canvas *canvaslms.Client

	mu       sync.Mutex
	expired  bool
	renewing chan struct{}
	renewErr error
}

func newSource(h *provider.Host) *source {
	src := &source{s: school.NewSession(h)}
	src.plat = &courseapi.Client{Base: icourseBase, Tenant: tenantCode, Do: src.do, Address: src.address}
	src.canvas = &canvaslms.Client{Base: canvasBase, HC: src.s.HC}
	return src
}

func (src *source) Platform() *courseapi.Client { return src.plat }
func (src *source) Canvas() *canvaslms.Client   { return src.canvas }

// address sends the daemon through WebVPN with the gateway's session cookie,
// which carries iCourse's own.
func (src *source) address(raw string) provider.Address {
	u, err := vpnURL(raw)
	if err != nil {
		u = raw
	}
	referer, _ := vpnURL(icourseBase + "/")
	h := map[string]string{"User-Agent": courseapi.UserAgent, "Referer": referer}
	if c := src.s.Jar.Header(webVPNBase + "/"); c != "" {
		h["Cookie"] = c
	}
	return provider.Address{URL: u, Headers: h}
}

func (src *source) Status(ctx context.Context) provider.Auth {
	saved := src.s.Load(ctx)
	src.mu.Lock()
	expired := src.expired
	src.mu.Unlock()
	display := saved.Display
	if display == "" {
		display = saved.Username
	}
	switch {
	case saved.Username != "" && saved.Cookies != "" && !expired:
		return provider.Auth{State: provider.SignedIn, Account: &provider.Account{Display: display}}
	case saved.Username != "":
		return provider.Auth{State: provider.Expired, Account: &provider.Account{Display: display}}
	}
	return provider.Auth{State: provider.SignedOut}
}

func (src *source) SignIn(ctx context.Context, username, password string) (provider.Auth, error) {
	src.s.Jar.Reset()
	src.plat.Reset()
	if err := src.signInICourse(ctx, username, password); err != nil {
		if errors.Is(err, errBadCredentials) {
			return provider.Auth{}, provider.Errorf(provider.KindInvalid, "无法登录：%v", err)
		}
		return provider.Auth{}, provider.Errorf(provider.KindUnavailable, "无法登录：%v", err)
	}
	src.mu.Lock()
	src.expired = false
	src.mu.Unlock()
	display := username
	if u, err := src.plat.UserInfo(ctx); err == nil && u.Realname != "" {
		display = u.Realname
	}
	if err := src.s.Update(ctx, func(s *school.Saved) {
		*s = school.Saved{Username: username, Password: password, Display: display}
	}); err != nil {
		return provider.Auth{}, err
	}
	return provider.Auth{State: provider.SignedIn, Account: &provider.Account{Display: display}}, nil
}

func (src *source) Answer(context.Context, string, string) (provider.Auth, error) {
	return provider.Auth{}, provider.Errorf(provider.KindInvalid, "无法提交：这个来源不需要验证码")
}

func (src *source) SignOut(ctx context.Context) error {
	src.mu.Lock()
	src.expired = false
	src.mu.Unlock()
	return src.s.Clear(ctx)
}

// signInICourse signs in to WebVPN and then, through the gateway, to
// iCourse, and checks that iCourse answers as the signed-in user. With an
// empty password it succeeds only while the UIS session lasts.
func (src *source) signInICourse(ctx context.Context, username, password string) error {
	hc := src.s.HC
	if err := (&uis{hc: hc}).login(ctx, webVPNBase+"/login?cas_login=true", username, password); err != nil {
		return err
	}
	start, err := vpnURL(icourseBase + "/casapi/index.php?r=auth/login&school_login=1&tenant_code=" + tenantCode + "&forward=" + url.QueryEscape(icourseBase+"/"))
	if err != nil {
		return err
	}
	if err := (&uis{hc: hc, vpn: true, entity: icourseBase}).login(ctx, start, username, password); err != nil {
		return err
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, icourseBase+"/userapi/v1/infosimple", nil)
	resp, err := src.viaVPN(ctx, req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var r struct {
		Code any `json:"code"`
	}
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if vpnSignedOut(resp) || json.Unmarshal(b, &r) != nil || (fmt.Sprint(r.Code) != "0" && fmt.Sprint(r.Code) != "200") {
		return errors.New("iCourse 没有接受这次登录，请稍后重试")
	}
	return nil
}

// renew signs in again: the empty password first, which works while the
// UIS session lasts, then the saved one. Concurrent callers share a renewal.
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
	err := src.signInICourse(ctx, saved.Username, "")
	if errors.Is(err, errNeedsPassword) && saved.Password != "" {
		err = src.signInICourse(ctx, saved.Username, saved.Password)
	}
	if err == nil {
		src.s.Update(ctx, func(*school.Saved) {})
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

// do sends an iCourse request through WebVPN, signing in again once when the
// gateway sends it to its sign-in.
func (src *source) do(ctx context.Context, req *http.Request) (*http.Response, error) {
	if src.Status(ctx).State == provider.SignedOut {
		return nil, school.AuthRequired("无法读取课程：请先在「来源」中登录")
	}
	for attempt := 0; ; attempt++ {
		resp, err := src.viaVPN(ctx, req)
		if err != nil || !vpnSignedOut(resp) {
			return resp, err
		}
		resp.Body.Close()
		if attempt == 1 {
			return nil, school.AuthRequired("无法读取课程：WebVPN 会话无效，请在「来源」中重新登录")
		}
		if err := src.renew(ctx); err != nil {
			return nil, err
		}
	}
}

func (src *source) viaVPN(ctx context.Context, req *http.Request) (*http.Response, error) {
	u, err := vpnURL(req.URL.String())
	if err != nil {
		return nil, err
	}
	r := req.Clone(ctx)
	if r.URL, err = url.Parse(u); err != nil {
		return nil, err
	}
	r.Host = ""
	resp, err := src.s.HC.Do(r)
	if err != nil {
		return nil, fmt.Errorf("无法连接 WebVPN：%w", err)
	}
	return resp, nil
}

// CanvasSignIn opens Canvas's CAS login through UIS, silently while the UIS
// session lasts and otherwise with the saved password.
func (src *source) CanvasSignIn(ctx context.Context) error {
	saved := src.s.Load(ctx)
	if saved.Username == "" {
		return school.AuthRequired("无法打开 Canvas：请先在「来源」中登录")
	}
	u := &uis{hc: src.s.HC}
	err := u.login(ctx, canvasBase+"/login/cas", saved.Username, "")
	if errors.Is(err, errNeedsPassword) && saved.Password != "" {
		err = u.login(ctx, canvasBase+"/login/cas", saved.Username, saved.Password)
	}
	if errors.Is(err, errNeedsPassword) || errors.Is(err, errBadCredentials) {
		return school.AuthRequired(fmt.Sprintf("无法打开 Canvas：%v。请在「来源」中重新登录", err))
	}
	if err != nil {
		return provider.Errorf(provider.KindUnavailable, "无法打开 Canvas：%v", err)
	}
	return src.s.Update(ctx, func(*school.Saved) {})
}
