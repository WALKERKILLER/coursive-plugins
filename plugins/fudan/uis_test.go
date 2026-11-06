package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestVPNHostMatchesKnownGateway(t *testing.T) {
	got := vpnHost("icourse.fudan.edu.cn")
	if !strings.HasPrefix(got, "77726476706e69737468656265737421f9f44e") || len(got) != 32+2*len("icourse.fudan.edu.cn") {
		t.Fatalf("vpnHost = %s", got)
	}
	u, _ := vpnURL("https://icourse.fudan.edu.cn/courseapi/v3/x?a=1")
	if !strings.HasPrefix(u, webVPNBase+"/https/"+got+"/courseapi/v3/x?a=1") {
		t.Fatal(u)
	}
	if u, _ := vpnURL("http://10.0.0.1:8080/p"); !strings.HasPrefix(u, webVPNBase+"/http-8080/") {
		t.Fatal(u)
	}
}

// fakeUIS plays UIS and a service that trusts its tickets.
func fakeUIS(t *testing.T, password string) (idp, svc *httptest.Server) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	der, _ := x509.MarshalPKIXPublicKey(&key.PublicKey)
	mux := http.NewServeMux()
	idp = httptest.NewServer(mux)
	svcMux := http.NewServeMux()
	svc = httptest.NewServer(svcMux)
	t.Cleanup(idp.Close)
	t.Cleanup(svc.Close)
	svcMux.HandleFunc("/login/cas", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("ticket") == "ST-1" {
			http.SetCookie(w, &http.Cookie{Name: "svc_session", Value: "ok", Path: "/"})
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
		if c, err := r.Cookie("svc_session"); err == nil && c.Value == "ok" {
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
		http.Redirect(w, r, idp.URL+"/ac/#/index?lck=L%2B1&entityId="+svc.URL, http.StatusFound)
	})
	svcMux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("home")) })
	mux.HandleFunc("/ac/", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("<html>login</html>")) })
	mux.HandleFunc("/idp/authn/queryAuthMethods", func(w http.ResponseWriter, r *http.Request) {
		var in map[string]string
		json.NewDecoder(r.Body).Decode(&in)
		if in["lck"] != "L+1" || in["entityId"] != svc.URL {
			http.Error(w, "bad context", 400)
			return
		}
		w.Write([]byte(`{"code":200,"requestType":"chain_type","data":[{"moduleCode":"qr","authChainCode":"q"},{"moduleCode":"userAndPwd","authChainCode":"C1"}]}`))
	})
	mux.HandleFunc("/idp/authn/getJsPublicKey", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"code": "200", "data": base64.StdEncoding.EncodeToString(der)})
	})
	mux.HandleFunc("/idp/authn/authExecute", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			AuthChainCode string            `json:"authChainCode"`
			RequestType   string            `json:"requestType"`
			AuthPara      map[string]string `json:"authPara"`
		}
		json.NewDecoder(r.Body).Decode(&in)
		ct, _ := base64.StdEncoding.DecodeString(in.AuthPara["password"])
		pw, err := rsa.DecryptPKCS1v15(nil, key, ct)
		if err != nil || string(pw) != password || in.AuthChainCode != "C1" || in.RequestType != "chain_type" || in.AuthPara["loginName"] != "20300" {
			w.Write([]byte(`{"code":"500","message":"用户名或密码错误"}`))
			return
		}
		w.Write([]byte(`{"code":"200","loginToken":"T1"}`))
	})
	mux.HandleFunc("/idp/authCenter/authnEngine", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		if r.Form.Get("loginToken") != "T1" {
			http.Error(w, "no", 400)
			return
		}
		w.Write([]byte(`<script>var locationValue = "` + svc.URL + `/login/cas?ticket=ST-1&amp;x=1";</script>`))
	})
	return idp, svc
}

func client() *http.Client { j, _ := cookiejar.New(nil); return &http.Client{Jar: j} }

