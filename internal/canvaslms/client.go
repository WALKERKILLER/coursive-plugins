// Package canvaslms reads courses, assignments, announcements and files from
// a Canvas LMS instance with the student's web session, which a provider
// obtains through its institution's single sign-on.
package canvaslms

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/walkerkiller/coursive-plugins/sdk/provider"
)

var (
	// ErrAuth means Canvas rejected the session.
	ErrAuth = errors.New("Canvas session expired")
	// ErrIdP means single sign-on stopped at the identity provider, which
	// wants credentials again.
	ErrIdP = errors.New("identity provider wants credentials again")
)

const userAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36"

// Client talks to one Canvas instance.
type Client struct {
	Base string       // e.g. https://canvas.example.edu
	HC   *http.Client // carries the web session cookies
}

func (c *Client) do(ctx context.Context, path string) (*http.Response, error) {
	u := path
	if !strings.HasPrefix(path, "http") {
		u = c.Base + path
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json+canvas-string-ids, application/json")
	req.Header.Set("User-Agent", userAgent)
	// An expired session is redirected into the sign-in flow, which may end
	// anywhere; stop at the first redirect instead.
	hc := *c.HC
	hc.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	// Without a signed-in user Canvas answers 401 for most of the API but
	// 404 for /users/self, which has no user to resolve.
	if resp.StatusCode == http.StatusUnauthorized || (resp.StatusCode >= 300 && resp.StatusCode < 400) ||
		(resp.StatusCode == http.StatusNotFound && req.URL.Path == "/api/v1/users/self") {
		resp.Body.Close()
		return nil, ErrAuth
	}
	if resp.StatusCode >= 400 {
		resp.Body.Close()
		return nil, &HTTPError{Code: resp.StatusCode, Path: req.URL.Path}
	}
	return resp, nil
}

// HTTPError is a Canvas API failure other than authentication.
type HTTPError struct {
	Code int
	Path string
}

func (e *HTTPError) Error() string { return fmt.Sprintf("Canvas %s: HTTP %d", e.Path, e.Code) }

// Canvas prefixes JSON with "while(1);" for session requests.
func decodeJSON(r io.Reader, out any) error {
	b, err := io.ReadAll(io.LimitReader(r, 32<<20))
	if err != nil {
		return err
	}
	b = []byte(strings.TrimPrefix(string(b), "while(1);"))
	return json.Unmarshal(b, out)
}

func (c *Client) getJSON(ctx context.Context, path string, out any) error {
	resp, err := c.do(ctx, path)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return decodeJSON(resp.Body, out)
}

var nextLink = regexp.MustCompile(`<([^>]+)>;\s*rel="next"`)

// getAll follows Link: rel="next" pages (at most 50).
func getAll[T any](ctx context.Context, c *Client, path string) ([]T, error) {
	var out []T
	for page := 0; path != "" && page < 50; page++ {
		resp, err := c.do(ctx, path)
		if err != nil {
			return out, err
		}
		var items []T
		err = decodeJSON(resp.Body, &items)
		resp.Body.Close()
		if err != nil {
			return out, err
		}
		out = append(out, items...)
		path = ""
		if m := nextLink.FindStringSubmatch(resp.Header.Get("Link")); m != nil {
			path = m[1]
		}
	}
	return out, nil
}

// User is the signed-in Canvas user.
type User struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Self returns the signed-in user, or ErrAuth.
func (c *Client) Self(ctx context.Context) (User, error) {
	var u User
	return u, c.getJSON(ctx, "/api/v1/users/self", &u)
}

// Courses lists the active enrollments.
func (c *Client) Courses(ctx context.Context) ([]provider.LMSCourse, error) {
	type course struct {
		ID         string `json:"id"`
		Name       string `json:"name"`
		CourseCode string `json:"course_code"`
		Term       *struct {
			Name string `json:"name"`
		} `json:"term"`
	}
	cs, err := getAll[course](ctx, c, "/api/v1/courses?enrollment_state=active&include[]=term&per_page=100")
	out := make([]provider.LMSCourse, 0, len(cs))
	for _, k := range cs {
		if k.Name == "" {
			continue // restricted enrollments have no name
		}
		co := provider.LMSCourse{ID: k.ID, Title: k.Name, Code: k.CourseCode}
		if k.Term != nil {
			co.Term = k.Term.Name
		}
		out = append(out, co)
	}
	return out, err
}

// Assignments lists a course's assignments.
func (c *Client) Assignments(ctx context.Context, courseID string) ([]provider.Assignment, error) {
	type assignment struct {
		ID             string     `json:"id"`
		Name           string     `json:"name"`
		DueAt          *time.Time `json:"due_at"`
		HTMLURL        string     `json:"html_url"`
		PointsPossible *float64   `json:"points_possible"`
		Submission     *struct {
			WorkflowState string     `json:"workflow_state"`
			SubmittedAt   *time.Time `json:"submitted_at"`
		} `json:"submission"`
	}
	as, err := getAll[assignment](ctx, c, "/api/v1/courses/"+url.PathEscape(courseID)+"/assignments?include[]=submission&order_by=due_at&per_page=100")
	out := make([]provider.Assignment, 0, len(as))
	for _, a := range as {
		submitted := false
		if s := a.Submission; s != nil {
			switch s.WorkflowState {
			case "submitted", "graded", "pending_review":
				submitted = true
			default:
				submitted = s.SubmittedAt != nil
			}
		}
		out = append(out, provider.Assignment{ID: a.ID, Course: courseID, Title: a.Name, DueAt: a.DueAt,
			URL: a.HTMLURL, Points: a.PointsPossible, Submitted: submitted})
	}
	return out, err
}

// Announcements lists the given courses' announcements posted since since.
func (c *Client) Announcements(ctx context.Context, courseIDs []string, since time.Time) ([]provider.Announcement, error) {
	type announcement struct {
		ID          string     `json:"id"`
		Title       string     `json:"title"`
		Message     string     `json:"message"`
		PostedAt    *time.Time `json:"posted_at"`
		ContextCode string     `json:"context_code"`
		HTMLURL     string     `json:"html_url"`
		Author      struct {
			DisplayName string `json:"display_name"`
		} `json:"author"`
	}
	out := []provider.Announcement{}
	for i := 0; i < len(courseIDs); i += 10 {
		q := url.Values{"start_date": {since.UTC().Format(time.RFC3339)}, "end_date": {time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339)}, "per_page": {"50"}}
		for _, id := range courseIDs[i:min(i+10, len(courseIDs))] {
			q.Add("context_codes[]", "course_"+id)
		}
		page, err := getAll[announcement](ctx, c, "/api/v1/announcements?"+q.Encode())
		for _, a := range page {
			out = append(out, provider.Announcement{ID: a.ID, Course: strings.TrimPrefix(a.ContextCode, "course_"), Title: a.Title,
				HTML: a.Message, PostedAt: a.PostedAt, URL: a.HTMLURL, Author: a.Author.DisplayName})
		}
		if err != nil {
			return out, err
		}
	}
	return out, nil
}

