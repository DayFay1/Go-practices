package api

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
)

func TestRealIPMiddleware_IgnoresForwardedForWhenProxyUntrusted(t *testing.T) {
	mw := RealIPMiddleware([]netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")})
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.RemoteAddr))
	}))

	req := httptest.NewRequest(http.MethodGet, "http://example.com", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("X-Forwarded-For", "203.0.113.10")

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if got := rr.Body.String(); got != "127.0.0.1:1234" {
		t.Fatalf("expected RemoteAddr to stay unchanged, got %q", got)
	}
}

func TestRealIPMiddleware_UsesForwardedForWhenProxyTrusted(t *testing.T) {
	mw := RealIPMiddleware([]netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")})
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.RemoteAddr))
	}))

	req := httptest.NewRequest(http.MethodGet, "http://example.com", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("X-Forwarded-For", "203.0.113.10")

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if got := rr.Body.String(); got != "203.0.113.10" {
		t.Fatalf("expected RemoteAddr to be set from X-Forwarded-For, got %q", got)
	}
}

func TestRealIPMiddleware_UsesRealIPHeaderWhenProxyTrusted(t *testing.T) {
	mw := RealIPMiddleware([]netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")})
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.RemoteAddr))
	}))

	req := httptest.NewRequest(http.MethodGet, "http://example.com", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("X-Real-IP", "198.51.100.7")

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if got := rr.Body.String(); got != "198.51.100.7" {
		t.Fatalf("expected RemoteAddr to be set from X-Real-IP, got %q", got)
	}
}