func TestUISLogin(t *testing.T) {
	idp, svc := fakeUIS(t, "secret")
	hc := client()
	u := &uis{base: idp.URL, hc: hc}
	if err := u.login(t.Context(), svc.URL+"/login/cas", "20300", "secret"); err != nil {
		t.Fatal(err)
	}
	// Already signed in: the service answers without UIS.
	if err := u.login(t.Context(), svc.URL+"/login/cas", "20300", "wrong"); err != nil {
		t.Fatal(err)
	}
	err := (&uis{base: idp.URL, hc: client()}).login(t.Context(), svc.URL+"/login/cas", "20300", "wrong")
	if err == nil || !strings.Contains(err.Error(), "用户名或密码错误") {
		t.Fatalf("want credential error, got %v", err)
	}
	if err := (&uis{base: idp.URL, hc: client()}).login(t.Context(), svc.URL+"/login/cas", "20300", ""); err != errNeedsPassword {
		t.Fatalf("want errNeedsPassword, got %v", err)
	}
}

func TestUISExistingSessionFollowsTicketWithoutPasswordExchange(t *testing.T) {
	accepted := make(chan bool, 1)
	svc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		accepted <- r.URL.Query().Get("ticket") == "existing-session" && r.URL.Query().Get("x") == "1"
		w.Write([]byte("service"))
	}))
	defer svc.Close()
	idp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ac/" || r.Method != "GET" {
			t.Error("live SSO session unexpectedly entered password authentication")
			http.Error(w, "unexpected authentication request", 400)
			return
		}
		w.Write([]byte(`<script>var locationValue = "` + svc.URL + `/?ticket=existing-session&amp;x=1";</script>`))
	}))
	defer idp.Close()
	u := &uis{base: idp.URL, hc: client()}
	if err := u.login(t.Context(), idp.URL+"/ac/", "", ""); err != nil {
		t.Fatal(err)
	}
	if len(accepted) != 1 || !<-accepted {
		t.Fatal("existing service ticket was not accepted")
	}
}

type trackedBody struct {
	io.Reader
	closed bool
}

func (b *trackedBody) Close() error { b.closed = true; return nil }

func TestVPNSignedOut(t *testing.T) {
	for _, tt := range []struct {
		name, path, body, contentType string
		status                        int
		signedOut                     bool
	}{
		{"JSON mislabeled as HTML", "/https/encoded/courseapi/detail", `{"code":0,"data":{"sub_list":[]}}`, "text/html; charset=UTF-8", 200, false},
		{"long JSON preserves all bytes", "/https/encoded/courseapi/detail", "  {\"data\":\"" + strings.Repeat("x", 4096) + "\"}", "text/html", 200, false},
		{"JSON array", "/https/encoded/api", "\n[]", "text/html", 200, false},
		{"actual login HTML", "/https/encoded/ac/", "<!doctype html><html>login</html>", "text/html", 200, true},
		{"gateway login redirect", "/login", `{"code":0}`, "application/json", 200, true},
		{"unauthorized", "/https/encoded/api", `{"code":401}`, "application/json", 401, true},
		{"video range", "/https/encoded/video.mp4", "video data", "video/mp4", 206, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			body := &trackedBody{Reader: strings.NewReader(tt.body)}
			resp := &http.Response{StatusCode: tt.status, Header: http.Header{"Content-Type": {tt.contentType}},
				Request: &http.Request{URL: &url.URL{Scheme: "https", Host: "webvpn.fudan.edu.cn", Path: tt.path}}, Body: body}
			if got := vpnSignedOut(resp); got != tt.signedOut {
				t.Errorf("signedOut=%v want %v", got, tt.signedOut)
			}
			raw, err := io.ReadAll(resp.Body)
			if err != nil || string(raw) != tt.body {
				t.Fatal("classification consumed or changed body")
			}
			resp.Body.Close()
			if !body.closed {
				t.Fatal("original response body not closed")
			}
		})
	}
}
