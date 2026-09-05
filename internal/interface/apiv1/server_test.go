package apiv1

import (
	"encoding/base64"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"finarch/internal/domain/model"
	"finarch/internal/domain/service"
	"finarch/internal/infrastructure/auth"

	"github.com/gin-gonic/gin"
)

func TestMapDomainError_UsernameTaken(t *testing.T) {
	status, payload := mapDomainError(service.ErrUsernameTaken)
	if status != http.StatusConflict {
		t.Fatalf("expected 409, got %d", status)
	}
	if payload.Code != "username_taken" {
		t.Fatalf("expected username_taken code, got %s", payload.Code)
	}
}

func TestMapDomainError_ExpiredToken(t *testing.T) {
	status, payload := mapDomainError(service.ErrExpiredToken)
	if status != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", status)
	}
	if payload.Code != "expired_token" {
		t.Fatalf("expected expired_token code, got %s", payload.Code)
	}
}

func TestMapDomainError_NotAuthorizedIsForbidden(t *testing.T) {
	status, payload := mapDomainError(service.ErrNotAuthorized)
	if status != http.StatusForbidden || payload.Code != "not_authorized" {
		t.Fatalf("not authorized mapping = (%d, %q), want (403, not_authorized)", status, payload.Code)
	}
}

func TestMapDomainError_InternalNeverLeaksSQL(t *testing.T) {
	status, payload := mapDomainError(errors.New("sql: no rows in result set"))
	if status != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", status)
	}
	if payload.Code != "internal_error" {
		t.Fatalf("expected internal_error code, got %s", payload.Code)
	}
	if payload.Message == "sql: no rows in result set" {
		t.Fatalf("raw SQL error must not be exposed")
	}
}

func TestRealIPIgnoresForwardedHeadersFromUntrustedPeer(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.10:4321"
	req.Header.Set("X-Forwarded-For", "198.51.100.77")
	req.Header.Set("X-Real-IP", "198.51.100.88")
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req

	if got := (&Server{}).realIP(c); got != "203.0.113.10" {
		t.Fatalf("realIP() = %q, want direct peer", got)
	}
}

func TestRealIPIgnoresForwardedHeadersWhenProxyModeIsDisabled(t *testing.T) {
	config, err := ParseProxyConfig("false", "")
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{}
	s.ConfigureProxy(config)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.10:4321"
	req.Header.Set("X-Forwarded-For", "198.51.100.77")
	req.Header.Set("X-Real-IP", "198.51.100.88")
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req

	if got := s.realIP(c); got != "203.0.113.10" {
		t.Fatalf("realIP() = %q, want direct peer while proxy mode is disabled", got)
	}
}

func TestRealIPUsesRightmostUntrustedAddressBehindConfiguredProxy(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.10:4321"
	// The leftmost address is attacker supplied. The edge appended the actual
	// client, and an internal proxy appended itself.
	req.Header.Set("X-Forwarded-For", "198.51.100.77, 203.0.113.25, 10.0.0.9")
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req

	config, err := ParseProxyConfig("true", "10.0.0.10,10.0.0.9")
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{}
	s.ConfigureProxy(config)
	if got := s.realIP(c); got != "203.0.113.25" {
		t.Fatalf("realIP() = %q, want rightmost untrusted hop", got)
	}
}

