package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/walkerkiller/coursive-plugins/internal/courseapi"
	"golang.org/x/net/html"
)

const (
	platformBase = "https://look.tongji.edu.cn"
	iamBase      = "https://iam.tongji.edu.cn"
	canvasBase   = "https://canvas.tongji.edu.cn"
	tenantCode   = "222"

	// Public key from the identity provider's crypt.js.
	iamRSAPublicKey      = "MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQC9t16RqQWUE/J1IyOfoNHc4r/h6RPnXcWTJ4IbhQVUsEqMMm65F0hiytAgozXmVw68yPJywbpblDrx9zl1wdRcdHCoUvmPdr9/oCQtpQyVc7BXZIN6wJlD6MTeMeni+N0toNPxfXjiAawjNHGZZuT8wQpNEMwsVyJ/lonXaVdGZwIDAQAB"
	checkcodePlaceholder = "请输入验证码"
)

var (
	errBadCredentials  = errors.New("学号或密码错误")
	errCaptchaRequired = errors.New("需要验证码")
	errSSOExpired      = errors.New("统一身份认证会话已过期")
	jwtInCookie        = regexp.MustCompile(`s:\d+:"(eyJ[^"]+)"`)
	spChainCode        = regexp.MustCompile(`#spAuthChainCode1"\)\.val\('([0-9a-fA-F]+)'\)`)
)

// loginError carries the identity provider's message for display.
type loginError struct {
	kind error
	msg  string
}

func (e *loginError) Error() string {
	if e.msg != "" {
		return e.kind.Error() + "：" + e.msg
	}
	return e.kind.Error()
}
func (e *loginError) Unwrap() error { return e.kind }

// loginForm is the parsed username and password form.
type loginForm struct {
	action string
	fields url.Values
	order  []string
	lcKey  string
}

func ssoStartURL() string {
	return platformBase + "/casapi/index.php?r=auth/login&auType=&tenant_code=" + tenantCode +
		"&forward=" + url.QueryEscape(platformBase+"/validate")
}

func encryptPassword(pw string) (string, error) {
	der, err := base64.StdEncoding.DecodeString(iamRSAPublicKey)
	if err != nil {
		return "", err
	}
	pub, err := x509.ParsePKIXPublicKey(der)
	if err != nil {
		return "", err
	}
	ct, err := rsa.EncryptPKCS1v15(rand.Reader, pub.(*rsa.PublicKey), []byte(pw))
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(ct), nil
}

func newRequest(ctx context.Context, method, u string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", courseapi.UserAgent)
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	return req, nil
}

// startSSO walks the redirect chain from the platform to the identity
// provider. If the provider's session in the jar is still valid, the chain
// ends back on the platform with a fresh _token cookie and the form is nil.
func startSSO(ctx context.Context, hc *http.Client) (*loginForm, string, error) {
	req, err := newRequest(ctx, http.MethodGet, ssoStartURL(), nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("无法打开登录页：%w", err)
	}
	defer resp.Body.Close()
	iam, _ := url.Parse(iamBase)
	if resp.Request.URL.Hostname() != iam.Hostname() {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<20))
		if tok := tokenFromJar(hc); tok != "" {
			return nil, tok, nil
		}
		return nil, "", fmt.Errorf("无法登录：单点登录停在 %s，没有返回令牌", resp.Request.URL.Hostname())
	}
	page, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, "", err
	}
	doc, err := html.Parse(bytes.NewReader(page))
	if err != nil {
		return nil, "", err
	}
	f := findForm(doc, "authen1Form")
	if f == nil {
		return nil, "", errors.New("无法登录：没有找到统一身份认证的登录表单")
	}
	// The page script fills the auth-chain code after load.
	if m := spChainCode.FindSubmatch(page); m != nil {
		f.fields.Set("spAuthChainCode", string(m[1]))
	}
	return f, "", nil
}