type file struct {
	ID          string     `json:"id"`
	DisplayName string     `json:"display_name"`
	Size        int64      `json:"size"`
	UpdatedAt   *time.Time `json:"updated_at"`
	URL         string     `json:"url"`
}

func (f file) entry(folder string) provider.File {
	e := provider.File{ID: f.ID, Name: f.DisplayName, Folder: folder, Size: f.Size}
	if f.UpdatedAt != nil {
		e.UpdatedAt = f.UpdatedAt.UTC().Format(time.RFC3339)
	}
	return e
}

// Files lists a course's files, newest first. Courses that hide the Files
// page from students still list files placed in modules.
func (c *Client) Files(ctx context.Context, courseID string) ([]provider.File, error) {
	fs, err := getAll[file](ctx, c, "/api/v1/courses/"+url.PathEscape(courseID)+"/files?sort=updated_at&order=desc&per_page=100")
	var he *HTTPError
	if err == nil || !errors.As(err, &he) || (he.Code != http.StatusForbidden && he.Code != http.StatusNotFound) {
		out := make([]provider.File, 0, len(fs))
		for _, f := range fs {
			out = append(out, f.entry(""))
		}
		return out, err
	}
	type module struct {
		Name  string `json:"name"`
		Items []struct {
			Type      string `json:"type"`
			Title     string `json:"title"`
			ContentID string `json:"content_id"`
		} `json:"items"`
	}
	mods, err := getAll[module](ctx, c, "/api/v1/courses/"+url.PathEscape(courseID)+"/modules?include[]=items&per_page=50")
	if err != nil {
		return nil, err
	}
	out := []provider.File{}
	for _, m := range mods {
		for _, it := range m.Items {
			if it.Type == "File" && it.ContentID != "" {
				out = append(out, file{ID: it.ContentID, DisplayName: it.Title}.entry(m.Name))
			}
		}
	}
	return out, nil
}

// FileURL returns a file's fresh download URL.
func (c *Client) FileURL(ctx context.Context, id string) (string, error) {
	var f file
	if err := c.getJSON(ctx, "/api/v1/files/"+url.PathEscape(id), &f); err != nil {
		return "", err
	}
	if f.URL == "" {
		return "", errors.New("the file has no download address; the teacher may have locked it")
	}
	return f.URL, nil
}

// OIDCSignIn opens Canvas's OpenID Connect login with hc's cookies. With a
// live identity-provider session the redirects end on Canvas, signed in;
// otherwise they stop at the provider and ErrIdP is returned.
func OIDCSignIn(ctx context.Context, hc *http.Client, base string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/login/openid_connect", nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := hc.Do(req)
	if err != nil {
		return fmt.Errorf("open Canvas sign-in: %w", err)
	}
	io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<20))
	resp.Body.Close()
	b, _ := url.Parse(base)
	final := resp.Request.URL
	if final.Host != b.Host {
		return ErrIdP
	}
	if strings.HasPrefix(final.Path, "/login") {
		return fmt.Errorf("Canvas sign-in stopped at %s", final.Path)
	}
	return nil
}
