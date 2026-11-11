package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestTOTPSixDigitRFCVectors(t *testing.T) {
	key, err := totpKey("gezdgnbv gy3tqojq gezdgnbv gy3tqojq")
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []struct {
		seconds int64
		want    string
	}{{59, "287082"}, {1111111109, "081804"}, {20000000000, "353130"}} {
		if code := totpCode(key, time.Unix(v.seconds, 0)); code != v.want {
			t.Errorf("at %d: got %s, want %s", v.seconds, code, v.want)
		}
	}
}

func TestIncompleteSecondFactor(t *testing.T) {
	for _, tc := range []struct {
		name, secret, module, number, reply string
		posts                               int
	}{
		{"missing key", "", "userAndOtp", "r", "", 0},
		{"invalid key", "bad!", "userAndOtp", "r", "", 0},
		{"unsupported method", "MY======", "sms", "r", "", 0},
		{"incomplete context", "MY======", "userAndOtp", "", "", 0},
		{"rejected code", "MY======", "userAndOtp", "r", `{"code":401,"message":"private OTP response"}`, 1},
		{"further challenge", "MY======", "userAndOtp", "r", `{"code":200,"second":true,"loginToken":"incomplete"}`, 1},
		{"missing token", "MY======", "userAndOtp", "r", `{"code":200}`, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			posts := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { posts++; w.Write([]byte(tc.reply)) }))
			defer srv.Close()
			u := uis{base: srv.URL, hc: srv.Client(), totpSecret: tc.secret}
			ch := authResult{ModuleCodes: []string{tc.module}, RequestNumber: tc.number, AuthChainCode: "C"}
			tok, err := u.completeTOTP(t.Context(), ch, "account", "e", "chain", "l", time.Unix(59, 0))
			if tok != "" || !errors.Is(err, errSecondFactor) || posts != tc.posts {
				t.Errorf("token %q, error %v, posts %d", tok, err, posts)
			}
		})
	}
}

func TestRejectedFactorStopsPasswordReplay(t *testing.T) {
	var src source
	passwords := 0
	login := func(password string) error {
		if password == "" {
			return errNeedsPassword
		}
		passwords++
		return errSecondFactor
	}
	for range 2 {
		if err := src.withSavedPassword("password", login); !errors.Is(err, errSecondFactor) {
			t.Fatal(err)
		}
	}
	if passwords != 1 {
		t.Fatalf("submitted password %d times", passwords)
	}
	if err := src.withSavedPassword("password", func(string) error { return nil }); err != nil {
		t.Fatal("live SSO must still work", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// The campus service challenges through the gateway after WebVPN itself has
// accepted the first login. The OTP must follow this second leg's URL/session.
func TestSecondFactorThroughWebVPN(t *testing.T) {
	hc := client()
	gateway, _ := vpnURL(uisBase)
	u := uis{hc: hc, vpn: true, totpSecret: "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"}
	vpnOrigin, err := url.Parse(webVPNBase)
	if err != nil {
		t.Fatal(err)
	}
	hc.Jar.SetCookies(vpnOrigin, []*http.Cookie{{Name: "gateway", Value: "session", Path: "/"}})
	hc.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != gateway+"/idp/authn/authExecute" || r.Header.Get("Referer") != gateway+"/ac/" || r.Header.Get("Origin") != webVPNBase || r.Method != "POST" {
			t.Fatal("OTP submitted outside its WebVPN context")
		}
		if cookie, err := r.Cookie("gateway"); err != nil || cookie.Value != "session" {
			t.Fatal("missing VPN session")
		}
		var body struct {
			AuthPara map[string]string `json:"authPara"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.AuthPara["otpCode"] != "287082" || body.AuthPara["loginName"] != "student" {
			t.Fatal("wrong OTP or login name")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"code":"200","loginToken":"ticket-token"}`)), Header: make(http.Header)}, nil
	})
	token, err := u.completeTOTP(t.Context(), authResult{ModuleCodes: []string{"userAndOtp"}, RequestNumber: "2", AuthChainCode: "factor-chain"}, "student", icourseBase, "chain_type", "lck", time.Unix(59, 0))
	if err != nil || token != "ticket-token" {
		t.Fatalf("WebVPN completion returned %q, %v", token, err)
	}
}
