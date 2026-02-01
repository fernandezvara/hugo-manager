package server

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"github.com/fernandezvara/hugo-manager/internal/config"
	"github.com/fernandezvara/hugo-manager/internal/files"
	"github.com/fernandezvara/hugo-manager/internal/hugo"
	"github.com/fernandezvara/hugo-manager/internal/images"
	"github.com/fernandezvara/hugo-manager/internal/shortcodes"
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
				return true // Allow local connections
			},
		},
	}
}

// Start starts the HTTP server
func (s *Server) Start(addr string) error {
	mux := http.NewServeMux()

	// API routes
	mux.HandleFunc("/api/files", s.handleFiles)
	mux.HandleFunc("/api/files/", s.handleFile)
	mux.HandleFunc("/api/shortcodes", s.handleShortcodes)
	mux.HandleFunc("/api/shortcodes/", s.handleShortcode)
	mux.HandleFunc("/api/images/upload", s.handleImageUpload)
	mux.HandleFunc("/api/images/folders", s.handleImageFolders)
	mux.HandleFunc("/api/images/presets", s.handleImagePresets)
	mux.HandleFunc("/api/hugo/status", s.handleHugoStatus)
	mux.HandleFunc("/api/hugo/start", s.handleHugoStart)
	mux.HandleFunc("/api/hugo/stop", s.handleHugoStop)
	mux.HandleFunc("/api/hugo/restart", s.handleHugoRestart)
	mux.HandleFunc("/api/hugo/logs", s.handleHugoLogs)
	mux.HandleFunc("/api/hugo/ws", s.handleHugoWS)
	mux.HandleFunc("/api/config", s.handleConfig)
	mux.HandleFunc("/api/data/", s.handleDataFiles)

	// Static files from Vite build output
	distFS, err := fs.Sub(s.webFS, "dist")
	if err != nil {
		return fmt.Errorf("failed to get dist fs: %w", err)
	}
	mux.Handle("/static/dist/", http.StripPrefix("/static/dist/", http.FileServer(http.FS(distFS))))

	// Main page
	mux.HandleFunc("/", s.handleIndex)

	return http.ListenAndServe(addr, s.logMiddleware(mux))
}

func (s *Server) logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		if !strings.HasPrefix(r.URL.Path, "/api/hugo/ws") {
			log.Printf("%s %s %v", r.Method, r.URL.Path, time.Since(start))
		}
	})
}

// handleIndex serves the main HTML page
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	data, err := s.webFS.ReadFile("index.html")
	if err != nil {
		http.Error(w, "Failed to load page", http.StatusInternalServerError)
		return
	}

	// Inject configuration
	configJSON, _ := json.Marshal(map[string]interface{}{
		"hugoPort":    s.config.Hugo.Port,
		"editor":      s.config.Editor,
		"templates":   s.config.Templates,
		"projectName": filepath.Base(s.projectDir),
	})

	html := string(data)
	html = strings.Replace(html, "{{CONFIG_JSON}}", string(configJSON), 1)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}

// handleFiles returns the file tree
func (s *Server) handleFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	tree, err := s.fileMgr.GetTree()
	if err != nil {
		s.jsonError(w, "Failed to get file tree", http.StatusInternalServerError)
		return
	}

	s.jsonResponse(w, tree)
}

