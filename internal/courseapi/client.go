// Package courseapi reads a lecture-recording platform's course API: the
// enrolled courses, each course's recorded classes, their signed media
// addresses, slide snapshots and courseware. Several institutions run the
// same platform; a provider supplies how requests are authenticated and how
// the daemon reaches the addresses.
package courseapi

import (
	"context"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/walkerkiller/coursive-plugins/sdk/provider"
)

// UserAgent is sent with every platform request.
const UserAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36"

// Client talks to one platform.
type Client struct {
	// Base is the platform origin, such as https://lectures.example.edu.
	Base string
	// Tenant is the platform's tenant code.
	Tenant string
	// Do performs an authenticated API request.
	Do func(ctx context.Context, req *http.Request) (*http.Response, error)
	// Address turns a platform URL into what the daemon fetches.
	Address func(raw string) provider.Address

	mu   sync.Mutex
	user *User
}

// User is the signed-in platform user.
type User struct {
	ID       int64
	TenantID int64
	Account  string
	Realname string
	Phone    string
}

// flex decodes values the API returns as either string or number.
type flex string

func (f *flex) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		*f = ""
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		*f = flex(s)
		return nil
	}
	*f = flex(strings.Trim(string(b), `"`))
	return nil
}
func (f flex) Int() int64     { n, _ := strconv.ParseInt(string(f), 10, 64); return n }
func (f flex) String() string { return string(f) }

// HTTPError is a non-200 answer.
type HTTPError struct {
	Path string
	Code int
}

func (e *HTTPError) Error() string { return fmt.Sprintf("%s: HTTP %d", e.Path, e.Code) }

