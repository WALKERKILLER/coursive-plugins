package courseapi

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/walkerkiller/coursive-plugins/sdk/provider"
)

func TestMediaSignsStreams(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/userapi/v1/infosimple":
			w.Write([]byte(`{"code":0,"params":{"id":7,"tenant_id":3,"phone":"123"}}`))
		case "/courseapi/v3/portal-home-setting/get-sub-info":
			w.Write([]byte(`{"code":0,"data":{"now":100,"duration":"60","segment_video_list":{"0":{"teacher_list":{"preview_url":"https://cdn.example/a/t.mp4"},"ppt_list":{"preview_url":"https://cdn.example/a/p.mp4"}}}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := &Client{Base: srv.URL, Tenant: "1",
		Do:      func(_ context.Context, r *http.Request) (*http.Response, error) { return http.DefaultClient.Do(r) },
		Address: func(raw string) provider.Address { return provider.Address{URL: raw} }}
	m, err := c.Media(t.Context(), "c", "s")
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Streams) != 2 || m.Streams[0].Kind != "camera" || m.Streams[1].Kind != "screen" || m.Duration != 60 {
		t.Fatalf("streams = %+v", m)
	}
	u, _ := url.Parse(m.Streams[0].URL)
	sum := md5.Sum([]byte("/a/t.mp4" + "7" + "3" + "321" + "100"))
	if got := u.Query().Get("t"); got != "7-100-"+hex.EncodeToString(sum[:]) || u.Query().Get("clientUUID") == "" {
		t.Fatalf("t = %s", got)
	}
	if m.Streams[0].ID != StreamID("https://cdn.example/a/t.mp4") || strings.Contains(m.Streams[0].ID, "/") {
		t.Fatalf("id = %s", m.Streams[0].ID)
	}
}
