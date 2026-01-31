package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const ConfigFileName = "github.com/fernandezvara/hugo-manager.yaml"

// Config represents the hugo-manager configuration
type Config struct {
	Server   ServerConfig   `yaml:"server" json:"server"`
	Hugo     HugoConfig     `yaml:"hugo" json:"hugo"`
	Editor   EditorConfig   `yaml:"editor" json:"editor"`
	Images   ImagesConfig   `yaml:"images" json:"images"`
	FileTree FileTreeConfig `yaml:"file_tree" json:"file_tree"`
}

type ServerConfig struct {
	Port int `yaml:"port" json:"port"`
}

type HugoConfig struct {
	Port              int      `yaml:"port" json:"port"`
	AutoStart         bool     `yaml:"auto_start" json:"auto_start"`
	AdditionalArgs    []string `yaml:"additional_args" json:"additional_args"`
	DisableFastRender bool     `yaml:"disable_fast_render" json:"disable_fast_render"`
}

type EditorConfig struct {
	Theme         string `yaml:"theme" json:"theme"`
	FontSize      int    `yaml:"font_size" json:"font_size"`
	TabSize       int    `yaml:"tab_size" json:"tab_size"`
	WordWrap      bool   `yaml:"word_wrap" json:"word_wrap"`
	LineNumbers   bool   `yaml:"line_numbers" json:"line_numbers"`
	AutoSave      bool   `yaml:"auto_save" json:"auto_save"`
	AutoSaveDelay int    `yaml:"auto_save_delay" json:"auto_save_delay"`
}

type ImagesConfig struct {
	BaseDir        string        `yaml:"base_dir" json:"base_dir"`
	DefaultQuality int           `yaml:"default_quality" json:"default_quality"`
	Presets        []ImagePreset `yaml:"presets" json:"presets"`
	OutputFormat   string        `yaml:"output_format" json:"output_format"`
	Folders        []string      `yaml:"folders" json:"folders"`
}

type ImagePreset struct {
	Name   string `yaml:"name" json:"name"`
	Widths []int  `yaml:"widths" json:"widths"`
}

type FileTreeConfig struct {
	ShowDirs    []string `yaml:"show_dirs" json:"show_dirs"`
	HiddenFiles []string `yaml:"hidden_files" json:"hidden_files"`
	HiddenDirs  []string `yaml:"hidden_dirs" json:"hidden_dirs"`
}

// Default returns a default configuration
func Default() *Config {
	return &Config{
		Server: ServerConfig{
			Port: 8080,
		},
		Hugo: HugoConfig{
			Port:              1313,
			AutoStart:         true,
			DisableFastRender: true,
			AdditionalArgs:    []string{"--bind", "0.0.0.0"},
		},
		Editor: EditorConfig{
			Theme:         "one-dark",
			FontSize:      14,
			TabSize:       2,
			WordWrap:      true,
			LineNumbers:   true,
			AutoSave:      false,
			AutoSaveDelay: 1000,
		},
		Images: ImagesConfig{
			BaseDir:        "static/images",
			DefaultQuality: 85,
			Presets: []ImagePreset{
				{Name: "Full responsive", Widths: []int{320, 640, 1024, 1920}},
				{Name: "Mobile only", Widths: []int{320, 640}},
				{Name: "Desktop only", Widths: []int{1024, 1920}},
				{Name: "Thumbnail", Widths: []int{150, 300}},
				{Name: "Social media", Widths: []int{1200}},
				{Name: "Custom", Widths: []int{}},
			},
			OutputFormat: "jpg",
			Folders: []string{
				"personas",
				"blog",
				"general",
				"institutions",
			},
		},
		FileTree: FileTreeConfig{
			ShowDirs: []string{
				"content",
				"layouts/shortcodes",
				"static",
				"data",
			},
			HiddenFiles: []string{
				".DS_Store",
				"Thumbs.db",
				".gitignore",
			},
			HiddenDirs: []string{
				".git",
				"node_modules",
				"public",
				"resources",
			},
		},
	}
}

// Load loads the configuration from the project directory
func Load(projectDir string) (*Config, error) {
	configPath := filepath.Join(projectDir, ConfigFileName)

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	cfg := Default()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Save saves the configuration to the project directory
func Save(projectDir string, cfg *Config) error {
	configPath := filepath.Join(projectDir, ConfigFileName)

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}

	header := []byte(`# Hugo Manager Configuration
# See documentation at https://github.com/your-repo/hugo-manager

`)
	data = append(header, data...)

	return os.WriteFile(configPath, data, 0644)
}

// GetConfigPath returns the path to the config file
func GetConfigPath(projectDir string) string {
	return filepath.Join(projectDir, ConfigFileName)
}
