package server

import (
	"fmt"
	"net/http"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"
	"github.com/netscribe/netscribe/internal/handlers"
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
		ReadTimeout:  30,
		WriteTimeout: 60,
		IdleTimeout:  120,
	}

	return srv
}

// chiMiddlewareLogger is a minimal request logger using Chi's built-in.
func chiMiddlewareLogger(next http.Handler) http.Handler {
	return chi.Chain(chi.Logger).Handler(next)
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