func (c *Client) getJSON(ctx context.Context, path string, q url.Values, out any) error {
	u := c.Base + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept-Language", "zh_cn")
	resp, err := c.Do(ctx, req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return &HTTPError{Path: path, Code: resp.StatusCode}
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, out); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

func okCode(v any) bool {
	switch fmt.Sprint(v) {
	case "0", "200", "1000":
		return true
	}
	return false
}

// UserInfo returns the signed-in user, cached until Reset.
func (c *Client) UserInfo(ctx context.Context) (*User, error) {
	c.mu.Lock()
	if c.user != nil {
		u := c.user
		c.mu.Unlock()
		return u, nil
	}
	c.mu.Unlock()
	var r struct {
		Code   any `json:"code"`
		Params struct {
			ID       int64  `json:"id"`
			TenantID int64  `json:"tenant_id"`
			Account  string `json:"account"`
			Realname string `json:"realname"`
			Phone    flex   `json:"phone"`
		} `json:"params"`
	}
	if err := c.getJSON(ctx, "/userapi/v1/infosimple", nil, &r); err != nil {
		return nil, err
	}
	if !okCode(r.Code) {
		return nil, fmt.Errorf("userinfo code=%v", r.Code)
	}
	p := r.Params
	u := &User{ID: p.ID, TenantID: p.TenantID, Account: p.Account, Realname: p.Realname, Phone: p.Phone.String()}
	c.mu.Lock()
	c.user = u
	c.mu.Unlock()
	return u, nil
}

// Reset drops the cached user (after signing out or switching accounts).
func (c *Client) Reset() { c.mu.Lock(); c.user = nil; c.mu.Unlock() }

// Courses lists every enrolled course, all pages.
func (c *Client) Courses(ctx context.Context) ([]provider.Course, error) {
	var out []provider.Course
	seen := map[string]bool{}
	for page := 1; page <= 20; page++ {
		var r struct {
			Params struct {
				Result struct {
					Total int `json:"total"`
					Data  []struct {
						ID       flex   `json:"Id"`
						Title    string `json:"Title"`
						Teacher  string `json:"Teacher"`
						Teachers []struct {
							Username string `json:"Username"`
							Realname string `json:"Realname"`
						} `json:"Teachers"`
						College  string `json:"KkxyName"`
						TermName string `json:"TermName"`
						Thumb    string `json:"Thumb"`
						StartAt  int64  `json:"StartAt"`
						EndAt    int64  `json:"EndAt"`
						Progress struct {
							SubjectNum      int     `json:"subjectNum"`
							SubjectProgress float64 `json:"subjectProgress"`
						} `json:"progress"`
						PrevSubjectID flex   `json:"PrevSubjectId"`
						PrevLearnTime string `json:"PrevLearnTime"`
					} `json:"data"`
				} `json:"result"`
			} `json:"params"`
		}
		q := url.Values{"nowpage": {strconv.Itoa(page)}, "per-page": {"100"}}
		if err := c.getJSON(ctx, "/personal/courseapi/vlabpassportapi/v1/account-profile/course", q, &r); err != nil {
			if page == 1 {
				return nil, err
			}
			break
		}
		items := r.Params.Result.Data
		for _, it := range items {
			id := it.ID.String()
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			co := provider.Course{ID: id, Title: it.Title, Teacher: it.Teacher, College: it.College, Term: it.TermName,
				Thumb: it.Thumb, StartAt: it.StartAt, EndAt: it.EndAt, LectureTotal: it.Progress.SubjectNum,
				Progress: it.Progress.SubjectProgress, LastLectureID: it.PrevSubjectID.String(), LastLearnAt: it.PrevLearnTime}
			if co.LastLectureID == "0" {
				co.LastLectureID = ""
			}
			for _, t := range it.Teachers {
				co.Teachers = append(co.Teachers, provider.Teacher{Username: t.Username, Realname: t.Realname})
			}
			out = append(out, co)
		}
		if len(items) == 0 || page*100 >= r.Params.Result.Total {
			break
		}
	}
	return out, nil
}

type rawLecture struct {
	ID         flex   `json:"id"`
	SubTitle   string `json:"sub_title"`
	ClassBegin flex   `json:"class_begin"`
	ClassOver  flex   `json:"class_over"`
	Lecturer   string `json:"lecturer_name"`
	Room       string `json:"room_name"`
	Playback   flex   `json:"playback_status"`
	Date       string `json:"date"`
}

// Lectures lists a course's recorded classes, newest first.
func (c *Client) Lectures(ctx context.Context, courseID string) ([]provider.Lecture, error) {
	var r struct {
		Code any `json:"code"`
		Data struct {
			SubList json.RawMessage `json:"sub_list"`
		} `json:"data"`
	}
	q := url.Values{"course_id": {courseID}, "tenant_code": {c.Tenant}}
	if err := c.getJSON(ctx, "/courseapi/v3/multi-search/get-course-detail", q, &r); err != nil {
		return nil, err
	}
	if !okCode(r.Code) {
		return nil, fmt.Errorf("course detail code=%v", r.Code)
	}
	var out []provider.Lecture
	add := func(rl rawLecture, date string) {
		if rl.ID == "" {
			return
		}
		l := provider.Lecture{ID: rl.ID.String(), Title: rl.SubTitle, BeginAt: rl.ClassBegin.Int(), EndAt: rl.ClassOver.Int(),
			Lecturer: rl.Lecturer, Room: rl.Room, HasPlayback: rl.Playback == "1", Date: date}
		// class_begin is authoritative; the nested sub_list keys are not calendar dates.
		if l.BeginAt > 0 {
			l.Date = time.Unix(l.BeginAt, 0).In(shanghai).Format("2006-01-02")
		} else if rl.Date != "" {
			l.Date = rl.Date
		}
		out = append(out, l)
	}
	// sub_list is either {year:{month:{day:[...]}}} or a flat list.
	var nested map[string]map[string]map[string][]rawLecture
	var flat []rawLecture
	if json.Unmarshal(r.Data.SubList, &nested) == nil {
		for y, ms := range nested {
			for m, ds := range ms {
				for day, items := range ds {
					for _, it := range items {
						add(it, fmt.Sprintf("%s-%s-%s", y, pad2(m), pad2(day)))
					}
				}
			}
		}
	} else if json.Unmarshal(r.Data.SubList, &flat) == nil {
		for _, it := range flat {
			add(it, "")
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].BeginAt != out[j].BeginAt {
			return out[i].BeginAt > out[j].BeginAt
		}
		return out[i].ID > out[j].ID
	})
	return out, nil
}

var shanghai = func() *time.Location {
	if loc, err := time.LoadLocation("Asia/Shanghai"); err == nil {
		return loc
	}
	return time.FixedZone("CST", 8*3600)
}()

