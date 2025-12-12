package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type config struct {
	host         string
	port         int
	targetURL    *url.URL
	cookieDomain string
	stripPrefix  string
	insecureTLS  bool
	corsOrigin   string
	corsHeaders  string
	corsMethods  string
	replaceOld   string
	replaceNew   string
}

func parseFlags() (*config, error) {
	var (
		host         = flag.String("host", "localhost", "Host interface to listen on")
		port         = flag.Int("port", 80, "Port to listen on")
		target       = flag.String("target", "", "Target base URL (required)")
		cookieDomain = flag.String("cookie-domain", "", "Override Domain attribute in Set-Cookie headers (defaults to host)")
		stripPrefix  = flag.String("strip-prefix", "", "Prefix to remove from incoming request paths")
		insecure     = flag.Bool("insecure", false, "Disable TLS verification when proxying HTTPS targets")
		corsOrigin   = flag.String("cors-allow-origin", "", "Override Access-Control-Allow-Origin header (empty = disable override)")
		corsHeaders  = flag.String("cors-allow-headers", "", "Override Access-Control-Allow-Headers header (empty = use default)")
		corsMethods  = flag.String("cors-allow-methods", "", "Override Access-Control-Allow-Methods header (empty = use default)")
		replaceLoc   = flag.String("replace-location", "", `Replace domain in Location header: "old:new". If old empty, uses target host; if new empty, uses host:port. Example ":" -> replace target host with local host:port.`)
	)

	flag.Parse()

	if *target == "" {
		return nil, fmt.Errorf("target URL is required")
	}

	parsed, err := url.Parse(*target)
	if err != nil {
		return nil, fmt.Errorf("invalid target URL: %w", err)
	}

	cfg := &config{
		host:      *host,
		port:      *port,
		targetURL: parsed,
		cookieDomain: func() string {
			if *cookieDomain != "" {
				return *cookieDomain
			}
			return *host
		}(),
		stripPrefix: strings.TrimRight(*stripPrefix, "/"),
		insecureTLS: *insecure,
		corsOrigin:  *corsOrigin,
		corsHeaders: *corsHeaders,
		corsMethods: *corsMethods,
	}

	if *replaceLoc != "" {
		parts := strings.SplitN(*replaceLoc, ":", 2)
		if len(parts) > 0 {
			cfg.replaceOld = parts[0]
		}
		if len(parts) == 2 {
			cfg.replaceNew = parts[1]
		}
		if cfg.replaceOld == "" {
			cfg.replaceOld = parsed.Host
		}
		if cfg.replaceNew == "" {
			cfg.replaceNew = net.JoinHostPort(*host, strconv.Itoa(*port))
		}
	}

	return cfg, nil
}

func main() {
	cfg, err := parseFlags()
	if err != nil {
		log.Fatal(err)
	}

	proxy := httputil.NewSingleHostReverseProxy(cfg.targetURL)

	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: cfg.insecureTLS, // #nosec G402
		},
	}
	proxy.Transport = transport

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
						return nil
					}
					if cfg.replaceOld != "" && strings.Contains(loc, cfg.replaceOld) {
						resp.Header.Set("Location", strings.ReplaceAll(loc, cfg.replaceOld, cfg.replaceNew))
					}
				} else if cfg.replaceOld != "" && strings.Contains(loc, cfg.replaceOld) {
					resp.Header.Set("Location", strings.ReplaceAll(loc, cfg.replaceOld, cfg.replaceNew))
				}
			}
		}

		return nil
	}

	proxy.ErrorHandler = func(rw http.ResponseWriter, req *http.Request, err error) {
		log.Printf("proxy error: %v", err)
		rw.WriteHeader(http.StatusBadGateway)
		_, _ = rw.Write([]byte("proxy error"))
	}

	addr := net.JoinHostPort(cfg.host, strconv.Itoa(cfg.port))

	server := &http.Server{
		Addr:         addr,
		Handler:      loggingMiddleware(proxy),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("proxy listening on http://%s -> %s", addr, cfg.targetURL)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	// Graceful shutdown on interrupt/terminate
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}

func rewriteCookieDomain(cookieHeader, replacement string) string {
	parts := strings.Split(cookieHeader, ";")
	rewritten := false

	for i, part := range parts {
		part = strings.TrimSpace(part)
		if len(part) == 0 {
			continue
		}
		if strings.HasPrefix(strings.ToLower(part), "domain=") {
			parts[i] = "Domain=" + replacement
			rewritten = true
			break
		}
	}

	if !rewritten {
		parts = append(parts, "Domain="+replacement)
	}

	return strings.Join(parts, "; ")
}

func determineAllowedOrigin(req *http.Request, cfg *config) string {
	if cfg.corsOrigin != "" {
		return cfg.corsOrigin
	}

	if req == nil {
		return ""
	}

	if origin := strings.TrimSpace(req.Header.Get("Origin")); origin != "" {
		return origin
	}

	if referer := strings.TrimSpace(req.Header.Get("Referer")); referer != "" {
		if parsed, err := url.Parse(referer); err == nil && parsed.Scheme != "" && parsed.Host != "" {
			return parsed.Scheme + "://" + parsed.Host
		}
	}

	return ""
}