func TestRealIPAcceptsCanonicalProxyAddressForms(t *testing.T) {
	tests := []struct {
		name    string
		remote  string
		trusted string
		xff     string
		xRealIP string
		want    string
	}{
		{
			name:   "IPv4 host and port",
			remote: "10.0.0.10:4321", trusted: "10.0.0.10",
			xff: "203.0.113.25:443", want: "203.0.113.25",
		},
		{
			name:   "bracketed IPv6 host and port",
			remote: "10.0.0.10:4321", trusted: "10.0.0.10",
			xff: "[2001:db8::25]:443", want: "2001:db8::25",
		},
		{
			name:   "zoned IPv6 peer and intermediary",
			remote: "[fe80::10%eth0]:4321", trusted: "fe80::10,fe80::9",
			xff: "203.0.113.25, [fe80::9%eth1]:443", want: "203.0.113.25",
		},
		{
			name:   "IPv4-mapped IPv6 is canonicalized",
			remote: "[::ffff:10.0.0.10]:4321", trusted: "10.0.0.10",
			xff: "[::ffff:203.0.113.25]:443", want: "203.0.113.25",
		},
		{
			name:   "zoned X-Real-IP fallback",
			remote: "10.0.0.10:4321", trusted: "10.0.0.10",
			xRealIP: "[fe80::25%eth9]:443", want: "fe80::25",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := ParseProxyConfig("true", tt.trusted)
			if err != nil {
				t.Fatal(err)
			}
			s := &Server{}
			s.ConfigureProxy(config)
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tt.remote
			if tt.xff != "" {
				req.Header.Set("X-Forwarded-For", tt.xff)
			}
			if tt.xRealIP != "" {
				req.Header.Set("X-Real-IP", tt.xRealIP)
			}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = req
			if got := s.realIP(c); got != tt.want {
				t.Fatalf("realIP() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseForwardedIP(t *testing.T) {
	valid := map[string]string{
		" 192.0.2.1 ":            "192.0.2.1",
		"192.0.2.1:443":          "192.0.2.1",
		"2001:db8::1":            "2001:db8::1",
		"2001:db8::1:443":        "2001:db8::1:443",
		"[2001:db8::1]:443":      "2001:db8::1",
		"fe80::1%eth0":           "fe80::1",
		"[fe80::1%eth0]:443":     "fe80::1",
		"::ffff:192.0.2.1":       "192.0.2.1",
		"[::ffff:192.0.2.1]:443": "192.0.2.1",
	}
	for raw, want := range valid {
		ip, ok := parseForwardedIP(raw)
		if !ok || ip.String() != want {
			t.Errorf("parseForwardedIP(%q) = (%v, %v), want (%s, true)", raw, ip, ok, want)
		}
	}

	for _, raw := range []string{
		"",
		"unknown",
		"proxy.example:443",
		"192.0.2.1:not-a-port",
		"192.0.2.1:70000",
		"[2001:db8::1]",
		"[2001:db8::1]:not-a-port",
		`"192.0.2.1"`,
		"for=192.0.2.1",
	} {
		if ip, ok := parseForwardedIP(raw); ok {
			t.Errorf("parseForwardedIP(%q) unexpectedly accepted as %s", raw, ip)
		}
	}
}

func TestRealIPCombinesRepeatedXFFFieldsAsOneValidatedChain(t *testing.T) {
	config, err := ParseProxyConfig("true", "10.0.0.10,10.0.0.9")
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{}
	s.ConfigureProxy(config)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.10:4321"
	req.Header.Add("X-Forwarded-For", "198.51.100.77, 203.0.113.25")
	req.Header.Add("X-Forwarded-For", "10.0.0.9")
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req

	if got := s.realIP(c); got != "203.0.113.25" {
		t.Fatalf("realIP() = %q, want rightmost untrusted hop across repeated fields", got)
	}
}

func TestRealIPFailsClosedOnMalformedOrAmbiguousProxyHeaders(t *testing.T) {
	tests := []struct {
		name    string
		xff     []string
		xRealIP []string
	}{
		{name: "malformed middle hop", xff: []string{"198.51.100.77, 203.0.113.25:bad, 10.0.0.9"}},
		{name: "malformed left hop is still rejected", xff: []string{"unknown, 203.0.113.25, 10.0.0.9"}},
		{name: "empty hop", xff: []string{"198.51.100.77, , 10.0.0.9"}},
		{name: "malformed repeated XFF field", xff: []string{"203.0.113.25", "unknown"}},
		{name: "empty XFF does not fall through", xff: []string{""}, xRealIP: []string{"203.0.113.25"}},
		{name: "malformed X-Real-IP", xRealIP: []string{"203.0.113.25:bad"}},
		{name: "duplicate X-Real-IP fields", xRealIP: []string{"203.0.113.25", "198.51.100.77"}},
		{name: "all-trusted XFF does not mix headers", xff: []string{"10.0.0.9"}, xRealIP: []string{"198.51.100.77"}},
	}

	config, err := ParseProxyConfig("true", "10.0.0.10,10.0.0.9")
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{}
	s.ConfigureProxy(config)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = "10.0.0.10:4321"
			for _, value := range tt.xff {
				req.Header.Add("X-Forwarded-For", value)
			}
			for _, value := range tt.xRealIP {
				req.Header.Add("X-Real-IP", value)
			}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = req
			if got := s.realIP(c); got != "10.0.0.10" {
				t.Fatalf("realIP() = %q, want fail-closed direct peer", got)
			}
		})
	}
}

func TestRealIPCanonicalizesUntrustedZonedIPv6Peer(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "[fe80::1%eth0]:49152"
	req.Header.Set("X-Forwarded-For", "198.51.100.77")
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req

	if got := (&Server{}).realIP(c); got != "fe80::1" {
		t.Fatalf("realIP() = %q, want stable address without zone or source port", got)
	}
}

func TestRealIPUsesStableFailClosedRateLimitKeys(t *testing.T) {
	tests := []struct {
		name    string
		server  *Server
		remotes []string
		xffs    []string
	}{
		{
			name:    "IPv6 zone and source port are not part of key",
			server:  &Server{},
			remotes: []string{"[fe80::1%eth0]:49152", "[fe80::1%eth1]:58321"},
			xffs:    []string{"198.51.100.1", "198.51.100.2"},
		},
		{
			name:    "IPv4 and mapped IPv6 share canonical key",
			server:  &Server{},
			remotes: []string{"192.0.2.1:49152", "[::ffff:192.0.2.1]:58321"},
			xffs:    []string{"198.51.100.1", "198.51.100.2"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limiter := auth.NewIPRateLimiter(1, time.Minute)
			for i := range tt.remotes {
				req := httptest.NewRequest(http.MethodGet, "/", nil)
				req.RemoteAddr = tt.remotes[i]
				req.Header.Set("X-Forwarded-For", tt.xffs[i])
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = req
				allowed := limiter.Allow(tt.server.realIP(c))
				if allowed != (i == 0) {
					t.Fatalf("request %d allowed = %v, want %v", i+1, allowed, i == 0)
				}
			}
		})
	}
}

func TestRealIPUsesSharedKeyForUnparseableDirectPeers(t *testing.T) {
	limiter := auth.NewIPRateLimiter(1, time.Minute)
	for i, remote := range []string{"unparseable-peer:49152", "unparseable-peer:58321"} {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = remote
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = req
		if key := (&Server{}).realIP(c); key != "" {
			t.Fatalf("realIP() key = %q, want conservative empty key", key)
		}
		if allowed := limiter.Allow((&Server{}).realIP(c)); allowed != (i == 0) {
			t.Fatalf("request %d allowed = %v, want %v", i+1, allowed, i == 0)
		}
	}
}

func TestParseProxyConfigDefaultsToDirectPeerOnly(t *testing.T) {
	config, err := ParseProxyConfig("", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(config.trustedProxyNets) != 0 {
		t.Fatalf("trusted networks = %v, want none", config.trustedProxyNets)
	}
}

func TestParseProxyConfigRequiresExplicitValidTrustBoundary(t *testing.T) {
	tests := []struct {
		name   string
		behind string
		cidrs  string
	}{
		{name: "invalid bool", behind: "tru", cidrs: "10.0.0.1"},
		{name: "uppercase bool rejected", behind: "TRUE", cidrs: "10.0.0.1"},
		{name: "missing proxies", behind: "true"},
		{name: "cidrs without opt in", behind: "false", cidrs: "10.0.0.1"},
		{name: "invalid address", behind: "true", cidrs: "not-an-ip"},
		{name: "invalid cidr", behind: "true", cidrs: "10.0.0.0/99"},
		{name: "empty list entry", behind: "true", cidrs: "10.0.0.1,,10.0.0.2"},
		{name: "trust all IPv4", behind: "true", cidrs: "0.0.0.0/0"},
		{name: "trust all IPv6", behind: "true", cidrs: "::/0"},
		{name: "mapped trust all IPv4", behind: "true", cidrs: "::ffff:0:0/96"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ParseProxyConfig(tt.behind, tt.cidrs); err == nil {
				t.Fatal("ParseProxyConfig() error = nil")
			}
		})
	}
}

func TestParseProxyConfigAcceptsExplicitIPsAndCIDRs(t *testing.T) {
	config, err := ParseProxyConfig("true", "127.0.0.1, 10.0.0.0/8, ::1")
	if err != nil {
		t.Fatal(err)
	}
	if len(config.trustedProxyNets) != 3 {
		t.Fatalf("trusted networks = %v, want 3", config.trustedProxyNets)
	}
}

func TestOperationsAccessRequiresExplicitStrongSecret(t *testing.T) {
	t.Setenv(operationsEnabledEnv, "true")
	t.Setenv(operationsSecretEnv, "too-short")
	if enabled, _ := loadOperationsAccess(); enabled {
		t.Fatal("short operations secret must keep system operations disabled")
	}

	t.Setenv(operationsSecretEnv, "0123456789abcdef0123456789abcdef")
	if enabled, _ := loadOperationsAccess(); !enabled {
		t.Fatal("explicit enable plus strong independent secret should enable operations")
	}
}

func TestCORSIsDeniedByDefault(t *testing.T) {
	r := gin.New()
	r.Use((&Server{}).corsMiddleware())
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "https://attacker.example")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("same request handling status = %d, want 200", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("unexpected cross-origin allowance %q", got)
	}
	if got := w.Header().Get("Vary"); got != "Origin" {
		t.Fatalf("Vary = %q, want Origin", got)
	}
	if got := w.Header().Get("Access-Control-Allow-Credentials"); got != "" {
		t.Fatalf("credentials header must be omitted, got %q", got)
	}
}

