// Package localauth provides bearer-token authentication for local HTTP services.
//
// The middleware deliberately requires both a local Host and a bearer token. This
// keeps a token copied from a local configuration file from becoming a portable
// credential for a DNS-rebinding or cross-origin request.
package localauth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// TokenBytes is the number of random bytes used to create a token.
const TokenBytes = 32

var (
	// ErrEmptyToken reports an attempt to write an empty token.
	ErrEmptyToken = errors.New("localauth: token is empty")
	// ErrEmptyTokenFile reports a token file that contains no token.
	ErrEmptyTokenFile = errors.New("localauth: token file is empty")
)

// Options configures Middleware.
type Options struct {
	// Token is the expected bearer token. An empty token rejects every request.
	Token string
	// Port is used to build the default local Host and Origin allowlists.
	Port int
	// AllowedHosts replaces the default 127.0.0.1:<port> and localhost:<port>
	// allowlist when non-empty. Values must include their port.
	AllowedHosts []string
	// AllowedOrigins replaces the default http origins for the local hosts when
	// non-empty. Origins are compared as complete, exact origins.
	AllowedOrigins []string
}

// GenerateToken creates a cryptographically random, URL-safe bearer token.
func GenerateToken() (string, error) {
	buf := make([]byte, TokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("localauth: generate token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// ReadToken reads and trims a token from path. Token files are expected to
// contain one token and may end with a newline.
func ReadToken(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("localauth: read token %q: %w", path, err)
	}
	token := strings.TrimSpace(string(data))
	if token == "" {
		return "", ErrEmptyTokenFile
	}
	return token, nil
}

// WriteToken writes token with owner-only permissions. The parent directory is
// created with owner-only permissions when necessary. On Windows, mode bits do
// not express the complete ACL; callers should keep the file under the user's
// profile and rely on the profile ACL (see the package README).
func WriteToken(path, token string) error {
	if strings.TrimSpace(token) == "" {
		return ErrEmptyToken
	}
	if path == "" {
		return errors.New("localauth: token path is empty")
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("localauth: create token directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("localauth: open token file: %w", err)
	}
	defer file.Close()

	// Chmod also tightens an existing file whose permissions were changed after
	// the first write.
	if err := file.Chmod(0o600); err != nil {
		return fmt.Errorf("localauth: protect token file: %w", err)
	}
	if _, err := file.WriteString(token + "\n"); err != nil {
		return fmt.Errorf("localauth: write token file: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("localauth: sync token file: %w", err)
	}
	if err := verifyTokenFile(path); err != nil {
		return err
	}
	return nil
}

func verifyTokenFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("localauth: verify token file: %w", err)
	}
	// Windows ACLs are not represented by Unix mode bits. The documented
	// contract there is the user's profile ACL rather than this mode check.
	if os.PathSeparator != '\\' && info.Mode().Perm() != 0o600 {
		return fmt.Errorf("localauth: token file %q has permissions %o, want 600", path, info.Mode().Perm())
	}
	return nil
}

// Middleware authenticates requests and invokes next only for a valid local
// request. It returns 403 for a non-local Host or Origin and 401 for an invalid
// or missing bearer token.
func Middleware(options Options, next http.Handler) http.Handler {
	if next == nil {
		next = http.NotFoundHandler()
	}
	hosts := allowlist(options.AllowedHosts, options.Port, false)
	origins := allowlist(options.AllowedOrigins, options.Port, true)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !containsFold(hosts, r.Host) {
			unauthorizedHost(w)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" && !containsFold(origins, origin) {
			unauthorizedHost(w)
			return
		}

		presented, ok := ParseBearerToken(r.Header.Get("Authorization"))
		if !ok || !secureEqual(presented, options.Token) {
			w.Header().Set("WWW-Authenticate", `Bearer`)
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ParseBearerToken extracts a token from an Authorization header. The scheme
// comparison is case-insensitive; the token is returned verbatim apart from
// surrounding HTTP whitespace.
func ParseBearerToken(header string) (string, bool) {
	const prefix = "Bearer "
	if len(header) < len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return "", false
	}
	token := strings.TrimSpace(header[len(prefix):])
	return token, token != ""
}

func allowlist(explicit []string, port int, origin bool) []string {
	if len(explicit) > 0 {
		return append([]string(nil), explicit...)
	}
	if port <= 0 || port > 65535 {
		return nil
	}
	portText := strconv.Itoa(port)
	if origin {
		return []string{"http://127.0.0.1:" + portText, "http://localhost:" + portText}
	}
	return []string{"127.0.0.1:" + portText, "localhost:" + portText}
}

func containsFold(values []string, candidate string) bool {
	for _, value := range values {
		if strings.EqualFold(value, candidate) {
			return true
		}
	}
	return false
}

// secureEqual hashes first so ConstantTimeCompare always compares fixed-size
// values, including when an attacker supplies a token of a different length.
func secureEqual(left, right string) bool {
	leftHash := sha256.Sum256([]byte(left))
	rightHash := sha256.Sum256([]byte(right))
	return subtle.ConstantTimeCompare(leftHash[:], rightHash[:]) == 1
}

func unauthorizedHost(w http.ResponseWriter) {
	http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
}
