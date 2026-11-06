package main

import (
	"bufio"
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/hex"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const webVPNBase = "https://webvpn.fudan.edu.cn"

// The WebVPN gateway names a campus host by its AES-128-CFB ciphertext
// under a fixed public key and IV.
var vpnKey = []byte("wrdvpnisthebest!")

// vpnHost returns the gateway path segment for a hostname.
func vpnHost(host string) string {
	block, _ := aes.NewCipher(vpnKey)
	// The gateway pads the plaintext to whole 16-byte blocks before CFB and
	// keeps only as many ciphertext bytes as the host has.
	n := len(host)
	padded := []byte(host)
	if r := n % 16; r != 0 {
		padded = append(padded, bytes.Repeat([]byte{'0'}, 16-r)...)
	}
	out := make([]byte, len(padded))
	cipher.NewCFBEncrypter(block, vpnKey).XORKeyStream(out, padded)
	return hex.EncodeToString(vpnKey) + hex.EncodeToString(out[:n])
}

// vpnURL rewrites a campus URL to go through WebVPN.
func vpnURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	proto := u.Scheme
	if p := u.Port(); p != "" {
		proto += "-" + p
	}
	out := webVPNBase + "/" + proto + "/" + vpnHost(u.Hostname()) + u.EscapedPath()
	if u.RawQuery != "" {
		out += "?" + u.RawQuery
	}
	return out, nil
}

// vpnSignedOut recognizes gateway redirects and HTML sign-in responses. The
// course-detail API labels successful JSON as text/html, so the header alone
// cannot establish that a session expired.
func vpnSignedOut(resp *http.Response) bool {
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden ||
		!strings.HasPrefix(resp.Request.URL.Path, "/http") {
		return true
	}
	if !strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/html") {
		return false
	}
	if resp.Body == nil {
		return true
	}
	original := resp.Body
	reader := bufio.NewReader(original)
	// Peek leaves the bytes for the JSON decoder; keep the original closer
	// so the caller releases the connection as usual.
	resp.Body = struct {
		io.Reader
		io.Closer
	}{reader, original}
	prefix, _ := reader.Peek(512)
	prefix = bytes.TrimSpace(prefix)
	// JSON-looking responses go on to normal decoding and API-code checks;
	// malformed JSON must not become a misleading "session expired" error.
	return len(prefix) == 0 || (prefix[0] != '{' && prefix[0] != '[')
}