func TestCORSReflectsOnlyExplicitlyAllowedOrigin(t *testing.T) {
	s := &Server{corsAllowedOrigins: parseCORSAllowedOrigins("https://app.example")}
	r := gin.New()
	r.Use(s.corsMiddleware())

	allowedReq := httptest.NewRequest(http.MethodOptions, "/api/v1/auth/login", nil)
	allowedReq.Header.Set("Origin", "https://app.example")
	allowed := httptest.NewRecorder()
	r.ServeHTTP(allowed, allowedReq)
	if allowed.Code != http.StatusNoContent {
		t.Fatalf("allowed preflight status = %d, want 204", allowed.Code)
	}
	if got := allowed.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example" {
		t.Fatalf("allowed origin = %q", got)
	}
	if got := allowed.Header().Get("Access-Control-Allow-Credentials"); got != "" {
		t.Fatalf("credentials header must be omitted, got %q", got)
	}

	deniedReq := httptest.NewRequest(http.MethodOptions, "/api/v1/auth/login", nil)
	deniedReq.Header.Set("Origin", "https://attacker.example")
	denied := httptest.NewRecorder()
	r.ServeHTTP(denied, deniedReq)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("denied preflight status = %d, want 403", denied.Code)
	}
	if got := denied.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("denied origin was reflected as %q", got)
	}
}