// login signs in with a password and returns the platform JWT.
func login(ctx context.Context, hc *http.Client, username, password, captcha string) (string, error) {
	clearPlatformToken(hc)
	form, tok, err := startSSO(ctx, hc)
	if err != nil {
		return "", err
	}
	if tok != "" {
		return tok, nil
	}
	if captcha == "" {
		if need, err := captchaNeeded(ctx, hc, username); err == nil && need {
			return "", &loginError{kind: errCaptchaRequired}
		}
	}
	enc, err := encryptPassword(password)
	if err != nil {
		return "", err
	}
	form.fields.Set("j_username", username)
	form.fields.Set("j_password", enc)
	if captcha != "" {
		form.fields.Set("j_checkcode", captcha)
	} else {
		form.fields.Set("j_checkcode", checkcodePlaceholder)
	}
	body := form.encode()

	chain := iamBase + "/idp/authcenter/ActionAuthChain?authnLcKey=" + url.QueryEscape(form.lcKey)
	req, _ := newRequest(ctx, http.MethodPost, chain, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	resp, err := hc.Do(req)
	if err != nil {
		return "", fmt.Errorf("无法提交登录：%w", err)
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()
	var chainResp map[string]any
	if err := json.Unmarshal(raw, &chainResp); err == nil {
		if fmt.Sprint(chainResp["loginFailed"]) == "true" {
			msg, _ := chainResp["authnErrorTip"].(string)
			if strings.Contains(msg, "验证码") {
				return "", &loginError{kind: errCaptchaRequired, msg: msg}
			}
			return "", &loginError{kind: errBadCredentials, msg: msg}
		}
	}

	action := form.action
	if strings.HasPrefix(action, "/") {
		action = iamBase + action
	}
	req, _ = newRequest(ctx, http.MethodPost, action, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", chain)
	resp, err = hc.Do(req)
	if err != nil {
		return "", fmt.Errorf("无法完成单点登录跳转：%w", err)
	}
	io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<20))
	resp.Body.Close()
	if tok := tokenFromJar(hc); tok != "" {
		return tok, nil
	}
	return "", &loginError{kind: errBadCredentials, msg: "登录流程结束但没有获得令牌"}
}

// silentRenew reuses the single sign-on cookies to mint a new JWT without
// the password; errSSOExpired means the identity provider wants it again.
func silentRenew(ctx context.Context, hc *http.Client) (string, error) {
	clearPlatformToken(hc)
	form, tok, err := startSSO(ctx, hc)
	if err != nil {
		return "", err
	}
	if form != nil {
		return "", errSSOExpired
	}
	return tok, nil
}

func captchaNeeded(ctx context.Context, hc *http.Client, username string) (bool, error) {
	v := url.Values{"j_username": {username}, "j_authMethodID": {"1"}, "spAuthChainCode": {""}}
	req, _ := newRequest(ctx, http.MethodPost, iamBase+"/idp/displayVerificationCode.do", strings.NewReader(v.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	resp, err := hc.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 64))
	return strings.TrimSpace(string(b)) == "true", nil
}

// captchaImage fetches the captcha bound to the jar's session as a data URL.
func captchaImage(ctx context.Context, hc *http.Client) (string, error) {
	if _, _, err := startSSO(ctx, hc); err != nil {
		return "", err
	}
	u := fmt.Sprintf("%s/idp/Kaptcha.jpg?date=%d", iamBase, time.Now().UnixMilli())
	req, _ := newRequest(ctx, http.MethodGet, u, nil)
	resp, err := hc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 512<<10))
	if err != nil {
		return "", err
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "image/") {
		ct = "image/jpeg"
	}
	return "data:" + ct + ";base64," + base64.StdEncoding.EncodeToString(b), nil
}

func platformURL() *url.URL { u, _ := url.Parse(platformBase + "/"); return u }

func tokenFromJar(hc *http.Client) string {
	for _, c := range hc.Jar.Cookies(platformURL()) {
		if c.Name != "_token" {
			continue
		}
		v := c.Value
		if dec, err := url.QueryUnescape(v); err == nil {
			v = dec
		}
		if m := jwtInCookie.FindStringSubmatch(v); m != nil {
			return m[1]
		}
		if strings.HasPrefix(v, "eyJ") {
			return v
		}
	}
	return ""
}

func clearPlatformToken(hc *http.Client) {
	hc.Jar.SetCookies(platformURL(), []*http.Cookie{{Name: "_token", Value: "", Path: "/", MaxAge: -1}})
}

// jwtExpiry reads exp from a JWT payload without verifying it; the server does.
func jwtExpiry(tok string) time.Time {
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		return time.Time{}
	}
	b, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		return time.Time{}
	}
	var c struct {
		Exp int64 `json:"exp"`
	}
	if json.Unmarshal(b, &c) != nil || c.Exp == 0 {
		return time.Time{}
	}
	return time.Unix(c.Exp, 0)
}

func (f *loginForm) encode() string {
	var b strings.Builder
	for i, k := range f.order {
		if i > 0 {
			b.WriteByte('&')
		}
		b.WriteString(url.QueryEscape(k))
		b.WriteByte('=')
		b.WriteString(url.QueryEscape(f.fields.Get(k)))
	}
	return b.String()
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func findForm(n *html.Node, id string) *loginForm {
	if n.Type == html.ElementNode && n.Data == "form" && attr(n, "id") == id {
		f := &loginForm{action: attr(n, "action"), fields: url.Values{}}
		var walk func(*html.Node)
		walk = func(c *html.Node) {
			if c.Type == html.ElementNode && c.Data == "input" {
				if name := attr(c, "name"); name != "" {
					if _, seen := f.fields[name]; !seen {
						f.order = append(f.order, name)
						f.fields.Set(name, attr(c, "value"))
					}
				}
			}
			for k := c.FirstChild; k != nil; k = k.NextSibling {
				walk(k)
			}
		}
		walk(n)
		f.lcKey = f.fields.Get("authnLcKey")
		return f
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if f := findForm(c, id); f != nil {
			return f
		}
	}
	return nil
}
