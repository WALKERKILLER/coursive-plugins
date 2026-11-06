// Package jar is a cookie jar that can be saved and restored, so a provider
// keeps single sign-on sessions in the daemon's vault across restarts.
package jar

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync"
	"time"
)

type saved struct {
	URL    string       `json:"url"`
	Cookie *http.Cookie `json:"cookie"`
}

// Jar wraps cookiejar.Jar and remembers every cookie it was given.
type Jar struct {
	mu   sync.Mutex
	jar  *cookiejar.Jar
	seen map[string]saved // host|path|name
}

// New returns an empty jar.
func New() *Jar {
	j, _ := cookiejar.New(nil)
	return &Jar{jar: j, seen: map[string]saved{}}
}

func (r *Jar) SetCookies(u *url.URL, cs []*http.Cookie) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.jar.SetCookies(u, cs)
	for _, c := range cs {
		host := c.Domain
		if host == "" {
			host = u.Hostname()
		}
		k := host + "|" + c.Path + "|" + c.Name
		if c.MaxAge < 0 || (!c.Expires.IsZero() && c.Expires.Before(time.Now())) {
			delete(r.seen, k)
			continue
		}
		cc := *c
		if c.MaxAge > 0 {
			cc.Expires = time.Now().Add(time.Duration(c.MaxAge) * time.Second)
			cc.MaxAge = 0
		}
		r.seen[k] = saved{URL: u.Scheme + "://" + u.Host + "/", Cookie: &cc}
	}
}

func (r *Jar) Cookies(u *url.URL) []*http.Cookie {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.jar.Cookies(u)
}

// Header is the Cookie header the jar sends to raw.
func (r *Jar) Header(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	var parts []string
	for _, c := range r.Cookies(u) {
		parts = append(parts, c.Name+"="+c.Value)
	}
	return strings.Join(parts, "; ")
}

// Marshal snapshots the unexpired cookies.
func (r *Jar) Marshal() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]saved, 0, len(r.seen))
	for _, c := range r.seen {
		if c.Cookie.Expires.IsZero() || c.Cookie.Expires.After(time.Now()) {
			out = append(out, c)
		}
	}
	b, _ := json.Marshal(out)
	return string(b)
}

// Restore loads a snapshot made by Marshal.
func (r *Jar) Restore(s string) {
	var cs []saved
	if json.Unmarshal([]byte(s), &cs) != nil {
		return
	}
	for _, c := range cs {
		if u, err := url.Parse(c.URL); err == nil && c.Cookie != nil {
			r.SetCookies(u, []*http.Cookie{c.Cookie})
		}
	}
}

// Reset drops every cookie.
func (r *Jar) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.jar, _ = cookiejar.New(nil)
	r.seen = map[string]saved{}
}