// handleFile handles individual file operations
func (s *Server) handleFile(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/files/")
	if path == "" {
		http.Error(w, "Path required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		content, err := s.fileMgr.ReadFile(path)
		if err != nil {
			s.jsonError(w, "Failed to read file: "+err.Error(), http.StatusNotFound)
			return
		}
		info, _ := s.fileMgr.GetFileInfo(path)
		s.jsonResponse(w, map[string]interface{}{
			"content": content,
			"info":    info,
		})

	case http.MethodPut:
		var req struct {
			Content string `json:"content"`
			NewName string `json:"newName"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.jsonError(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		if req.NewName != "" {
			// Rename operation
			newPath := filepath.Join(filepath.Dir(path), req.NewName)
			if err := s.fileMgr.RenameFile(path, newPath); err != nil {
				if strings.Contains(err.Error(), "already exists") {
					s.jsonError(w, "Destination already exists", http.StatusConflict)
				} else if strings.Contains(err.Error(), "does not exist") {
					s.jsonError(w, "Source does not exist", http.StatusNotFound)
				} else if strings.Contains(err.Error(), "invalid path") {
					s.jsonError(w, "Invalid path", http.StatusBadRequest)
				} else {
					s.jsonError(w, "Failed to rename: "+err.Error(), http.StatusInternalServerError)
				}
				return
			}
			s.jsonResponse(w, map[string]string{"status": "renamed"})
		} else {
			// Save operation
			if err := s.fileMgr.WriteFile(path, req.Content); err != nil {
				s.jsonError(w, "Failed to save file: "+err.Error(), http.StatusInternalServerError)
				return
			}
			s.jsonResponse(w, map[string]string{"status": "saved"})
		}

	case http.MethodPost:
		var req struct {
			Content  string                 `json:"content"`
			IsDir    bool                   `json:"isDir"`
			Template string                 `json:"template"`
			Data     map[string]interface{} `json:"data"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.jsonError(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		if req.IsDir {
			if err := s.fileMgr.CreateDir(path); err != nil {
				if strings.Contains(err.Error(), "already exists") {
					s.jsonError(w, "Directory already exists", http.StatusConflict)
				} else if strings.Contains(err.Error(), "invalid path") {
					s.jsonError(w, "Invalid directory path", http.StatusBadRequest)
				} else {
					s.jsonError(w, "Failed to create directory: "+err.Error(), http.StatusInternalServerError)
				}
				return
			}
		} else if req.Template != "" {
			// Create from template
			if err := s.fileMgr.CreateFileFromTemplate(path, req.Template, req.Data, s.config.Templates); err != nil {
				if err.Error() == "file already exists: "+path {
					s.jsonError(w, "File already exists", http.StatusConflict)
					return
				}
				s.jsonError(w, "Failed to create file from template: "+err.Error(), http.StatusInternalServerError)
				return
			}
		} else {
			// Regular file creation
			if err := s.fileMgr.CreateFile(path, req.Content); err != nil {
				if err.Error() == "file already exists: "+path {
					s.jsonError(w, "File already exists", http.StatusConflict)
					return
				}
				s.jsonError(w, "Failed to create file: "+err.Error(), http.StatusInternalServerError)
				return
			}
		}
		s.jsonResponse(w, map[string]string{"status": "created"})

	case http.MethodDelete:
		if err := s.fileMgr.DeleteFile(path); err != nil {
			if strings.Contains(err.Error(), "does not exist") {
				s.jsonError(w, "File or directory does not exist", http.StatusNotFound)
			} else if strings.Contains(err.Error(), "not empty") {
				s.jsonError(w, "Directory not empty", http.StatusConflict)
			} else if strings.Contains(err.Error(), "invalid path") {
				s.jsonError(w, "Invalid path", http.StatusBadRequest)
			} else {
				s.jsonError(w, "Failed to delete: "+err.Error(), http.StatusInternalServerError)
			}
			return
		}
		s.jsonResponse(w, map[string]string{"status": "deleted"})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleShortcodes returns all detected shortcodes
func (s *Server) handleShortcodes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	shortcodes, err := s.shortcodeMgr.DetectAll()
	if err != nil {
		s.jsonError(w, "Failed to detect shortcodes", http.StatusInternalServerError)
		return
	}

	s.jsonResponse(w, shortcodes)
}

// handleShortcode returns a specific shortcode
func (s *Server) handleShortcode(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/api/shortcodes/")
	if name == "" {
		http.Error(w, "Shortcode name required", http.StatusBadRequest)
		return
	}

	sc, err := s.shortcodeMgr.GetShortcode(name)
	if err != nil {
		s.jsonError(w, err.Error(), http.StatusNotFound)
		return
	}

	s.jsonResponse(w, sc)
}

// handleImageUpload handles image uploads
func (s *Server) handleImageUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse multipart form (max 50MB)
	if err := r.ParseMultipartForm(50 << 20); err != nil {
		s.jsonError(w, "Failed to parse form: "+err.Error(), http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("image")
	if err != nil {
		s.jsonError(w, "No image file provided", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Get filename from form or use original filename
	filename := r.FormValue("filename")
	if filename == "" {
		filename = header.Filename
	}

	// Parse options
	opts := images.UploadOptions{
		Folder:   r.FormValue("folder"),
		Filename: filename,
		Quality:  85,
	}

	if q := r.FormValue("quality"); q != "" {
		if qInt, err := strconv.Atoi(q); err == nil {
			opts.Quality = qInt
		}
	}

	if widths := r.FormValue("widths"); widths != "" {
		var w []int
		if err := json.Unmarshal([]byte(widths), &w); err == nil {
			opts.Widths = w
		}
	}

	result, err := s.imageMgr.Process(file, opts)
	if err != nil {
		s.jsonError(w, "Failed to process image: "+err.Error(), http.StatusInternalServerError)
		return
	}

	s.jsonResponse(w, result)
}

// handleImageFolders returns available image folders
func (s *Server) handleImageFolders(w http.ResponseWriter, r *http.Request) {
	s.jsonResponse(w, s.imageMgr.GetFolders())
}

// handleImagePresets returns available image presets
func (s *Server) handleImagePresets(w http.ResponseWriter, r *http.Request) {
	s.jsonResponse(w, s.imageMgr.GetPresets())
}

// handleHugoStatus returns Hugo server status
func (s *Server) handleHugoStatus(w http.ResponseWriter, r *http.Request) {
	status, msg := s.hugoMgr.GetStatus()
	s.jsonResponse(w, map[string]interface{}{
		"status":  status,
		"message": msg,
		"port":    s.hugoMgr.GetPort(),
	})
}

// handleHugoStart starts the Hugo server
func (s *Server) handleHugoStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := s.hugoMgr.Start(); err != nil {
		s.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.jsonResponse(w, map[string]string{"status": "starting"})
}

// handleHugoStop stops the Hugo server
func (s *Server) handleHugoStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := s.hugoMgr.Stop(); err != nil {
		s.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.jsonResponse(w, map[string]string{"status": "stopped"})
}

// handleHugoRestart restarts the Hugo server
func (s *Server) handleHugoRestart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := s.hugoMgr.Restart(); err != nil {
		s.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.jsonResponse(w, map[string]string{"status": "restarting"})
}

// handleHugoLogs returns recent Hugo logs
func (s *Server) handleHugoLogs(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if lInt, err := strconv.Atoi(l); err == nil {
			limit = lInt
		}
	}

	logs := s.hugoMgr.GetLogs(limit)
	s.jsonResponse(w, logs)
}

// handleHugoWS handles WebSocket connections for live log streaming
func (s *Server) handleHugoWS(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	// Subscribe to log updates
	logChan := s.hugoMgr.Subscribe()
	defer s.hugoMgr.Unsubscribe(logChan)

	// Send existing logs
	for _, entry := range s.hugoMgr.GetLogs(50) {
		data, _ := json.Marshal(entry)
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			return
		}
	}

	// Stream new logs
	for entry := range logChan {
		data, _ := json.Marshal(entry)
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			return
		}
	}
}

// handleConfig handles configuration get/update
func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.jsonResponse(w, s.config)

	case http.MethodPut:
		var newConfig config.Config
		if err := json.NewDecoder(r.Body).Decode(&newConfig); err != nil {
			s.jsonError(w, "Invalid configuration", http.StatusBadRequest)
			return
		}
		if err := config.Save(s.projectDir, &newConfig); err != nil {
			s.jsonError(w, "Failed to save configuration", http.StatusInternalServerError)
			return
		}
		s.config = &newConfig
		s.jsonResponse(w, map[string]string{"status": "saved"})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleDataFiles returns files for shortcode file selectors
func (s *Server) handleDataFiles(w http.ResponseWriter, r *http.Request) {
	dataType := strings.TrimPrefix(r.URL.Path, "/api/data/")
	if dataType == "" {
		dataType = "all"
	}

	files, err := s.fileMgr.ListDataFiles(dataType)
	if err != nil {
		s.jsonError(w, "Failed to list data files", http.StatusInternalServerError)
		return
	}

	s.jsonResponse(w, files)
}

func (s *Server) jsonResponse(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func (s *Server) jsonError(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

// readAll reads all data from a reader
func readAll(r io.Reader) ([]byte, error) {
	return io.ReadAll(r)
}
