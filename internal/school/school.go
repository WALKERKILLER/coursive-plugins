// Package school serves the provider protocol for an institution whose
// lectures are on the shared course platform and whose coursework is on
// Canvas. Each institution's plugin supplies only its sign-in.
package school

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/walkerkiller/coursive-plugins/internal/canvaslms"
	"github.com/walkerkiller/coursive-plugins/internal/courseapi"
	"github.com/walkerkiller/coursive-plugins/internal/jar"
	"github.com/walkerkiller/coursive-plugins/sdk/provider"
)

// Saved is the session kept in the daemon's vault.
type Saved struct {
	Username   string `json:"username,omitempty"`
	Password   string `json:"password,omitempty"`
	TOTPSecret string `json:"totp_secret,omitempty"`
	Display    string `json:"display,omitempty"`
	Token      string `json:"token,omitempty"`
	Cookies    string `json:"cookies,omitempty"`
}

const vaultKey = "session"

// Session is what every institution's sign-in shares: the cookie jar, the
// HTTP clients and the vault-backed saved state.
type Session struct {
	Host *provider.Host
	Jar  *jar.Jar
	// HC follows redirects with the jar; API uses no jar.
	HC, API *http.Client

	mu     sync.Mutex
	loaded bool
	saved  Saved
}

// NewSession builds the clients for one provider process.
func NewSession(h *provider.Host) *Session {
	tr := &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		DialContext:         (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:   true,
		MaxIdleConnsPerHost: 16,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}
	j := jar.New()
	return &Session{Host: h, Jar: j,
		HC:  &http.Client{Jar: j, Timeout: 40 * time.Second, Transport: tr},
		API: &http.Client{Timeout: 30 * time.Second, Transport: tr}}
}

// Load reads the saved session once.
func (s *Session) Load(ctx context.Context) Saved {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.loaded {
		if v, ok, err := s.Host.VaultGet(ctx, vaultKey); err == nil {
			s.loaded = true
			if ok && json.Unmarshal([]byte(v), &s.saved) == nil {
				s.Jar.Restore(s.saved.Cookies)
			}
		}
	}
	return s.saved
}

// Update changes the saved session and writes it with the jar's cookies.
func (s *Session) Update(ctx context.Context, f func(*Saved)) error {
	s.mu.Lock()
	f(&s.saved)
	s.saved.Cookies = s.Jar.Marshal()
	b, _ := json.Marshal(s.saved)
	s.mu.Unlock()
	return s.Host.VaultPut(ctx, vaultKey, string(b))
}

// Clear forgets the session.
func (s *Session) Clear(ctx context.Context) error {
	s.mu.Lock()
	s.saved = Saved{}
	s.Jar.Reset()
	s.mu.Unlock()
	return s.Host.VaultDelete(ctx, vaultKey)
}

// Source is one institution's sign-in.
type Source interface {
	// Status reports the sign-in state without network requests.
	Status(ctx context.Context) provider.Auth
	SignIn(ctx context.Context, fields map[string]string) (provider.Auth, error)
	Answer(ctx context.Context, challenge, answer string) (provider.Auth, error)
	SignOut(ctx context.Context) error
	// Platform is the lecture platform's client, signed in through Do.
	Platform() *courseapi.Client
	// Canvas is the Canvas client; CanvasSignIn opens its web session.
	Canvas() *canvaslms.Client
	CanvasSignIn(ctx context.Context) error
}

// Fields is the sign-in form both institutions use.
var Fields = func() []provider.Field {
	remember := true
	return []provider.Field{
		{Key: "username", Title: "学号", Kind: "text"},
		{Key: "password", Title: "密码", Kind: "password", Remember: &remember},
	}
}()

// AuthRequired reports that the source needs signing in again.
func AuthRequired(msg string) error { return provider.Errorf(provider.KindAuthRequired, "%s", msg) }

