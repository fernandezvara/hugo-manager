package server

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/fernandezvara/hugo-manager/internal/config"
	"github.com/fernandezvara/hugo-manager/internal/files"
	"github.com/fernandezvara/hugo-manager/internal/hugo"
	"github.com/fernandezvara/hugo-manager/internal/images"
	"github.com/fernandezvara/hugo-manager/internal/shortcodes"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
)

// Server handles HTTP requests
type Server struct {
	projectDir   string
	config       *config.Config
	hugoMgr      *hugo.Manager
	fileMgr      *files.Manager
	shortcodeMgr *shortcodes.Parser
	imageMgr     *images.Processor
	webFS        embed.FS
	upgrader     websocket.Upgrader
}

// New creates a new server
func New(projectDir string, cfg *config.Config, hugoMgr *hugo.Manager, webFS embed.FS) *Server {
	return &Server{
		projectDir:   projectDir,
		config:       cfg,
		hugoMgr:      hugoMgr,
		fileMgr:      files.NewManager(projectDir, cfg.FileTree),
		shortcodeMgr: shortcodes.NewParser(projectDir),
		imageMgr:     images.NewProcessor(projectDir, cfg.Images),
		webFS:        webFS,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				// Check if origin is allowed based on configuration
				origin := r.Header.Get("Origin")
				if origin == "" {
					return true // Allow same-origin requests
				}

				// If no specific origins configured, allow all
				if len(cfg.Server.WSOrigins) == 0 || (len(cfg.Server.WSOrigins) == 1 && cfg.Server.WSOrigins[0] == "*") {
					return true
				}

				// Check against allowed origins
				for _, allowedOrigin := range cfg.Server.WSOrigins {
					if allowedOrigin == "*" || allowedOrigin == origin {
						return true
					}
				}
				return false
			},
		},
	}
}

// Start starts the HTTP server with chi router and graceful shutdown
func (s *Server) Start(addr string) error {
	r := chi.NewRouter()

	// Setup middleware
	s.setupMiddleware(r)

	// Setup routes
	s.setupRoutes(r)

	// Static files from Vite build output
	distFS, err := fs.Sub(s.webFS, "dist")
	if err != nil {
		return fmt.Errorf("failed to get dist fs: %w", err)
	}
	r.Handle("/static/dist/*", http.StripPrefix("/static/dist/", http.FileServer(http.FS(distFS))))

	// Create HTTP server with configuration-based timeouts
	server := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  time.Duration(s.config.Server.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(s.config.Server.WriteTimeout) * time.Second,
		IdleTimeout:  time.Duration(s.config.Server.IdleTimeout) * time.Second,
	}

	// Start server in a goroutine
	go func() {
		s.logInfo("Starting server on %s", addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logError("Server failed to start: %v", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	s.logInfo("Shutting down server...")

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(s.config.Server.ShutdownTimeout)*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		s.logError("Server forced to shutdown: %v", err)
		return err
	}

	s.logInfo("Server gracefully stopped")
	return nil
}

// setupRoutes configures all routes for the chi router
func (s *Server) setupRoutes(r chi.Router) {
	// Main page
	r.Get("/", s.handleIndex)

	// API routes
	r.Route("/api", func(r chi.Router) {
		// File management routes
		r.Route("/files", func(r chi.Router) {
			r.Get("/", s.handleFiles)
			r.Get("/search", s.handleFileSearch)
			r.Get("/raw", s.handleFileRaw)
			r.Get("/{path}", s.handleFileGet)
			r.Put("/{path}", s.handleFilePut)
			r.Post("/{path}", s.handleFilePost)
			r.Delete("/{path}", s.handleFileDelete)
			r.Post("/upload", s.handleFileUpload)
			r.Post("/copy", s.handleFileCopy)
		})

		// Shortcode routes
		r.Route("/shortcodes", func(r chi.Router) {
			r.Get("/", s.handleShortcodes)
			r.Get("/{name}", s.handleShortcode)
		})

		// Image management routes
		r.Route("/images", func(r chi.Router) {
			r.Post("/upload", s.handleImageUpload)
			r.Post("/process", s.handleImageProcess)
			r.Get("/processed", s.handleImageProcessed)
			r.Get("/folders", s.handleImageFolders)
			r.Get("/presets", s.handleImagePresets)
		})

		// Hugo management routes
		r.Route("/hugo", func(r chi.Router) {
			r.Get("/status", s.handleHugoStatus)
			r.Post("/start", s.handleHugoStart)
			r.Post("/stop", s.handleHugoStop)
			r.Post("/restart", s.handleHugoRestart)
			r.Get("/logs", s.handleHugoLogs)
			r.Get("/ws", s.handleHugoWS)
		})

		// Configuration routes
		r.Route("/config", func(r chi.Router) {
			r.Use(s.authMiddleware) // Protect config routes
			r.Get("/", s.handleConfigGet)
			r.Put("/", s.handleConfigPut)
		})

		// Data files for shortcodes
		r.Route("/data", func(r chi.Router) {
			r.Get("/", s.handleDataFiles)
			r.Get("/*", s.handleDataFiles)
		})
	})
}
