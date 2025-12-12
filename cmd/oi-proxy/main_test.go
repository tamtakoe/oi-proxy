package main

import (
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"testing"
)

func TestRewriteCookieDomain(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		replacement string
		want        string
	}{
		{
			name:        "rewrite existing domain",
			input:       "session=abc; Domain=example.org; Path=/",
			replacement: "proxy.test",
			want:        "session=abc; Domain=proxy.test; Path=/",
		},
		{
			name:        "append when domain absent",
			input:       "session=abc; Path=/",
			replacement: "proxy.test",
			want:        "session=abc; Path=/; Domain=proxy.test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := rewriteCookieDomain(tt.input, tt.replacement)
			if !strings.Contains(got, "Domain="+tt.replacement) {
				t.Fatalf("rewriteCookieDomain(%q, %q) = %q, expected Domain=%s", tt.input, tt.replacement, got, tt.replacement)
			}
			if !strings.Contains(got, "session=abc") || !strings.Contains(got, "Path=/") {
				t.Fatalf("rewriteCookieDomain(%q, %q) missing base attributes: %q", tt.input, tt.replacement, got)
			}
		})
	}
}

func TestDetermineAllowedOrigin(t *testing.T) {
	cfg := &config{corsOrigin: "https://forced.example"}
	if got := determineAllowedOrigin(nil, cfg); got != "https://forced.example" {
		t.Fatalf("expected forced origin, got %q", got)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", " https://from-origin.test ")
	cfg.corsOrigin = ""

	if got := determineAllowedOrigin(req, cfg); got != "https://from-origin.test" {
		t.Fatalf("expected Origin header, got %q", got)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.Header.Set("Referer", " https://from-referer.test/path ")

	if got := determineAllowedOrigin(req2, cfg); got != "https://from-referer.test" {
		t.Fatalf("expected Referer host, got %q", got)
	}
}

func TestProxy_RewritesCookieAndOriginAndStripPrefix(t *testing.T) {
	var (
		gotPath string
		gotHost string
		mu      sync.Mutex
		locRewritten bool
	)

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotPath = r.URL.Path
		gotHost = r.Host
		mu.Unlock()

		w.Header().Add("Set-Cookie", "a=1; Domain=example.org; Path=/")
		w.Header().Set("Location", "https://"+r.Host+"/redirect")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(target.Close)

	targetURL, err := url.Parse(target.URL)
	if err != nil {
		t.Fatalf("parse target: %v", err)
	}

	cfg := &config{
		host:         "localhost",
		port:         8080,
		targetURL:    targetURL,
		cookieDomain: "proxy.test",
		stripPrefix:  "/api",
		insecureTLS:  false,
		corsOrigin:   "",
		corsHeaders:  "X-Test-Header",
		corsMethods:  "GET,POST",
		replaceOld:   "",
		replaceNew:   "localhost:8080",
	}
	if cfg.replaceNew == "" {
		t.Fatalf("replaceNew must be set in test")
	}

	proxy := httputil.NewSingleHostReverseProxy(cfg.targetURL)
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		if cfg.stripPrefix != "" && strings.HasPrefix(req.URL.Path, cfg.stripPrefix) {
			req.URL.Path = strings.TrimPrefix(req.URL.Path, cfg.stripPrefix)
			if req.URL.Path == "" {
				req.URL.Path = "/"
			}
		}
		req.Host = cfg.targetURL.Host
	}

	proxy.ModifyResponse = func(resp *http.Response) error {
		if cookies := resp.Header["Set-Cookie"]; len(cookies) > 0 {
			rewritten := make([]string, len(cookies))
			for i, raw := range cookies {
				rewritten[i] = rewriteCookieDomain(raw, cfg.cookieDomain)
			}
			resp.Header["Set-Cookie"] = rewritten
		}
		if allowed := determineAllowedOrigin(resp.Request, cfg); allowed != "" {
			resp.Header.Set("Access-Control-Allow-Origin", allowed)
			resp.Header.Set("Access-Control-Allow-Credentials", "true")
			if cfg.corsHeaders != "" {
				resp.Header.Set("Access-Control-Allow-Headers", cfg.corsHeaders)
			} else if resp.Header.Get("Access-Control-Allow-Headers") == "" {
				resp.Header.Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With")
			}
			if cfg.corsMethods != "" {
				resp.Header.Set("Access-Control-Allow-Methods", cfg.corsMethods)
			} else if resp.Header.Get("Access-Control-Allow-Methods") == "" {
				resp.Header.Set("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
			}
		}
		if cfg.replaceOld != "" || cfg.targetURL != nil {
			if loc := resp.Header.Get("Location"); loc != "" {
				if parsed, err := url.Parse(loc); err == nil {
					if parsed.Host != "" && cfg.replaceNew != "" {
						parsed.Host = cfg.replaceNew
						resp.Header.Set("Location", parsed.String())
						locRewritten = true
						return nil
					}
					if cfg.replaceOld != "" && strings.Contains(loc, cfg.replaceOld) {
						resp.Header.Set("Location", strings.ReplaceAll(loc, cfg.replaceOld, cfg.replaceNew))
						locRewritten = true
					}
				} else if cfg.replaceOld != "" && strings.Contains(loc, cfg.replaceOld) {
					resp.Header.Set("Location", strings.ReplaceAll(loc, cfg.replaceOld, cfg.replaceNew))
					locRewritten = true
				}
			}
		}
		return nil
	}

	server := httptest.NewServer(loggingMiddleware(proxy))
	t.Cleanup(server.Close)

	req, err := http.NewRequest(http.MethodGet, server.URL+"/api/hello", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Origin", "https://caller.test")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("proxy request failed: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}

	mu.Lock()
	defer mu.Unlock()
	if gotPath != "/hello" {
		t.Fatalf("target path = %q, want %q", gotPath, "/hello")
	}
	if gotHost != targetURL.Host {
		t.Fatalf("target host = %q, want %q", gotHost, targetURL.Host)
	}

	if origin := resp.Header.Get("Access-Control-Allow-Origin"); origin != "https://caller.test" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want %q", origin, "https://caller.test")
	}
	if allowHeaders := resp.Header.Get("Access-Control-Allow-Headers"); allowHeaders != "X-Test-Header" {
		t.Fatalf("Access-Control-Allow-Headers = %q, want %q", allowHeaders, "X-Test-Header")
	}
	if allowMethods := resp.Header.Get("Access-Control-Allow-Methods"); allowMethods != "GET,POST" {
		t.Fatalf("Access-Control-Allow-Methods = %q, want %q", allowMethods, "GET,POST")
	}
	if loc := resp.Header.Get("Location"); loc != "https://localhost:8080/redirect" {
		t.Fatalf("Location = %q, want %q", loc, "https://localhost:8080/redirect")
	}
	if !locRewritten {
		t.Fatalf("Location was not rewritten")
	}

	if cookie := resp.Header.Get("Set-Cookie"); !strings.Contains(cookie, "Domain=proxy.test") {
		t.Fatalf("Set-Cookie = %q, expected Domain=proxy.test", cookie)
	}
}