// Handler serves every protocol method for src.
func Handler(src Source, additionalFields ...provider.Field) provider.Handler {
	fields := slices.Concat(Fields, additionalFields)
	var canvasMu sync.Mutex
	canvasReady := false
	// withCanvas opens the Canvas session on first use and once more when
	// Canvas reports it expired.
	withCanvas := func(ctx context.Context, f func(c *canvaslms.Client) error) error {
		for attempt := 0; ; attempt++ {
			canvasMu.Lock()
			ready := canvasReady
			canvasMu.Unlock()
			if !ready {
				if err := src.CanvasSignIn(ctx); err != nil {
					return err
				}
				canvasMu.Lock()
				canvasReady = true
				canvasMu.Unlock()
			}
			err := f(src.Canvas())
			if !errors.Is(err, canvaslms.ErrAuth) {
				return err
			}
			canvasMu.Lock()
			canvasReady = false
			canvasMu.Unlock()
			if attempt == 1 {
				return AuthRequired("无法打开 Canvas：登录已失效，请重新登录")
			}
		}
	}
	return func(ctx context.Context, method string, params json.RawMessage) (any, error) {
		var p struct {
			Fields    map[string]string `json:"fields"`
			Challenge string            `json:"challenge"`
			Answer    string            `json:"answer"`
			Course    string            `json:"course"`
			Lecture   string            `json:"lecture"`
			Stream    string            `json:"stream"`
			File      string            `json:"file"`
			Courses   []string          `json:"courses"`
			Since     time.Time         `json:"since"`
		}
		if err := provider.Decode(params, &p); err != nil {
			return nil, err
		}
		plat := src.Platform()
		switch method {
		case "initialize":
			in := provider.Init{Protocol: provider.Protocol,
				Capabilities: []string{provider.CapSlides, provider.CapCourseware, provider.CapLMS}}
			in.SignIn.Fields = fields
			return in, nil
		case "auth.status":
			return src.Status(ctx), nil
		case "auth.signIn":
			if p.Fields["username"] == "" || p.Fields["password"] == "" {
				return nil, provider.Errorf(provider.KindInvalid, "无法登录：请填写学号和密码")
			}
			canvasMu.Lock()
			canvasReady = false
			canvasMu.Unlock()
			return src.SignIn(ctx, p.Fields)
		case "auth.answer":
			return src.Answer(ctx, p.Challenge, p.Answer)
		case "auth.signOut":
			canvasMu.Lock()
			canvasReady = false
			canvasMu.Unlock()
			plat.Reset()
			return nil, src.SignOut(ctx)
		case "courses.list":
			cs, err := plat.Courses(ctx)
			if cs == nil {
				cs = []provider.Course{}
			}
			return map[string]any{"courses": cs, "cursor": ""}, err
		case "lectures.list":
			ls, err := plat.Lectures(ctx, p.Course)
			if ls == nil {
				ls = []provider.Lecture{}
			}
			return map[string]any{"lectures": ls, "cursor": ""}, err
		case "media.resolve":
			return plat.Media(ctx, p.Course, p.Lecture)
		case "media.refresh":
			m, err := plat.Media(ctx, p.Course, p.Lecture)
			if err != nil {
				return nil, err
			}
			for _, s := range m.Streams {
				if s.ID == p.Stream {
					return s.Address, nil
				}
			}
			return nil, provider.Errorf(provider.KindInvalid, "无法刷新视频地址：这节课已没有这个画面")
		case "slides.list":
			ss, err := plat.Slides(ctx, p.Course, p.Lecture)
			return map[string]any{"slides": ss}, err
		case "courseware.list":
			fs, err := plat.Courseware(ctx, p.Course, p.Lecture)
			return map[string]any{"files": fs}, err
		case "courseware.resolve":
			fs, err := plat.Courseware(ctx, p.Course, p.Lecture)
			if err != nil {
				return nil, err
			}
			for _, f := range fs {
				if f.ID == p.File {
					return f.Address, nil
				}
			}
			return nil, provider.Errorf(provider.KindInvalid, "无法下载课件：这节课已没有这个文件")
		case "lms.courses":
			var cs []provider.LMSCourse
			err := withCanvas(ctx, func(c *canvaslms.Client) (err error) { cs, err = c.Courses(ctx); return })
			return map[string]any{"courses": cs}, err
		case "lms.assignments":
			var as []provider.Assignment
			err := withCanvas(ctx, func(c *canvaslms.Client) (err error) { as, err = c.Assignments(ctx, p.Course); return })
			return map[string]any{"assignments": as}, err
		case "lms.announcements":
			var as []provider.Announcement
			err := withCanvas(ctx, func(c *canvaslms.Client) (err error) { as, err = c.Announcements(ctx, p.Courses, p.Since); return })
			return map[string]any{"announcements": as}, err
		case "lms.files":
			var fs []provider.File
			err := withCanvas(ctx, func(c *canvaslms.Client) (err error) { fs, err = c.Files(ctx, p.Course); return })
			return map[string]any{"files": fs}, err
		case "lms.file":
			var a provider.Address
			err := withCanvas(ctx, func(c *canvaslms.Client) error {
				u, err := c.FileURL(ctx, p.File)
				if err != nil {
					return err
				}
				a = provider.Address{URL: u, Headers: CookieHeader(c)}
				return nil
			})
			return a, err
		}
		return nil, provider.ErrMethodNotFound
	}
}

// CookieHeader is what the daemon sends to fetch a Canvas file.
func CookieHeader(c *canvaslms.Client) map[string]string {
	h := map[string]string{"User-Agent": courseapi.UserAgent}
	if j, ok := c.HC.Jar.(*jar.Jar); ok {
		if v := j.Header(c.Base + "/"); v != "" {
			h["Cookie"] = v
		}
	}
	return h
}
