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
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/walkerkiller/coursive-plugins/internal/courseapi"
)

// The protocol was learned from the public UIS web client; this
// implementation is original.

const uisBase = "https://id.fudan.edu.cn"

var (
	errBadCredentials = errors.New("学号或密码错误")
	// errNeedsPassword means UIS asked for credentials and none were given.
	errNeedsPassword = errors.New("统一身份认证需要重新输入密码")
	ticketRe         = regexp.MustCompile(`locationValue\s*=\s*"([^"]*ticket=[^"]*)"`)
	lckRe            = regexp.MustCompile(`lck=([^&#"]+)`)
	entityRe         = regexp.MustCompile(`entityId=([^&#"]+)`)
)

// uis signs in with a username and password for one CAS service.
type uis struct {
	base string       // default uisBase; tests point it at a fake
	hc   *http.Client // must carry a cookie jar
	// vpn reaches UIS through WebVPN, for services behind the gateway; the
	// gateway session must exist already.
	vpn bool
	// entity is the service's entity id when the sign-in address lacks one.
	entity     string
	totpSecret string
}

func (u *uis) origin() string {
	if u.base != "" {
		return u.base
	}
	if u.vpn {
		b, _ := vpnURL(uisBase)
		return b
	}
	return uisBase
}

func (u *uis) do(req *http.Request) (*http.Response, error) {
	req.Header.Set("User-Agent", courseapi.UserAgent)
	req.Header.Set("Referer", u.origin()+"/ac/")
	if req.Method == http.MethodPost {
		if b, err := url.Parse(u.origin()); err == nil {
			req.Header.Set("Origin", b.Scheme+"://"+b.Host)
		}
	}
	return u.hc.Do(req)
}

func (u *uis) postJSON(ctx context.Context, path string, in, out any) error {
	b, _ := json.Marshal(in)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.origin()+path, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := u.do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("UIS %s: HTTP %d", path, resp.StatusCode)
	}
	return json.Unmarshal(raw, out)
}