func TestCanonicalCORSOriginNormalizesDefaultPortsAndIPv6(t *testing.T) {
	tests := map[string]string{
		"https://APP.example:443/":  "https://app.example",
		"http://APP.example:80":     "http://app.example",
		"https://app.example:8443":  "https://app.example:8443",
		"https://[2001:db8::1]:443": "https://[2001:db8::1]",
		"http://[2001:db8::1]:8080": "http://[2001:db8::1]:8080",
	}
	for raw, want := range tests {
		got, ok := canonicalCORSOrigin(raw)
		if !ok || got != want {
			t.Fatalf("canonicalCORSOrigin(%q) = (%q, %v), want (%q, true)", raw, got, ok, want)
		}
	}
	for _, raw := range []string{"https://user@app.example", "https://app.example/path", "https://[fe80::1%25eth0]"} {
		if got, ok := canonicalCORSOrigin(raw); ok {
			t.Fatalf("canonicalCORSOrigin(%q) unexpectedly accepted as %q", raw, got)
		}
	}
}

func TestSessionOriginRequiresExactSecureOrigin(t *testing.T) {
	s := &Server{sessionCookieSecure: true}
	router := gin.New()
	router.POST("/session", s.sessionOriginMiddleware(), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	tests := []struct {
		name       string
		origins    []string
		fetchSite  string
		wantStatus int
	}{
		{name: "exact", origins: []string{"https://app.example"}, fetchSite: "same-origin", wantStatus: http.StatusNoContent},
		{name: "missing", wantStatus: http.StatusForbidden},
		{name: "wrong scheme", origins: []string{"http://app.example"}, wantStatus: http.StatusForbidden},
		{name: "different subdomain", origins: []string{"https://other.app.example"}, wantStatus: http.StatusForbidden},
		{name: "duplicate", origins: []string{"https://app.example", "https://app.example"}, wantStatus: http.StatusForbidden},
		{name: "cross site metadata", origins: []string{"https://app.example"}, fetchSite: "cross-site", wantStatus: http.StatusForbidden},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "https://app.example/session", nil)
			for _, origin := range tc.origins {
				req.Header.Add("Origin", origin)
			}
			if tc.fetchSite != "" {
				req.Header.Set("Sec-Fetch-Site", tc.fetchSite)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if response.Code != tc.wantStatus {
				t.Fatalf("status = %d body=%s, want %d", response.Code, response.Body.String(), tc.wantStatus)
			}
		})
	}
}

