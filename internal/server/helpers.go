package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
)

// Response helpers

// jsonResponse sends a JSON response with the given data and status code
func (s *Server) jsonResponse(w http.ResponseWriter, data interface{}, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		// If encoding fails, we can't send a JSON error response
		// Log it and send a plain text error
		s.logError("Failed to encode JSON response: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// jsonError sends a JSON error response
func (s *Server) jsonError(w http.ResponseWriter, code int, detail string) {
	errorResp := errorResponse{
		Code:   code,
		Detail: detail,
	}
	s.jsonResponse(w, errorResp, code)
}

// Response structs

// errorResponse represents a JSON error response
type errorResponse struct {
	Code   int    `json:"code"`
	Detail string `json:"detail"`
}

// successResponse represents a generic success response
type successResponse struct {
	Status string `json:"status"`
}

// fileCreateResponse represents the response for file creation
type fileCreateResponse struct {
	Path   string `json:"path"`
	Status string `json:"status"`
}

// fileUpdateResponse represents the response for file updates
type fileUpdateResponse struct {
	Path   string `json:"path"`
	Status string `json:"status"`
}

// fileDeleteResponse represents the response for file deletion
type fileDeleteResponse struct {
	Path   string `json:"path"`
	Status string `json:"status"`
}

// directoryCreateResponse represents the response for directory creation
type directoryCreateResponse struct {
	Path   string `json:"path"`
	Status string `json:"status"`
}

// Common status constants
const (
	StatusCreated = "created"
	StatusUpdated = "updated"
	StatusDeleted = "deleted"
	StatusRenamed = "renamed"
	StatusSuccess = "success"
	StatusError   = "error"
)

// Logging helpers

// logError logs an error message
func (s *Server) logError(format string, args ...interface{}) {
	// In a real implementation, this would use a proper logger
	// For now, we'll use the standard log package
	log.Printf("ERROR: "+format, args...)
}

// logInfo logs an info message
func (s *Server) logInfo(format string, args ...interface{}) {
	log.Printf("INFO: "+format, args...)
}

// logDebug logs a debug message
func (s *Server) logDebug(format string, args ...interface{}) {
	log.Printf("DEBUG: "+format, args...)
}

// Input validation helpers

// validatePath validates file paths to prevent directory traversal
func (s *Server) validatePath(path string) error {
	if path == "" {
		return nil
	}

	// Check for directory traversal attempts
	if strings.Contains(path, "..") {
		return fmt.Errorf("invalid path: directory traversal not allowed")
	}

	// Check for absolute paths (should be relative)
	if strings.HasPrefix(path, "/") {
		return fmt.Errorf("invalid path: absolute paths not allowed")
	}

	return nil
}

// validateFilename validates filenames for security
func (s *Server) validateFilename(filename string) error {
	if filename == "" {
		return fmt.Errorf("filename cannot be empty")
	}

	// Check for invalid characters
	invalidChars := []string{"<", ">", ":", "\"", "|", "?", "*", "\x00"}
	for _, char := range invalidChars {
		if strings.Contains(filename, char) {
			return fmt.Errorf("filename contains invalid character: %s", char)
		}
	}

	// Check for reserved names (Windows)
	reservedNames := []string{"CON", "PRN", "AUX", "NUL", "COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9", "LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9"}
	base := strings.ToUpper(strings.TrimSuffix(filename, filepath.Ext(filename)))
	for _, reserved := range reservedNames {
		if base == reserved {
			return fmt.Errorf("filename is reserved: %s", filename)
		}
	}

	return nil
}

// getURLParam safely extracts URL parameters from chi context
func (s *Server) getURLParam(r *http.Request, param string) string {
	return chi.URLParam(r, param)
}