// login runs the UIS chain for start, a URL that redirects to UIS (such as a
// service's CAS login page), and follows the ticket back to the service.
// Afterwards the jar holds the service's session cookies. While the UIS
// session lasts no password is needed; otherwise an empty password is
// errNeedsPassword.
func (u *uis) login(ctx context.Context, start, username, password string) error {
	// 1. The service redirects to UIS; the address carries lck and entityId.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, start, nil)
	if err != nil {
		return err
	}
	resp, err := u.do(req)
	if err != nil {
		return fmt.Errorf("无法打开统一身份认证：%w", err)
	}
	firstPage, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()
	if readErr != nil {
		return fmt.Errorf("无法读取统一身份认证的响应：%w", readErr)
	}
	final := resp.Request.URL.String()
	if !strings.HasPrefix(final, u.origin()) {
		return nil // still signed in: the service took us back directly
	}
	// A live UIS session returns the service ticket in an HTML script, with
	// no login form context; no password is needed.
	if ticket := ticketRe.FindSubmatch(firstPage); ticket != nil {
		return u.followTicket(ctx, html.UnescapeString(string(ticket[1])))
	}
	lck, entity := match(lckRe, final), match(entityRe, final)
	if entity == "" {
		entity = u.entity
	}
	if lck == "" || entity == "" {
		return errors.New("统一身份认证的登录页缺少 lck 或 entityId")
	}
	if password == "" {
		return errNeedsPassword
	}

	// 2. Which auth chain handles username and password.
	var methods struct {
		RequestType string `json:"requestType"`
		Data        []struct {
			ModuleCode    string `json:"moduleCode"`
			AuthChainCode string `json:"authChainCode"`
		} `json:"data"`
	}
	if err := u.postJSON(ctx, "/idp/authn/queryAuthMethods", map[string]string{"lck": lck, "entityId": entity}, &methods); err != nil {
		return err
	}
	chain := ""
	for _, m := range methods.Data {
		if m.ModuleCode == "userAndPwd" {
			chain = m.AuthChainCode
		}
	}
	if chain == "" {
		return errors.New("统一身份认证没有提供账号密码登录方式")
	}

	// 3. The password is encrypted with the page's RSA key.
	var key struct {
		Data string `json:"data"`
	}
	kreq, _ := http.NewRequestWithContext(ctx, http.MethodGet, u.origin()+"/idp/authn/getJsPublicKey", nil)
	kresp, err := u.do(kreq)
	if err != nil {
		return err
	}
	kraw, _ := io.ReadAll(io.LimitReader(kresp.Body, 64<<10))
	kresp.Body.Close()
	if err := json.Unmarshal(kraw, &key); err != nil {
		return fmt.Errorf("无法读取统一身份认证的公钥：%w", err)
	}
	enc, err := encrypt(key.Data, password)
	if err != nil {
		return err
	}

	// 4. Submit the credentials for a login token.
	var exec authResult
	if err := u.postJSON(ctx, "/idp/authn/authExecute", map[string]any{
		"authModuleCode": "userAndPwd", "authChainCode": chain, "entityId": entity, "requestType": methods.RequestType, "lck": lck,
		"authPara": map[string]string{"loginName": username, "password": enc, "verifyCode": ""},
	}, &exec); err != nil {
		return err
	}
	if fmt.Sprint(exec.Code) != "200" {
		if exec.Message != "" {
			return fmt.Errorf("%w：%s", errBadCredentials, exec.Message)
		}
		return errBadCredentials
	}

	if exec.Second {
		token, err := u.completeTOTP(ctx, exec, username, entity, methods.RequestType, lck, time.Now())
		if err != nil {
			return err
		}
		exec.LoginToken = token
	}
	if exec.LoginToken == "" {
		return errors.New("统一身份认证没有返回登录令牌")
	}

	// 5. Trade the token for the service ticket address.
	form := url.Values{"loginToken": {exec.LoginToken}}
	ereq, _ := http.NewRequestWithContext(ctx, http.MethodPost, u.origin()+"/idp/authCenter/authnEngine", strings.NewReader(form.Encode()))
	ereq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	eresp, err := u.do(ereq)
	if err != nil {
		return err
	}
	page, _ := io.ReadAll(io.LimitReader(eresp.Body, 1<<20))
	eresp.Body.Close()
	m := ticketRe.FindSubmatch(page)
	if m == nil {
		return errors.New("统一身份认证没有返回服务票据")
	}
	return u.followTicket(ctx, html.UnescapeString(string(m[1])))
}

// followTicket completes both password sign-in and silent SSO. Behind
// WebVPN, UIS may name the service's original address in the ticket page.
func (u *uis) followTicket(ctx context.Context, ticket string) error {
	var err error
	if u.vpn && !strings.HasPrefix(ticket, webVPNBase+"/") {
		if ticket, err = vpnURL(ticket); err != nil {
			return err
		}
	}
	treq, err := http.NewRequestWithContext(ctx, http.MethodGet, ticket, nil)
	if err != nil {
		return err
	}
	tresp, err := u.do(treq)
	if err != nil {
		return err
	}
	io.Copy(io.Discard, io.LimitReader(tresp.Body, 4<<20))
	tresp.Body.Close()
	if strings.HasPrefix(tresp.Request.URL.String(), u.origin()) {
		return errors.New("服务没有接受统一身份认证的票据")
	}
	return nil
}

func match(re *regexp.Regexp, s string) string {
	m := re.FindStringSubmatch(s)
	if m == nil {
		return ""
	}
	v, _ := url.QueryUnescape(m[1])
	return v
}

func encrypt(pubB64, password string) (string, error) {
	der, err := base64.StdEncoding.DecodeString(strings.TrimSpace(pubB64))
	if err != nil {
		return "", fmt.Errorf("无法读取统一身份认证的公钥：%w", err)
	}
	pub, err := x509.ParsePKIXPublicKey(der)
	if err != nil {
		return "", fmt.Errorf("无法读取统一身份认证的公钥：%w", err)
	}
	rk, ok := pub.(*rsa.PublicKey)
	if !ok {
		return "", errors.New("统一身份认证的公钥不是 RSA")
	}
	ct, err := rsa.EncryptPKCS1v15(rand.Reader, rk, []byte(password))
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(ct), nil
}
