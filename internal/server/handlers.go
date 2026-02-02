package server

import (
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"log"

	"github.com/fernandezvara/hugo-manager/internal/config"
	"github.com/fernandezvara/hugo-manager/internal/images"
	"github.com/gorilla/websocket"
)

// handleIndex serves the main HTML page
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	data, err := s.webFS.ReadFile("index.html")
	if err != nil {
		s.jsonError(w, http.StatusInternalServerError, "Failed to load page")
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
	show := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("show")))
	if show == "" {
		show = "all"
	}
	q := r.URL.Query().Get("q")
	folder := r.URL.Query().Get("folder")

	var tree interface{}
	var err error

	switch show {
	case "images":
		var roots []string
		if folder != "" {
			roots = []string{folder}
		} else {
			for _, f := range s.imageMgr.GetFolders() {
				roots = append(roots, f.Path)
			}
		}
		allowedTypes := map[string]bool{"image": true}
		tree, err = s.fileMgr.GetFilteredTree(roots, q, allowedTypes, true)
	case "markdown":
		roots := []string{folder}
		if folder == "" {
			roots = s.config.FileTree.ShowDirs
		}
		allowedTypes := map[string]bool{"markdown": true}
		tree, err = s.fileMgr.GetFilteredTree(roots, q, allowedTypes, true)
	case "all":
		if folder != "" {
			tree, err = s.fileMgr.GetTreeForRoots([]string{folder})
		} else {
			tree, err = s.fileMgr.GetTree()
		}
	default:
		s.jsonError(w, http.StatusBadRequest, "Invalid show parameter")
		return
	}

	if err != nil {
		s.jsonError(w, http.StatusInternalServerError, "Failed to get file tree")
		return
	}

	s.jsonResponse(w, tree, http.StatusOK)
}

func (s *Server) handleFileSearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	folder := r.URL.Query().Get("folder")

	var folders []string
	if folder != "" {
		folders = []string{folder}
	} else {
		for _, f := range s.imageMgr.GetFolders() {
			folders = append(folders, f.Path)
		}
	}

	results, err := s.fileMgr.SearchImages(folders, query)
	if err != nil {
		s.jsonError(w, http.StatusInternalServerError, "Failed to search files")
		return
	}

	s.jsonResponse(w, results, http.StatusOK)
}

func (s *Server) handleFileRaw(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		s.jsonError(w, http.StatusBadRequest, "path is required")
		return
	}

	if !s.fileMgr.IsValidPath(path) {
		s.jsonError(w, http.StatusBadRequest, "invalid path")
		return
	}

	data, err := s.fileMgr.ReadFileBytes(path)
	if err != nil {
		if os.IsNotExist(err) {
			s.jsonError(w, http.StatusNotFound, "File not found")
			return
		}
		s.jsonError(w, http.StatusInternalServerError, "Failed to read file")
		return
	}

	contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(path)))
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// handleFileGet handles GET requests for file content
func (s *Server) handleFileGet(w http.ResponseWriter, r *http.Request) {
	path := s.getURLParam(r, "path")
	if path == "" {
		s.jsonError(w, http.StatusBadRequest, "Path required")
		return
	}

	content, err := s.fileMgr.ReadFile(path)
	if err != nil {
		s.jsonError(w, http.StatusNotFound, "Failed to read file: "+err.Error())
		return
	}
	info, _ := s.fileMgr.GetFileInfo(path)
	s.jsonResponse(w, map[string]interface{}{
		"content": content,
		"info":    info,
	}, http.StatusOK)
}