func pad2(s string) string {
	if len(s) == 1 {
		return "0" + s
	}
	return s
}

type rawVideo struct {
	PreviewURL string `json:"preview_url"`
	Thumb      string `json:"thumb"`
	Duration   flex   `json:"duration"`
}

// streamKinds maps the platform's view labels to protocol kinds.
var streamKinds = map[string]string{"教师": "camera", "学生": "camera", "屏幕": "screen"}

// StreamID names a stream by its unsigned address, so it survives re-signing.
func StreamID(raw string) string {
	sum := sha1.Sum([]byte(raw))
	return hex.EncodeToString(sum[:6])
}

// Media resolves a lecture's signed streams.
func (c *Client) Media(ctx context.Context, courseID, subID string) (*provider.Media, error) {
	var r struct {
		Code any `json:"code"`
		Data struct {
			VideoList   map[string]json.RawMessage `json:"video_list"`
			SegmentList map[string]json.RawMessage `json:"segment_video_list"`
			PlayURL     map[string]any             `json:"playurl"`
			Now         flex                       `json:"now"`
			Duration    flex                       `json:"duration"`
			AllowDrag   flex                       `json:"is_allow_video_drag"`
		} `json:"data"`
	}
	q := url.Values{"course_id": {courseID}, "sub_id": {subID}, "tenant_code": {c.Tenant}}
	if err := c.getJSON(ctx, "/courseapi/v3/portal-home-setting/get-sub-info", q, &r); err != nil {
		return nil, err
	}
	if !okCode(r.Code) {
		return nil, fmt.Errorf("sub info code=%v", r.Code)
	}
	m := &provider.Media{Streams: []provider.Stream{}, Duration: r.Data.Duration.Int(), Seekable: r.Data.AllowDrag != "0"}
	var raws []string
	seen := map[string]bool{}
	add := func(label string, v rawVideo) {
		u := v.PreviewURL
		if u == "" || seen[u] || !(strings.Contains(u, ".mp4") || strings.Contains(u, ".m3u8")) {
			return
		}
		seen[u] = true
		raws = append(raws, u)
		m.Streams = append(m.Streams, provider.Stream{ID: StreamID(u), Kind: streamKinds[label], Label: label, Thumb: v.Thumb, Duration: v.Duration.Int()})
	}
	keys := make([]string, 0, len(r.Data.SegmentList))
	for k := range r.Data.SegmentList {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		var seg struct {
			Teacher json.RawMessage `json:"teacher_list"`
			Student json.RawMessage `json:"student_list"`
			PPT     json.RawMessage `json:"ppt_list"`
		}
		if json.Unmarshal(r.Data.SegmentList[k], &seg) != nil {
			continue
		}
		for _, pair := range []struct {
			label string
			raw   json.RawMessage
		}{{"教师", seg.Teacher}, {"屏幕", seg.PPT}, {"学生", seg.Student}} {
			var v rawVideo
			if json.Unmarshal(pair.raw, &v) == nil {
				add(pair.label, v)
			}
		}
	}
	vkeys := make([]string, 0, len(r.Data.VideoList))
	for k := range r.Data.VideoList {
		vkeys = append(vkeys, k)
	}
	sort.Strings(vkeys)
	for _, k := range vkeys {
		// Some deployments put scalar metadata such as "now" beside the videos.
		var video rawVideo
		if json.Unmarshal(r.Data.VideoList[k], &video) == nil {
			add("视频", video)
		}
	}
	if len(m.Streams) == 0 {
		for k, v := range r.Data.PlayURL {
			if s, ok := v.(string); ok && k != "now" {
				add("视频", rawVideo{PreviewURL: s})
			}
		}
	}
	now := r.Data.Now.Int()
	if now == 0 {
		var nestedNow flex
		if json.Unmarshal(r.Data.VideoList["now"], &nestedNow) == nil {
			now = nestedNow.Int()
		}
	}
	for i := range m.Streams {
		signed, err := c.Sign(ctx, raws[i], now)
		if err != nil {
			return nil, err
		}
		m.Streams[i].Address = c.Address(signed)
		if m.Streams[i].Duration == 0 {
			m.Streams[i].Duration = m.Duration
		}
	}
	if m.Duration == 0 && len(m.Streams) > 0 {
		m.Duration = m.Streams[0].Duration
	}
	return m, nil
}