func TestLoopbackDevelopmentMayAcceptOriginlessSessionRequest(t *testing.T) {
	s := &Server{allowOriginlessSessionRoutes: true}
	router := gin.New()
	router.POST("/session", s.sessionOriginMiddleware(), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "http://127.0.0.1/session", nil))
	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d body=%s, want %d", response.Code, response.Body.String(), http.StatusNoContent)
	}
}

func TestSecureRefreshCookieAttributes(t *testing.T) {
	s := &Server{sessionCookieSecure: true}
	response := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(response)
	c.Request = httptest.NewRequest(http.MethodPost, "https://app.example/api/v1/auth/login", nil)
	s.setRefreshCookie(c, strings.Repeat("a", 43), time.Now().Add(time.Hour))

	cookies := response.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("Set-Cookie count = %d, want 1", len(cookies))
	}
	cookie := cookies[0]
	if cookie.Name != secureRefreshCookieName || !cookie.HttpOnly || !cookie.Secure ||
		cookie.SameSite != http.SameSiteStrictMode || cookie.Path != "/" || cookie.MaxAge <= 0 {
		t.Fatalf("unsafe refresh cookie: %#v", cookie)
	}
	if got := response.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
}

func TestActionTokenPagesDoNotLeakOrCacheLegacyQueryTokens(t *testing.T) {
	router := gin.New()
	router.Use((&Server{}).securityHeaders())
	router.GET("/*path", func(c *gin.Context) { c.Status(http.StatusOK) })

	for _, path := range []string{
		"/verify-email?token=legacy-secret",
		"/reset-password?token=legacy-secret",
		"/confirm-delete-account?token=legacy-secret",
		"/confirm-email-change-old?token=legacy-secret",
		"/confirm-email-change?token=legacy-secret",
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if got := response.Header().Get("Referrer-Policy"); got != "no-referrer" {
			t.Errorf("%s Referrer-Policy = %q, want no-referrer", path, got)
		}
		if got := response.Header().Get("Cache-Control"); got != "no-store" {
			t.Errorf("%s Cache-Control = %q, want no-store", path, got)
		}
	}
}