// handleFilePut handles PUT requests for file updates/renames
func (s *Server) handleFilePut(w http.ResponseWriter, r *http.Request) {
	path := s.getURLParam(r, "path")
	if path == "" {
		s.jsonError(w, http.StatusBadRequest, "Path required")
		return
	}

	var req struct {
		Content string `json:"content"`
		NewName string `json:"newName"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.NewName != "" {
		// Rename operation
		newPath := filepath.Join(filepath.Dir(path), req.NewName)
		if err := s.fileMgr.RenameFile(path, newPath); err != nil {
			if strings.Contains(err.Error(), "already exists") {
				s.jsonError(w, http.StatusConflict, "Destination already exists")
			} else if strings.Contains(err.Error(), "does not exist") {
				s.jsonError(w, http.StatusNotFound, "Source does not exist")
			} else if strings.Contains(err.Error(), "invalid path") {
				s.jsonError(w, http.StatusBadRequest, "Invalid path")
			} else {
				s.jsonError(w, http.StatusInternalServerError, "Failed to rename: "+err.Error())
			}
			return
		}
		s.jsonResponse(w, &fileUpdateResponse{Path: path, Status: "renamed"}, http.StatusOK)
	} else {
		// Save operation
		if err := s.fileMgr.WriteFile(path, req.Content); err != nil {
			s.jsonError(w, http.StatusInternalServerError, "Failed to save file: "+err.Error())
			return
		}
		s.jsonResponse(w, &fileUpdateResponse{Path: path, Status: "saved"}, http.StatusOK)
	}
}

// handleFilePost handles POST requests for file/directory creation
func (s *Server) handleFilePost(w http.ResponseWriter, r *http.Request) {
	path := s.getURLParam(r, "path")
	if path == "" {
		s.jsonError(w, http.StatusBadRequest, "Path required")
		return
	}

	var req struct {
		Content  string                 `json:"content"`
		IsDir    bool                   `json:"isDir"`
		Template string                 `json:"template"`
		Data     map[string]interface{} `json:"data"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.IsDir {
		if err := s.fileMgr.CreateDir(path); err != nil {
			if strings.Contains(err.Error(), "already exists") {
				s.jsonError(w, http.StatusConflict, "Directory already exists")
			} else if strings.Contains(err.Error(), "invalid path") {
				s.jsonError(w, http.StatusBadRequest, "Invalid directory path")
			} else {
				s.jsonError(w, http.StatusInternalServerError, "Failed to create directory: "+err.Error())
			}
			return
		}
	} else if req.Template != "" {
		// Create from template
		if err := s.fileMgr.CreateFileFromTemplate(path, req.Template, req.Data, s.config.Templates); err != nil {
			if err.Error() == "file already exists: "+path {
				s.jsonError(w, http.StatusConflict, "File already exists")
				return
			}
			s.jsonError(w, http.StatusInternalServerError, "Failed to create file from template: "+err.Error())
			return
		}
	} else {
		// Regular file creation
		if err := s.fileMgr.CreateFile(path, req.Content); err != nil {
			if err.Error() == "file already exists: "+path {
				s.jsonError(w, http.StatusConflict, "File already exists")
				return
			}
			s.jsonError(w, http.StatusInternalServerError, "Failed to create file: "+err.Error())
			return
		}
	}
	s.jsonResponse(w, &fileCreateResponse{Path: path, Status: "created"}, http.StatusOK)
}

// handleFileDelete handles DELETE requests for file/directory deletion
func (s *Server) handleFileDelete(w http.ResponseWriter, r *http.Request) {
	path := s.getURLParam(r, "path")
	if path == "" {
		s.jsonError(w, http.StatusBadRequest, "Path required")
		return
	}

	if err := s.fileMgr.DeleteFile(path); err != nil {
		if strings.Contains(err.Error(), "does not exist") {
			s.jsonError(w, http.StatusNotFound, "File or directory does not exist")
		} else if strings.Contains(err.Error(), "not empty") {
			s.jsonError(w, http.StatusConflict, "Directory not empty")
		} else if strings.Contains(err.Error(), "invalid path") {
			s.jsonError(w, http.StatusBadRequest, "Invalid path")
		} else {
			s.jsonError(w, http.StatusInternalServerError, "Failed to delete: "+err.Error())
		}
		return
	}
	s.jsonResponse(w, &fileDeleteResponse{Path: path, Status: "deleted"}, http.StatusOK)
}

// handleShortcodes returns all detected shortcodes
func (s *Server) handleShortcodes(w http.ResponseWriter, r *http.Request) {
	shortcodes, err := s.shortcodeMgr.DetectAll()
	if err != nil {
		s.jsonError(w, http.StatusInternalServerError, "Failed to detect shortcodes")
		return
	}

	s.jsonResponse(w, shortcodes, http.StatusOK)
}

// handleShortcode returns a specific shortcode
func (s *Server) handleShortcode(w http.ResponseWriter, r *http.Request) {
	name := s.getURLParam(r, "name")
	if name == "" {
		s.jsonError(w, http.StatusBadRequest, "Shortcode name required")
		return
	}

	sc, err := s.shortcodeMgr.GetShortcode(name)
	if err != nil {
		s.jsonError(w, http.StatusNotFound, err.Error())
		return
	}

	s.jsonResponse(w, sc, http.StatusOK)
}

// handleImageUpload handles image uploads
func (s *Server) handleImageUpload(w http.ResponseWriter, r *http.Request) {
	// Parse multipart form (max 50MB)
	if err := r.ParseMultipartForm(50 << 20); err != nil {
		s.jsonError(w, http.StatusBadRequest, "Failed to parse form: "+err.Error())
		return
	}

	file, header, err := r.FormFile("image")
	if err != nil {
		s.jsonError(w, http.StatusBadRequest, "No image file provided")
		return
	}
	defer file.Close()

	// Get filename from form or use original filename
	filename := r.FormValue("filename")
	if filename == "" {
		filename = header.Filename
	}

	// Create processing options
	opts := images.UploadOptions{
		Folder:   r.FormValue("folder"),
		Filename: filename,
		Quality:  85,
	}

	if quality := r.FormValue("quality"); quality != "" {
		if q, err := strconv.Atoi(quality); err == nil {
			opts.Quality = q
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
		s.jsonError(w, http.StatusInternalServerError, "Failed to process image: "+err.Error())
		return
	}

	s.jsonResponse(w, result, http.StatusOK)
}

// handleImageProcessed builds result (variants/srcset/shortcode) from already-processed image variants
func (s *Server) handleImageProcessed(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	path = filepath.ToSlash(strings.TrimSpace(path))
	if path == "" {
		s.jsonError(w, http.StatusBadRequest, "path is required")
		return
	}
	if !s.fileMgr.IsValidPath(path) {
		s.jsonError(w, http.StatusBadRequest, "Invalid path")
		return
	}

	result, err := s.imageMgr.BuildResultFromProcessedVariants(path)
	if err != nil {
		s.jsonError(w, http.StatusBadRequest, err.Error())
		return
	}

	s.jsonResponse(w, result, http.StatusOK)
}

func (s *Server) handleImageFolders(w http.ResponseWriter, r *http.Request) {
	folders := s.imageMgr.GetFolders()
	s.jsonResponse(w, folders, http.StatusOK)
}

// handleImagePresets returns available image presets
func (s *Server) handleImagePresets(w http.ResponseWriter, r *http.Request) {
	presets := s.imageMgr.GetPresets()
	s.jsonResponse(w, presets, http.StatusOK)
}

// handleFileUpload handles generic file uploads
func (s *Server) handleFileUpload(w http.ResponseWriter, r *http.Request) {
	// Parse multipart form (max 50MB)
	if err := r.ParseMultipartForm(50 << 20); err != nil {
		s.jsonError(w, http.StatusBadRequest, "Failed to parse form data")
		return
	}

	// Get file from form
	file, header, err := r.FormFile("file")
	if err != nil {
		s.jsonError(w, http.StatusBadRequest, "No file provided")
		return
	}
	defer file.Close()

	// Get folder from form (required)
	folder := r.FormValue("folder")
	if folder == "" {
		s.jsonError(w, http.StatusBadRequest, "folder is required for file upload")
		return
	}

	// Get filename from form or use original filename
	filename := r.FormValue("filename")
	if filename == "" {
		filename = header.Filename
	}

	// Create full file path
	targetPath := filepath.Join(s.projectDir, folder, filename)

	// Create directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		s.jsonError(w, http.StatusInternalServerError, "Failed to create directory")
		return
	}

	// Create destination file
	dst, err := os.Create(targetPath)
	if err != nil {
		s.jsonError(w, http.StatusInternalServerError, "Failed to create file")
		return
	}
	defer dst.Close()

	// Copy file content
	if _, err := io.Copy(dst, file); err != nil {
		s.jsonError(w, http.StatusInternalServerError, "Failed to save file")
		return
	}

	// Return success response
	s.jsonResponse(w, map[string]interface{}{
		"message":  "File uploaded successfully",
		"filename": filename,
		"path":     filepath.Join(folder, filename),
		"size":     header.Size,
	}, http.StatusOK)
}

// handleFileCopy copies an existing file
func (s *Server) handleFileCopy(w http.ResponseWriter, r *http.Request) {
	// Parse multipart form (max 50MB)
	if err := r.ParseMultipartForm(50 << 20); err != nil {
		s.jsonError(w, http.StatusBadRequest, "Failed to parse form data")
		return
	}

	// Get source path from form
	sourcePath := r.FormValue("sourcePath")
	if sourcePath == "" {
		s.jsonError(w, http.StatusBadRequest, "sourcePath is required")
		return
	}

	// Get target filename from form
	targetFilename := r.FormValue("targetFilename")
	if targetFilename == "" {
		s.jsonError(w, http.StatusBadRequest, "targetFilename is required")
		return
	}

	// Get target folder (optional, defaults to source directory)
	targetFolder := r.FormValue("folder")
	if targetFolder == "" {
		// Extract directory from source path
		lastSlash := strings.LastIndex(sourcePath, "/")
		if lastSlash > 0 {
			targetFolder = sourcePath[:lastSlash]
		}
	}

	// Create full paths
	fullSourcePath := filepath.Join(s.projectDir, sourcePath)
	fullTargetPath := filepath.Join(s.projectDir, targetFolder, targetFilename)

	// Check if source file exists
	if _, err := os.Stat(fullSourcePath); os.IsNotExist(err) {
		s.jsonError(w, http.StatusBadRequest, "Source file not found")
		return
	}

	// Copy file
	source, err := os.Open(fullSourcePath)
	if err != nil {
		s.jsonError(w, http.StatusInternalServerError, "Failed to open source file")
		return
	}
	defer source.Close()

	destination, err := os.Create(fullTargetPath)
	if err != nil {
		s.jsonError(w, http.StatusInternalServerError, "Failed to create destination file")
		return
	}
	defer destination.Close()

	if _, err := io.Copy(destination, source); err != nil {
		s.jsonError(w, http.StatusInternalServerError, "Failed to copy file")
		return
	}

	// Return success response
	s.jsonResponse(w, map[string]interface{}{
		"message": "File copied successfully",
		"source":  sourcePath,
		"target":  filepath.Join(targetFolder, targetFilename),
	}, http.StatusOK)
}

// handleImageProcess processes existing images
func (s *Server) handleImageProcess(w http.ResponseWriter, r *http.Request) {
	// Parse multipart form (max 50MB)
	if err := r.ParseMultipartForm(50 << 20); err != nil {
		s.jsonError(w, http.StatusBadRequest, "Failed to parse form data")
		return
	}

	// Get source image path from form
	sourcePath := r.FormValue("sourcePath")
	if sourcePath == "" {
		s.jsonError(w, http.StatusBadRequest, "sourcePath is required")
		return
	}

	// Get target folder from form (required)
	targetFolder := r.FormValue("folder")
	if targetFolder == "" {
		s.jsonError(w, http.StatusBadRequest, "folder is required for image processing")
		return
	}

	// Get filename from form or use original filename
	filename := r.FormValue("filename")
	if filename == "" {
		filename = filepath.Base(sourcePath)
	}

	// Get processing options
	quality := r.FormValue("quality")
	if quality == "" {
		quality = "85"
	}

	preset := r.FormValue("preset")
	if preset == "" {
		preset = "Full responsive"
	}

	widths := r.FormValue("widths")

	// Create full source path
	fullSourcePath := filepath.Join(s.projectDir, sourcePath)

	// Check if source file exists
	if _, err := os.Stat(fullSourcePath); os.IsNotExist(err) {
		s.jsonError(w, http.StatusBadRequest, "Source image file not found")
		return
	}

	// Create processing options similar to upload but with existing file
	opts := images.UploadOptions{
		Folder:     targetFolder,
		Filename:   filename,
		Quality:    parseInt(quality),
		PresetName: preset,
		Widths:     parseWidths(widths),
	}

	// Process the existing image
	result, err := s.imageMgr.ProcessExistingImage(fullSourcePath, opts)
	if err != nil {
		s.jsonError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to process image: %v", err))
		return
	}

	// Return success response
	s.jsonResponse(w, result, http.StatusOK)
}

// Helper functions
func parseInt(s string) int {
	if i, err := strconv.Atoi(s); err == nil {
		return i
	}
	return 85
}

func parseWidths(s string) []int {
	if s == "" {
		return nil
	}

	var widths []int
	for _, w := range strings.Split(s, ",") {
		if i, err := strconv.Atoi(strings.TrimSpace(w)); err == nil && i > 0 {
			widths = append(widths, i)
		}
	}
	return widths
}

// handleHugoStatus returns Hugo server status
func (s *Server) handleHugoStatus(w http.ResponseWriter, r *http.Request) {
	status, msg := s.hugoMgr.GetStatus()
	s.jsonResponse(w, map[string]interface{}{
		"status":  status,
		"message": msg,
		"port":    s.hugoMgr.GetPort(),
	}, http.StatusOK)
}

// handleHugoStart starts the Hugo server
func (s *Server) handleHugoStart(w http.ResponseWriter, r *http.Request) {
	if err := s.hugoMgr.Start(); err != nil {
		s.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.jsonResponse(w, &successResponse{Status: "starting"}, http.StatusOK)
}

// handleHugoStop stops the Hugo server
func (s *Server) handleHugoStop(w http.ResponseWriter, r *http.Request) {
	if err := s.hugoMgr.Stop(); err != nil {
		s.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.jsonResponse(w, &successResponse{Status: "stopped"}, http.StatusOK)
}

// handleHugoRestart restarts the Hugo server
func (s *Server) handleHugoRestart(w http.ResponseWriter, r *http.Request) {
	if err := s.hugoMgr.Restart(); err != nil {
		s.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.jsonResponse(w, &successResponse{Status: "restarting"}, http.StatusOK)
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
	s.jsonResponse(w, logs, http.StatusOK)
}

// handleHugoWS handles WebSocket connections for live log streaming
func (s *Server) handleHugoWS(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	// Send logs to client every second
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		logs := s.hugoMgr.GetLogs(10)
		data, err := json.Marshal(logs)
		if err != nil {
			continue
		}

		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			return
		}
	}
}

// handleConfigGet handles GET requests for configuration
func (s *Server) handleConfigGet(w http.ResponseWriter, r *http.Request) {
	s.jsonResponse(w, s.config, http.StatusOK)
}

// handleConfigPut handles PUT requests for configuration updates
func (s *Server) handleConfigPut(w http.ResponseWriter, r *http.Request) {
	var newConfig config.Config
	if err := json.NewDecoder(r.Body).Decode(&newConfig); err != nil {
		s.jsonError(w, http.StatusBadRequest, "Invalid configuration")
		return
	}
	if err := config.Save(s.projectDir, &newConfig); err != nil {
		s.jsonError(w, http.StatusInternalServerError, "Failed to save configuration")
		return
	}
	s.config = &newConfig
	s.jsonResponse(w, &successResponse{Status: "saved"}, http.StatusOK)
}

// handleDataFiles returns files for shortcode file selectors
func (s *Server) handleDataFiles(w http.ResponseWriter, r *http.Request) {
	dataType := s.getURLParam(r, "*")
	if dataType == "" {
		dataType = "all"
	}

	files, err := s.fileMgr.ListDataFiles(dataType)
	if err != nil {
		s.jsonError(w, http.StatusInternalServerError, "Failed to list data files")
		return
	}

	s.jsonResponse(w, files, http.StatusOK)
}