// Courseware lists a lecture's attachments with their addresses.
func (c *Client) Courseware(ctx context.Context, courseID, subID string) ([]provider.File, error) {
	var r struct {
		Data struct {
			Content struct {
				Download map[string]struct {
					FileName string `json:"file_name"`
					PathName string `json:"path_name"`
				} `json:"download"`
			} `json:"content"`
		} `json:"data"`
	}
	q := url.Values{"course_id": {courseID}, "sub_id": {subID}, "tenant_code": {c.Tenant}}
	if err := c.getJSON(ctx, "/courseapi/v3/multi-search/get-sub-detail", q, &r); err != nil {
		return nil, err
	}
	out := []provider.File{}
	for _, d := range r.Data.Content.Download {
		if d.PathName != "" {
			out = append(out, provider.File{ID: StreamID(d.PathName), Name: d.FileName, Address: c.Address(d.PathName)})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Sign adds the platform CDN's authentication (clientUUID and t) to a media URL.
func (c *Client) Sign(ctx context.Context, raw string, now int64) (string, error) {
	if strings.Contains(raw, "clientUUID=") {
		return raw, nil
	}
	u, err := c.UserInfo(ctx)
	if err != nil {
		return "", err
	}
	if now == 0 {
		now = time.Now().Unix()
	}
	pu, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	phone := []rune(u.Phone)
	for i, j := 0, len(phone)-1; i < j; i, j = i+1, j-1 {
		phone[i], phone[j] = phone[j], phone[i]
	}
	sum := md5.Sum([]byte(fmt.Sprintf("%s%d%d%s%d", pu.Path, u.ID, u.TenantID, string(phone), now)))
	t := fmt.Sprintf("%d-%d-%s", u.ID, now, hex.EncodeToString(sum[:]))
	sep := "?"
	if strings.Contains(raw, "?") {
		sep = "&"
	}
	return raw + sep + "clientUUID=" + uuid4() + "&t=" + t, nil
}

func uuid4() string {
	var b [16]byte
	rand.Read(b[:])
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	h := hex.EncodeToString(b[:])
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}

// Slides lists the slide snapshots detected in a lecture, sorted by time.
func (c *Client) Slides(ctx context.Context, courseID, subID string) ([]provider.Slide, error) {
	out := []provider.Slide{}
	seen := map[string]bool{}
	for page := 1; page <= 30; page++ {
		var r struct {
			Code  any  `json:"code"`
			Total flex `json:"total"`
			List  []struct {
				Content    string `json:"content"`
				CreatedSec int64  `json:"created_sec"`
			} `json:"list"`
		}
		q := url.Values{"course_id": {courseID}, "sub_id": {subID}, "page": {strconv.Itoa(page)}, "per_page": {"100"}}
		if err := c.getJSON(ctx, "/pptnote/v1/schedule/search-ppt", q, &r); err != nil {
			return nil, err
		}
		if !okCode(r.Code) {
			return nil, fmt.Errorf("search-ppt code=%v", r.Code)
		}
		added := 0
		for _, it := range r.List {
			var ct struct {
				Img   string `json:"pptimgurl"`
				Thumb string `json:"pptthumb"`
				IsKey bool   `json:"is_key"`
			}
			json.Unmarshal([]byte(it.Content), &ct)
			img := ct.Img
			if img == "" {
				img = ct.Thumb
			}
			k := strconv.FormatInt(it.CreatedSec, 10) + img
			if img == "" || seen[k] {
				continue
			}
			seen[k] = true
			a := c.Address(img)
			s := provider.Slide{TSec: it.CreatedSec, URL: a.URL, Key: ct.IsKey, Headers: a.Headers}
			if ct.Thumb != "" {
				s.ThumbURL = c.Address(ct.Thumb).URL
			}
			out = append(out, s)
			added++
		}
		if added == 0 || len(r.List) < 100 || (r.Total.Int() > 0 && len(out) >= int(r.Total.Int())) {
			break
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].TSec < out[j].TSec })
	return out, nil
}
