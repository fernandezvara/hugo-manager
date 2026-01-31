package files

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/fernandezvara/hugo-manager/internal/config"
)

// FileInfo represents a file or directory in the tree
type FileInfo struct {
	Name     string     `json:"name"`
	Path     string     `json:"path"`
	IsDir    bool       `json:"isDir"`
	Size     int64      `json:"size,omitempty"`
	ModTime  int64      `json:"modTime,omitempty"`
	Children []FileInfo `json:"children,omitempty"`
	Type     string     `json:"type,omitempty"` // "markdown", "html", "yaml", "image", etc.
}

// Manager handles file operations
type Manager struct {
	projectDir string
	config     config.FileTreeConfig
}

// NewManager creates a new file manager
func NewManager(projectDir string, cfg config.FileTreeConfig) *Manager {
	return &Manager{
		projectDir: projectDir,
		config:     cfg,
	}
}

// GetTree returns the file tree for configured directories
func (m *Manager) GetTree() ([]FileInfo, error) {
	var tree []FileInfo

	for _, dir := range m.config.ShowDirs {
		fullPath := filepath.Join(m.projectDir, dir)
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			continue
		}

		info, err := m.buildTree(fullPath, dir, 0)
		if err != nil {
			continue
		}
		tree = append(tree, info)
	}

	return tree, nil
}

// buildTree recursively builds the file tree
func (m *Manager) buildTree(fullPath, relativePath string, depth int) (FileInfo, error) {
	stat, err := os.Stat(fullPath)
	if err != nil {
		return FileInfo{}, err
	}

	info := FileInfo{
		Name:    filepath.Base(relativePath),
		Path:    relativePath,
		IsDir:   stat.IsDir(),
		ModTime: stat.ModTime().Unix(),
	}

	if !stat.IsDir() {
		info.Size = stat.Size()
		info.Type = getFileType(fullPath)
		return info, nil
	}

	// Read directory contents
	entries, err := os.ReadDir(fullPath)
	if err != nil {
		return info, nil
	}

	var children []FileInfo
	for _, entry := range entries {
		name := entry.Name()

		// Skip hidden files and directories
		if m.isHidden(name, entry.IsDir()) {
			continue
		}

		childPath := filepath.Join(fullPath, name)
		childRelPath := filepath.Join(relativePath, name)

		childInfo, err := m.buildTree(childPath, childRelPath, depth+1)
		if err != nil {
			continue
		}
		children = append(children, childInfo)
	}

	// Sort: directories first, then alphabetically
	sort.Slice(children, func(i, j int) bool {
		if children[i].IsDir != children[j].IsDir {
			return children[i].IsDir
		}
		return strings.ToLower(children[i].Name) < strings.ToLower(children[j].Name)
	})

	info.Children = children
	return info, nil
}

// ReadFile reads a file's content
func (m *Manager) ReadFile(relativePath string) (string, error) {
	if !m.isValidPath(relativePath) {
		return "", fmt.Errorf("invalid path: %s", relativePath)
	}

	fullPath := filepath.Join(m.projectDir, relativePath)
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return "", err
	}

	return string(content), nil
}

// WriteFile writes content to a file
func (m *Manager) WriteFile(relativePath, content string) error {
	if !m.isValidPath(relativePath) {
		return fmt.Errorf("invalid path: %s", relativePath)
	}

	fullPath := filepath.Join(m.projectDir, relativePath)

	// Ensure directory exists
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	return os.WriteFile(fullPath, []byte(content), 0644)
}

// CreateFile creates a new file
func (m *Manager) CreateFile(relativePath, content string) error {
	if !m.isValidPath(relativePath) {
		return fmt.Errorf("invalid path: %s", relativePath)
	}

	fullPath := filepath.Join(m.projectDir, relativePath)

	// Check if file already exists
	if _, err := os.Stat(fullPath); err == nil {
		return fmt.Errorf("file already exists: %s", relativePath)
	}

	return m.WriteFile(relativePath, content)
}

// DeleteFile deletes a file
func (m *Manager) DeleteFile(relativePath string) error {
	if !m.isValidPath(relativePath) {
		return fmt.Errorf("invalid path: %s", relativePath)
	}

	fullPath := filepath.Join(m.projectDir, relativePath)
	return os.Remove(fullPath)
}

// RenameFile renames/moves a file
func (m *Manager) RenameFile(oldPath, newPath string) error {
	if !m.isValidPath(oldPath) || !m.isValidPath(newPath) {
		return fmt.Errorf("invalid path")
	}

	oldFull := filepath.Join(m.projectDir, oldPath)
	newFull := filepath.Join(m.projectDir, newPath)

	// Ensure target directory exists
	dir := filepath.Dir(newFull)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	return os.Rename(oldFull, newFull)
}

