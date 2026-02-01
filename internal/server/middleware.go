package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// setupMiddleware configures all middleware for the chi router
func (s *Server) setupMiddleware(r chi.Router) {
	// Standard chi middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(time.Duration(s.config.Server.Timeout) * time.Second))
	r.Use(middleware.AllowContentType("application/json", "multipart/form-data", "text/html"))

	// Custom middleware
	r.Use(s.corsMiddleware)
	r.Use(s.loggingMiddleware)
	r.Use(s.requestValidationMiddleware)
	r.Use(s.rateLimitMiddleware)
	r.Use(s.contentTypeMiddleware)
}

// corsMiddleware handles CORS headers based on configuration
func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set CORS headers based on configuration
		origins := s.config.Server.CORSOrigins
		if len(origins) == 0 {
			origins = []string{"*"}
		}

		origin := "*"
		if len(origins) > 0 && origins[0] != "*" {
			// For specific origins, check the request origin
			requestOrigin := r.Header.Get("Origin")
			if requestOrigin != "" {
				for _, allowedOrigin := range origins {
					if allowedOrigin == requestOrigin {
						origin = requestOrigin
						break
					}
				}
			}
		}

		w.Header().Set("Access-Control-Allow-Origin", origin)

		methods := s.config.Server.CORSMethods
		if len(methods) == 0 {
			methods = []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}
		}
		w.Header().Set("Access-Control-Allow-Methods", strings.Join(methods, ", "))

		headers := s.config.Server.CORSHeaders
		if len(headers) == 0 {
			headers = []string{"Content-Type", "Authorization"}
		}
		w.Header().Set("Access-Control-Allow-Headers", strings.Join(headers, ", "))

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// loggingMiddleware provides custom request logging
func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Create a wrapped response writer to capture status code
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

		// Process request
		next.ServeHTTP(ww, r)

		// Log request details
		duration := time.Since(start)
		s.logInfo("Request completed",
			"method", r.Method,
			"path", r.URL.Path,
			"status", ww.Status(),
			"duration", duration.String(),
			"remote", r.RemoteAddr,
			"user_agent", r.UserAgent(),
		)
	})
}

// authMiddleware provides authentication for protected routes
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// For now, we'll implement a simple check
		// In the future, this could be expanded to support JWT, OAuth, etc.

		// Simple token-based auth for demonstration
		// This can be configured via environment variables or config in the future
		token := r.Header.Get("Authorization")
		if token == "" {
			// No auth required for now - pass through
			next.ServeHTTP(w, r)
			return
		}

		// Add user info to context if token is present
		ctx := context.WithValue(r.Context(), "user", "admin")
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// requestValidationMiddleware provides request validation based on configuration
func (s *Server) requestValidationMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Validate request size based on configuration
		if s.config.Server.MaxRequestSize > 0 {
			if r.ContentLength > int64(s.config.Server.MaxRequestSize*1024*1024) {
				s.jsonError(w, http.StatusRequestEntityTooLarge,
					fmt.Sprintf("Request too large. Maximum size is %d MB", s.config.Server.MaxRequestSize))
				return
			}
		}

		// Additional validation can be added here:
		// - Content-Type validation for specific endpoints
		// - Required headers validation
		// - Query parameter validation
		// - Request body structure validation

		next.ServeHTTP(w, r)
	})
}

// rateLimitMiddleware provides rate limiting (pass-through for now)
func (s *Server) rateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// TODO: Implement actual rate limiting based on s.config.Server.RateLimit
		// For now, this is a pass-through middleware
		next.ServeHTTP(w, r)
	})
}

// contentTypeMiddleware ensures proper content type for API responses
func (s *Server) contentTypeMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only apply to API routes
		if len(r.URL.Path) >= 4 && r.URL.Path[:4] == "/api" {
			w.Header().Set("Content-Type", "application/json")
		}
		next.ServeHTTP(w, r)
	})
}
