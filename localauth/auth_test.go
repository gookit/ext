package localauth_test

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"testing"

	"github.com/gookit/ext/localauth"
	"github.com/gookit/goutil/x/assert"
)

func TestGenerateToken(t *testing.T) {
	token, err := localauth.GenerateToken()
	assert.NoError(t, err)
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	assert.NoError(t, err)
	assert.Len(t, decoded, localauth.TokenBytes)
	assert.NotEmpty(t, token)
}

func TestTokenFile(t *testing.T) {
	dir := t.TempDir()
	path := dir + string(os.PathSeparator) + "auth" + string(os.PathSeparator) + "token"

	assert.NoError(t, localauth.WriteToken(path, "secret-token"))
	got, err := localauth.ReadToken(path)
	assert.NoError(t, err)
	assert.Equal(t, "secret-token", got)

	info, err := os.Stat(path)
	assert.NoError(t, err)
	if runtime.GOOS != "windows" {
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}

	assert.Error(t, localauth.WriteToken(path, " "))
	assert.Error(t, localauth.WriteToken("", "token"))
}

func TestParseBearerToken(t *testing.T) {
	cases := []struct {
		name   string
		header string
		want   string
		ok     bool
	}{
		{name: "valid", header: "Bearer token", want: "token", ok: true},
		{name: "case insensitive scheme", header: "bEaReR token", want: "token", ok: true},
		{name: "surrounding whitespace", header: "Bearer  token ", want: "token", ok: true},
		{name: "missing", header: "", ok: false},
		{name: "basic", header: "Basic token", ok: false},
		{name: "empty", header: "Bearer ", ok: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := localauth.ParseBearerToken(tc.header)
			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.ok, ok)
		})
	}
}

func TestMiddleware(t *testing.T) {
	const (
		token = "secret-token"
		port  = 43127
	)
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	})
	handler := localauth.Middleware(localauth.Options{Token: token, Port: port}, next)

	request := func(host, origin, auth string, cookie ...bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, "http://"+host+"/status?token="+token, nil)
		r.Host = host
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		if auth != "" {
			r.Header.Set("Authorization", auth)
		}
		if len(cookie) > 0 && cookie[0] {
			r.AddCookie(&http.Cookie{Name: "token", Value: token})
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}

	t.Run("valid local request", func(t *testing.T) {
		called = false
		w := request("127.0.0.1:43127", "", "Bearer "+token)
		assert.Equal(t, http.StatusNoContent, w.Code)
		assert.True(t, called)
	})

	t.Run("valid localhost origin", func(t *testing.T) {
		called = false
		w := request("localhost:43127", "http://localhost:43127", "Bearer "+token)
		assert.Equal(t, http.StatusNoContent, w.Code)
		assert.True(t, called)
	})

	cases := []struct {
		name   string
		host   string
		origin string
		auth   string
		status int
		cookie bool
	}{
		{name: "missing token", host: "127.0.0.1:43127", status: http.StatusUnauthorized},
		{name: "query token is ignored", host: "127.0.0.1:43127", auth: "", status: http.StatusUnauthorized},
		{name: "cookie token is ignored", host: "127.0.0.1:43127", cookie: true, status: http.StatusUnauthorized},
		{name: "wrong token", host: "127.0.0.1:43127", auth: "Bearer wrong", status: http.StatusUnauthorized},
		{name: "wrong scheme", host: "127.0.0.1:43127", auth: "Basic " + token, status: http.StatusUnauthorized},
		{name: "wrong host", host: "127.0.0.1:43128", auth: "Bearer " + token, status: http.StatusForbidden},
		{name: "wrong origin", host: "127.0.0.1:43127", origin: "http://evil.example:43127", auth: "Bearer " + token, status: http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			called = false
			w := request(tc.host, tc.origin, tc.auth, tc.cookie)
			assert.Equal(t, tc.status, w.Code)
			assert.False(t, called)
		})
	}
}

func TestMiddlewareCustomAllowlist(t *testing.T) {
	called := false
	handler := localauth.Middleware(localauth.Options{
		Token:          "token",
		AllowedHosts:   []string{"127.0.0.1:0"},
		AllowedOrigins: []string{"https://app.example"},
	}, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	r := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:0/", nil)
	r.Host = "127.0.0.1:0"
	r.Header.Set("Origin", "https://app.example")
	r.Header.Set("Authorization", "Bearer token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.True(t, called)
}