// CreateDir creates a new directory
func (m *Manager) CreateDir(relativePath string) error {
	if !m.isValidPath(relativePath) {
		return fmt.Errorf("invalid path: %s", relativePath)
	}

	fullPath := filepath.Join(m.projectDir, relativePath)
	return os.MkdirAll(fullPath, 0755)
}

// CopyFile copies a file
func (m *Manager) CopyFile(srcPath, dstPath string) error {
	if !m.isValidPath(srcPath) || !m.isValidPath(dstPath) {
		return fmt.Errorf("invalid path")
	}

	srcFull := filepath.Join(m.projectDir, srcPath)
	dstFull := filepath.Join(m.projectDir, dstPath)

	src, err := os.Open(srcFull)
	if err != nil {
		return err
	}
	defer src.Close()

	// Ensure target directory exists
	dir := filepath.Dir(dstFull)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	dst, err := os.Create(dstFull)
	if err != nil {
		return err
	}
	defer dst.Close()

	_, err = io.Copy(dst, src)
	return err
}

// GetFileInfo returns info about a specific file
func (m *Manager) GetFileInfo(relativePath string) (*FileInfo, error) {
	if !m.isValidPath(relativePath) {
		return nil, fmt.Errorf("invalid path: %s", relativePath)
	}

	fullPath := filepath.Join(m.projectDir, relativePath)
	stat, err := os.Stat(fullPath)
	if err != nil {
		return nil, err
	}

	return &FileInfo{
		Name:    filepath.Base(relativePath),
		Path:    relativePath,
		IsDir:   stat.IsDir(),
		Size:    stat.Size(),
		ModTime: stat.ModTime().Unix(),
		Type:    getFileType(fullPath),
	}, nil
}

// ListDataFiles returns files from a specific data directory (for shortcode file selectors)
func (m *Manager) ListDataFiles(dataType string) ([]FileInfo, error) {
	var results []FileInfo

	// Common data directories
	dataDirs := map[string][]string{
		"personas":     {"content/personas"},
		"institutions": {"content/instituciones", "content/institutions"},
		"all":          {"content"},
	}

	dirs, ok := dataDirs[dataType]
	if !ok {
		dirs = []string{filepath.Join("content", dataType)}
	}

	for _, dir := range dirs {
		fullPath := filepath.Join(m.projectDir, dir)
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			continue
		}

		err := filepath.Walk(fullPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.IsDir() {
				return nil
			}

			ext := strings.ToLower(filepath.Ext(path))
			if ext == ".md" || ext == ".html" {
				relPath, _ := filepath.Rel(m.projectDir, path)
				// Remove extension for Hugo page references
				refPath := strings.TrimSuffix(relPath, ext)
				refPath = strings.TrimPrefix(refPath, "content/")

				results = append(results, FileInfo{
					Name: filepath.Base(path),
					Path: refPath,
				})
			}
			return nil
		})
		if err != nil {
			continue
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return strings.ToLower(results[i].Name) < strings.ToLower(results[j].Name)
	})

	return results, nil
}

// Exists checks if a file exists
func (m *Manager) Exists(relativePath string) bool {
	fullPath := filepath.Join(m.projectDir, relativePath)
	_, err := os.Stat(fullPath)
	return err == nil
}

// isValidPath checks if a path is safe (no directory traversal)
func (m *Manager) isValidPath(relativePath string) bool {
	// Clean the path
	cleaned := filepath.Clean(relativePath)

	// Check for directory traversal
	if strings.HasPrefix(cleaned, "..") || strings.Contains(cleaned, ".."+string(filepath.Separator)) {
		return false
	}

	// Ensure it's within project directory
	fullPath := filepath.Join(m.projectDir, cleaned)
	absProject, _ := filepath.Abs(m.projectDir)
	absPath, _ := filepath.Abs(fullPath)

	return strings.HasPrefix(absPath, absProject)
}

func (m *Manager) isHidden(name string, isDir bool) bool {
	// Skip dot files
	if strings.HasPrefix(name, ".") {
		return true
	}

	if isDir {
		for _, hidden := range m.config.HiddenDirs {
			if name == hidden {
				return true
			}
		}
	} else {
		for _, hidden := range m.config.HiddenFiles {
			if name == hidden {
				return true
			}
		}
	}

	return false
}

func getFileType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".md", ".markdown":
		return "markdown"
	case ".html", ".htm":
		return "html"
	case ".yaml", ".yml":
		return "yaml"
	case ".toml":
		return "toml"
	case ".json":
		return "json"
	case ".js":
		return "javascript"
	case ".css":
		return "css"
	case ".scss", ".sass":
		return "scss"
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".svg":
		return "image"
	case ".go":
		return "go"
	default:
		return "text"
	}
}
