package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/netscribe/netscribe/internal/handlers"
	"github.com/netscribe/netscribe/internal/models"
	"github.com/netscribe/netscribe/internal/parser"
	"github.com/netscribe/netscribe/internal/ui"
)

// New creates and configures an HTTP server with Chi router, dashboard UI,
// static asset routes, and API endpoints for HAR upload and code generation.
// The server is production-ready with secure defaults and proper error handling.
func New(port int) *http.Server {
	r := chi.NewRouter()

	// Middleware stack
	r.Use(chiMiddlewareLogger)
	r.Use(securityHeaders)

	// Static assets route (Tailwind output and any bundled JS/CSS).
	// In a full build pipeline this would be embedded; for development we serve from a static dir.
	// If the directory is missing the handler safely returns 404.
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.Dir("./static"))))

	// Dashboard UI (Templ-rendered synthwave neon interface)
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		component := ui.Dashboard()
		if err := component.Render(r.Context(), w); err != nil {
			http.Error(w, "failed to render dashboard template", http.StatusInternalServerError)
			return
		}
	})

	// API endpoints
	r.Post("/api/upload", handlers.UploadHAR)
	r.Post("/api/generate", handlers.Generate)

	// Health check
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	return srv
}

// NewProxy returns a standalone HTTP server that acts as a MITM-style proxy.
// It captures all traffic into an in-memory TrafficData, exposes /har for
// exporting a valid HAR file, and /clear to reset the buffer. HTTP is fully
// captured; HTTPS uses CONNECT tunneling.
func NewProxy(port int) *http.Server {
	h := &proxyHandler{
		traffic: &models.TrafficData{},
	}
	r := chi.NewRouter()

	r.Use(chiMiddlewareLogger)
	r.Use(securityHeaders)

	r.Handle("/*", h)

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	return srv
}

type proxyHandler struct {
	traffic *models.TrafficData
	mu      sync.Mutex
}

func (p *proxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/har":
		p.serveHAR(w, r)
		return
	case "/clear":
		p.clearCapture(w, r)
		return
	}
	p.serveProxy(w, r)
}

func (p *proxyHandler) serveHAR(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	td := p.traffic
	p.mu.Unlock()

	data, err := parser.ToHAR(td)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to build HAR: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="netscribe-capture.har"`)
	w.Write(data)
}

func (p *proxyHandler) clearCapture(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	p.traffic = &models.TrafficData{}
	p.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "cleared"})
}

func (p *proxyHandler) serveProxy(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodConnect {
		p.handleCONNECT(w, r)
		return
	}

	// Read request body (if present) before forwarding
	var bodyBytes []byte
	if r.Body != nil {
		var err error
		bodyBytes, err = io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to read request body: %v", err), http.StatusBadRequest)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	}

	// Build capture data outside lock
	reqHeaders := headerToMap(r.Header)
	query := queryToMap(r.URL.Query())
	var reqBody any
	if len(bodyBytes) > 0 {
		if json.Valid(bodyBytes) {
			json.Unmarshal(bodyBytes, &reqBody)
		} else {
			reqBody = string(bodyBytes)
		}
	}

	fullURL := r.URL.String()
	if !strings.HasPrefix(fullURL, "http") {
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		fullURL = scheme + "://" + r.Host + r.URL.RequestURI()
	}
	path := r.URL.EscapedPath()
	if path == "" {
		path = "/"
	}

	// Forward the request
	if r.URL.Scheme == "" {
		r.URL.Scheme = "http"
	}
	if r.URL.Host == "" {
		r.URL.Host = r.Host
	}

	proxyReq, err := http.NewRequestWithContext(r.Context(), r.Method, fullURL, io.NopCloser(bytes.NewReader(bodyBytes)))
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to create proxy request: %v", err), http.StatusInternalServerError)
		return
	}
	proxyReq.Header = r.Header.Clone()

	// Remove hop-by-hop headers
	for _, h := range []string{"Connection", "Proxy-Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization", "Te", "Trailers", "Transfer-Encoding", "Upgrade"} {
		proxyReq.Header.Del(h)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(proxyReq)
	if err != nil {
		http.Error(w, fmt.Sprintf("proxy request failed: %v", err), http.StatusBadGateway)
		p.mu.Lock()
		p.traffic.AddExample(r.Method, fullURL, path, reqHeaders, query, reqBody, 502, nil, map[string]any{"error": err.Error()})
		p.mu.Unlock()
		return
	}
	defer resp.Body.Close()

	// Read response body for capture
	var respBodyBytes []byte
	if resp.Body != nil {
		respBodyBytes, _ = io.ReadAll(resp.Body)
		resp.Body = io.NopCloser(bytes.NewReader(respBodyBytes))
	}

	var respBody any
	if len(respBodyBytes) > 0 {
		if json.Valid(respBodyBytes) {
			json.Unmarshal(respBodyBytes, &respBody)
		} else {
			respBody = string(respBodyBytes)
		}
	}
	respHeaders := headerToMap(resp.Header)

	// Record the exchange
	p.mu.Lock()
	p.traffic.AddExample(r.Method, fullURL, path, reqHeaders, query, reqBody, resp.StatusCode, respHeaders, respBody)
	p.mu.Unlock()

	// Forward response to client
	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	if len(respBodyBytes) > 0 {
		w.Write(respBodyBytes)
	}
}

func (p *proxyHandler) handleCONNECT(w http.ResponseWriter, r *http.Request) {
	target := r.Host
	if !strings.Contains(target, ":") {
		target += ":443"
	}
	destConn, err := net.Dial("tcp", target)
	if err != nil {
		http.Error(w, fmt.Sprintf("dial failed: %v", err), http.StatusServiceUnavailable)
		return
	}
	defer destConn.Close()

	w.WriteHeader(http.StatusOK)
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijacking not supported", http.StatusInternalServerError)
		return
	}
	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		http.Error(w, fmt.Sprintf("hijack failed: %v", err), http.StatusServiceUnavailable)
		return
	}
	defer clientConn.Close()

	go io.Copy(destConn, clientConn)
	io.Copy(clientConn, destConn)
}

func headerToMap(h http.Header) map[string]string {
	m := make(map[string]string, len(h))
	for k, vv := range h {
		if len(vv) > 0 {
			m[k] = vv[0]
		}
	}
	return m
}

func queryToMap(q map[string][]string) map[string]string {
	m := make(map[string]string, len(q))
	for k, vv := range q {
		if len(vv) > 0 {
			m[k] = vv[0]
		}
	}
	return m
}

// chiMiddlewareLogger wraps the standard chi request logger.
func chiMiddlewareLogger(next http.Handler) http.Handler {
	return middleware.Logger(next)
}

// securityHeaders adds synthwave-appropriate security headers with secure defaults.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline' https://cdn.tailwindcss.com; style-src 'self' 'unsafe-inline';")
		next.ServeHTTP(w, r)
	})
}