func TestEmailVerificationGETIsReadOnly(t *testing.T) {
	for _, path := range []string{"/verify-email?token=legacy", "/api/v1/auth/verify-email?token=legacy"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		if requestRequiresWriteLease(request) {
			t.Errorf("GET %s unexpectedly requires a write lease", path)
		}
	}
}

func TestCanonicalRefreshTokenValidation(t *testing.T) {
	valid := base64.RawURLEncoding.EncodeToString(make([]byte, refreshTokenDecodedBytes))
	if len(valid) != refreshTokenEncodedBytes || !isCanonicalRefreshToken(valid) {
		t.Fatalf("generated refresh token was rejected: %q", valid)
	}
	for _, raw := range []string{
		"",
		strings.Repeat("A", refreshTokenEncodedBytes-1),
		strings.Repeat("!", refreshTokenEncodedBytes),
		strings.Repeat("A", refreshTokenEncodedBytes-1) + "B",
		valid + "=",
	} {
		if isCanonicalRefreshToken(raw) {
			t.Fatalf("invalid refresh token accepted: %q", raw)
		}
	}
}

func TestSessionRateLimitsArePerRouteAndPerCanonicalToken(t *testing.T) {
	s := &Server{
		refreshIPLimiter:    auth.NewIPRateLimiter(100, time.Minute),
		refreshTokenLimiter: auth.NewIPRateLimiter(1, time.Minute),
		logoutIPLimiter:     auth.NewIPRateLimiter(100, time.Minute),
		logoutTokenLimiter:  auth.NewIPRateLimiter(1, time.Minute),
	}
	router := gin.New()
	router.POST("/refresh", s.refreshRateLimitMiddleware(), func(c *gin.Context) { c.Status(http.StatusNoContent) })
	router.POST("/logout", s.logoutRateLimitMiddleware(), func(c *gin.Context) { c.Status(http.StatusNoContent) })

	serve := func(path, cookieValue string) int {
		req := httptest.NewRequest(http.MethodPost, path, nil)
		if cookieValue != "" {
			req.AddCookie(&http.Cookie{Name: loopbackRefreshCookieName, Value: cookieValue})
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		return response.Code
	}

	for i := 0; i < 5; i++ {
		if got := serve("/refresh", strings.Repeat("!", refreshTokenEncodedBytes)); got != http.StatusNoContent {
			t.Fatalf("malformed request %d status = %d", i, got)
		}
	}
	first := base64.RawURLEncoding.EncodeToString(make([]byte, refreshTokenDecodedBytes))
	secondBytes := make([]byte, refreshTokenDecodedBytes)
	secondBytes[0] = 1
	second := base64.RawURLEncoding.EncodeToString(secondBytes)
	if got := serve("/refresh", first); got != http.StatusNoContent {
		t.Fatalf("first canonical refresh status = %d", got)
	}
	if got := serve("/refresh", first); got != http.StatusTooManyRequests {
		t.Fatalf("repeated canonical refresh status = %d, want 429", got)
	}
	if got := serve("/refresh", second); got != http.StatusNoContent {
		t.Fatalf("different token inherited another token budget: status=%d", got)
	}
	if got := serve("/logout", first); got != http.StatusNoContent {
		t.Fatalf("logout inherited refresh route budget: status=%d", got)
	}
	if got := serve("/logout", first); got != http.StatusTooManyRequests {
		t.Fatalf("repeated logout status = %d, want 429", got)
	}
}

func TestOrdinaryBodyLimitPreservesDedicatedMultipartUploadLimits(t *testing.T) {
	s := &Server{}
	router := gin.New()
	router.Use(s.jsonRequestBodyLimitMiddleware())
	for _, route := range []string{
		"/api/v1/attachments",
		"/api/v1/transactions/:id/attachments",
		"/api/v1/backup/restore",
		"/api/v1/backup/restore/send-verification",
	} {
		router.POST(route, func(c *gin.Context) { c.Status(http.StatusNoContent) })
	}

	for _, path := range []string{
		"/api/v1/attachments",
		"/api/v1/transactions/tx-1/attachments",
		"/api/v1/backup/restore",
		"/api/v1/backup/restore/send-verification",
	} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(strings.Repeat("x", jsonRequestBodyMaxBytes+1)))
			req.Header.Set("Content-Type", "multipart/form-data; boundary=test")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if response.Code != http.StatusNoContent {
				t.Fatalf("dedicated upload route was capped by ordinary body limit: status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}

func TestLoopbackBindAddress(t *testing.T) {
	for _, addr := range []string{"127.0.0.1:8080", "[::1]:8080", "localhost:8080"} {
		if !isLoopbackBindAddress(addr) {
			t.Fatalf("%q should be loopback", addr)
		}
	}
	for _, addr := range []string{":8080", "0.0.0.0:8080", "[::]:8080", "example.com:8080"} {
		if isLoopbackBindAddress(addr) {
			t.Fatalf("%q must not be treated as loopback", addr)
		}
	}
}

func TestScopedIdempotencyKeySeparatesUsersAndEndpoints(t *testing.T) {
	const rawKey = "client-request-123"
	base := scopedIdempotencyKey("user-a", "POST /api/v1/transactions", rawKey)
	if base == scopedIdempotencyKey("user-b", "POST /api/v1/transactions", rawKey) {
		t.Fatal("same client key collided across users")
	}
	if base == scopedIdempotencyKey("user-a", "POST /api/v1/reimbursements", rawKey) {
		t.Fatal("same client key collided across endpoints")
	}
	if base == rawKey {
		t.Fatal("raw client key must not be persisted directly")
	}
}

func TestNormalizeTagIDsMatchesSetSemantics(t *testing.T) {
	got := normalizeTagIDs([]string{"tag-b", "tag-a", "tag-b", "tag-a"})
	if len(got) != 2 || got[0] != "tag-a" || got[1] != "tag-b" {
		t.Fatalf("normalizeTagIDs() = %#v, want [tag-a tag-b]", got)
	}
}

func TestLegacyAmountCanonicalizationUsesFinancialRounding(t *testing.T) {
	first, err := createTransactionRequestHash(createTransactionRequest{AmountYuan: 0.28}, model.TxTypeExpense, nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := createTransactionRequestHash(createTransactionRequest{AmountYuan: 0.29}, model.TxTypeExpense, nil)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("financially distinct legacy amounts produced the same idempotency hash")
	}
	if cents, err := budgetAmountCents(0, 1.005); err != nil || cents != 101 {
		t.Fatalf("budgetAmountCents(1.005) = (%d, %v), want (101, nil)", cents, err)
	}
	if _, err := budgetAmountCents(0, math.MaxFloat64); err == nil {
		t.Fatal("out-of-range budget amount was accepted")
	}
}

func TestRecurringJSONTracksExplicitZeroAmount(t *testing.T) {
	zero := 0.0
	provided, err := recurringRequestFromJSON("user", "rule", recurringRuleJSONRequest{AmountYuan: &zero})
	if err != nil {
		t.Fatal(err)
	}
	if !provided.AmountProvided {
		t.Fatal("explicit zero amount was treated as an omitted PATCH field")
	}
	omitted, err := recurringRequestFromJSON("user", "rule", recurringRuleJSONRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if omitted.AmountProvided {
		t.Fatal("omitted amount was treated as explicitly provided")
	}
}
