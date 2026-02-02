package images

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	_ "image/gif"

	"github.com/fernandezvara/hugo-manager/internal/config"
)

// Processor handles image operations
type Processor struct {
	projectDir string
	config     config.ImagesConfig
}

// ProcessedImage represents a processed image variant
type ProcessedImage struct {
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	Path     string `json:"path"`
	URL      string `json:"url"`
	Size     int64  `json:"size"`
	Filename string `json:"filename"`
}

// ProcessResult contains all generated image variants
type ProcessResult struct {
	Original  string           `json:"original"`
	Variants  []ProcessedImage `json:"variants"`
	Srcset    string           `json:"srcset"`
	Shortcode string           `json:"shortcode"`
	HTML      string           `json:"html"`
}

// UploadOptions contains options for image upload
type UploadOptions struct {
	Folder     string `json:"folder"`
	Filename   string `json:"filename"`
	Quality    int    `json:"quality"`
	Widths     []int  `json:"widths"`
	PresetName string `json:"presetName"`
}

// FolderInfo represents an image folder
type FolderInfo struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// NewProcessor creates a new image processor
func NewProcessor(projectDir string, cfg config.ImagesConfig) *Processor {
	return &Processor{
		projectDir: projectDir,
		config:     cfg,
	}
}

// GetFolders returns available image folders from common locations
func (p *Processor) GetFolders() []FolderInfo {
	var folders []FolderInfo

	// Common image directories to scan
	commonDirs := []string{
		"static/images",
		"assets/images",
		"static/img",
		"assets/img",
	}

	for _, dir := range commonDirs {
		fullPath := filepath.Join(p.projectDir, dir)
		if entries, err := os.ReadDir(fullPath); err == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					name := entry.Name()
					folders = append(folders, FolderInfo{
						Name: name,
						Path: filepath.Join(dir, name),
					})
				}
			}
		}
	}

	// Sort alphabetically
	sort.Slice(folders, func(i, j int) bool {
		return folders[i].Name < folders[j].Name
	})

	return folders
}

// GetPresets returns available image presets
func (p *Processor) GetPresets() []config.ImagePreset {
	return p.config.Presets
}

// Process processes an uploaded image
func (p *Processor) Process(reader io.Reader, opts UploadOptions) (*ProcessResult, error) {
	// Set defaults
	if opts.Quality <= 0 {
		opts.Quality = p.config.DefaultQuality
	}
	if len(opts.Widths) == 0 {
		opts.Widths = []int{1920} // Default to single full-size
	}

	// Decode the image
	img, format, err := image.Decode(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %w", err)
	}

	bounds := img.Bounds()
	origWidth := bounds.Dx()
	origHeight := bounds.Dy()

	// Determine output format
	outputFormat := p.config.OutputFormat
	if outputFormat == "" {
		outputFormat = format
	}

	// Create output directory
	if opts.Folder == "" {
		return nil, fmt.Errorf("folder is required for image upload")
	}

	// Use folder directly (always a complete path)
	outputDir := filepath.Join(p.projectDir, opts.Folder)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	// Sanitize filename
	baseName := sanitizeFilename(opts.Filename)
	if baseName == "" {
		baseName = "image"
	}
	// Remove extension if present
	baseName = strings.TrimSuffix(baseName, filepath.Ext(baseName))

	result := &ProcessResult{
		Variants: []ProcessedImage{},
	}

	// Sort widths descending for srcset
	sort.Sort(sort.Reverse(sort.IntSlice(opts.Widths)))

	// Process each width
	for _, targetWidth := range opts.Widths {
		// Skip if target width is larger than original
		if targetWidth > origWidth {
			targetWidth = origWidth
		}

		// Calculate height maintaining aspect ratio
		targetHeight := int(float64(origHeight) * float64(targetWidth) / float64(origWidth))

		// Resize the image
		resized := resize(img, targetWidth, targetHeight)

		// Generate filename
		ext := getExtension(outputFormat)
		filename := fmt.Sprintf("%s.%dx%d%s", baseName, targetWidth, targetHeight, ext)
		outputPath := filepath.Join(outputDir, filename)

		// Save the image
		if err := saveImage(resized, outputPath, outputFormat, opts.Quality); err != nil {
			return nil, fmt.Errorf("failed to save image %s: %w", filename, err)
		}

		// Get file size
		stat, _ := os.Stat(outputPath)
		size := int64(0)
		if stat != nil {
			size = stat.Size()
		}

		// Calculate URL path
		relPath, _ := filepath.Rel(p.projectDir, outputPath)
		relPath = strings.TrimPrefix(relPath, "static")
		urlPath := "/" + strings.ReplaceAll(relPath, "\\", "/")

		variant := ProcessedImage{
			Width:    targetWidth,
			Height:   targetHeight,
			Path:     relPath,
			URL:      urlPath,
			Size:     size,
			Filename: filename,
		}
		result.Variants = append(result.Variants, variant)

		// First (largest) is the original reference
		if result.Original == "" {
			result.Original = urlPath
		}
	}

	// Generate srcset string
	result.Srcset = p.generateSrcset(result.Variants)

	// Generate shortcode
	result.Shortcode = p.generateShortcode(baseName, result)

	// Generate raw HTML
	result.HTML = p.generateHTML(baseName, result)

	return result, nil
}

func (p *Processor) BuildResultFromProcessedVariants(selectedPath string) (*ProcessResult, error) {
	selectedPath = filepath.ToSlash(strings.TrimSpace(selectedPath))
	if selectedPath == "" {
		return nil, fmt.Errorf("path is required")
	}

	fullSelectedPath := filepath.Join(p.projectDir, filepath.FromSlash(selectedPath))
	stat, err := os.Stat(fullSelectedPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat selected image: %w", err)
	}
	if stat.IsDir() {
		return nil, fmt.Errorf("selected path is a directory")
	}

	dirAbs := filepath.Dir(fullSelectedPath)
	fileName := filepath.Base(fullSelectedPath)
	fileExt := strings.ToLower(filepath.Ext(fileName))

	variantRe := regexp.MustCompile(`^(?P<base>.+)\.(?P<w>\d+)x(?P<h>\d+)(?P<ext>\.[^.]+)$`)
	m := variantRe.FindStringSubmatch(fileName)

	baseName := strings.TrimSuffix(fileName, filepath.Ext(fileName))
	if m != nil {
		baseName = m[variantRe.SubexpIndex("base")]
		fileExt = strings.ToLower(m[variantRe.SubexpIndex("ext")])
	}

	entries, err := os.ReadDir(dirAbs)
	if err != nil {
		return nil, fmt.Errorf("failed to read image directory: %w", err)
	}

	result := &ProcessResult{Variants: []ProcessedImage{}}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.ToLower(filepath.Ext(name)) != fileExt {
			continue
		}
		mm := variantRe.FindStringSubmatch(name)
		if mm == nil {
			continue
		}
		entryBase := mm[variantRe.SubexpIndex("base")]
		if entryBase != baseName {
			continue
		}
		w, errW := strconv.Atoi(mm[variantRe.SubexpIndex("w")])
		h, errH := strconv.Atoi(mm[variantRe.SubexpIndex("h")])
		if errW != nil || errH != nil || w <= 0 || h <= 0 {
			continue
		}

		absPath := filepath.Join(dirAbs, name)
		st, err := os.Stat(absPath)
		if err != nil {
			continue
		}

		relPath, _ := filepath.Rel(p.projectDir, absPath)
		relPath = strings.TrimPrefix(relPath, "static")
		urlPath := strings.ReplaceAll(filepath.ToSlash(relPath), "\\", "/")

		result.Variants = append(result.Variants, ProcessedImage{
			Width:    w,
			Height:   h,
			Path:     filepath.ToSlash(relPath),
			URL:      urlPath,
			Size:     st.Size(),
			Filename: name,
		})
	}

	if len(result.Variants) == 0 {
		relPath, _ := filepath.Rel(p.projectDir, fullSelectedPath)
		relPath = strings.TrimPrefix(relPath, "static")
		urlPath := strings.ReplaceAll(filepath.ToSlash(relPath), "\\", "/")
		result.Variants = append(result.Variants, ProcessedImage{
			Width:    0,
			Height:   0,
			Path:     filepath.ToSlash(relPath),
			URL:      urlPath,
			Size:     stat.Size(),
			Filename: filepath.Base(relPath),
		})
	}

	sort.Slice(result.Variants, func(i, j int) bool {
		if result.Variants[i].Width == result.Variants[j].Width {
			return result.Variants[i].Filename < result.Variants[j].Filename
		}
		return result.Variants[i].Width > result.Variants[j].Width
	})

	result.Original = result.Variants[0].URL
	result.Srcset = p.generateSrcset(result.Variants)
	result.Shortcode = p.generateShortcode(baseName, result)
	result.HTML = p.generateHTML(baseName, result)

	return result, nil
}

// ProcessExistingImage processes an existing image file with the given options
func (p *Processor) ProcessExistingImage(sourcePath string, opts UploadOptions) (*ProcessResult, error) {
	// Open the existing image file
	file, err := os.Open(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open source image: %w", err)
	}
	defer file.Close()

	// Decode the image
	img, format, err := image.Decode(file)
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %w", err)
	}

	// Get image dimensions
	origWidth := img.Bounds().Dx()
	origHeight := img.Bounds().Dy()

	// Create output directory
	outputDir := filepath.Join(p.projectDir, opts.Folder)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	// Extract base name from filename (without extension)
	baseName := strings.TrimSuffix(opts.Filename, filepath.Ext(opts.Filename))

	// Determine output format
	outputFormat := p.config.OutputFormat
	if outputFormat == "" {
		outputFormat = format
	}

	// Initialize result
	result := &ProcessResult{
		Variants: []ProcessedImage{},
	}

	// Process each width
	for _, targetWidth := range opts.Widths {
		// Skip if target width is larger than original
		if targetWidth > origWidth {
			targetWidth = origWidth
		}

		// Calculate height maintaining aspect ratio
		targetHeight := int(float64(origHeight) * float64(targetWidth) / float64(origWidth))

		// Resize the image
		resized := resize(img, targetWidth, targetHeight)

		// Generate filename
		ext := getExtension(outputFormat)
		filename := fmt.Sprintf("%s.%dx%d%s", baseName, targetWidth, targetHeight, ext)
		outputPath := filepath.Join(outputDir, filename)

		// Save the image
		if err := saveImage(resized, outputPath, outputFormat, opts.Quality); err != nil {
			return nil, fmt.Errorf("failed to save image %s: %w", filename, err)
		}

		// Get file size
		stat, _ := os.Stat(outputPath)
		size := int64(0)
		if stat != nil {
			size = stat.Size()
		}

		// Calculate URL path
		relPath, _ := filepath.Rel(p.projectDir, outputPath)
		relPath = strings.TrimPrefix(relPath, "static")
		urlPath := "/" + strings.ReplaceAll(relPath, "\\", "/")

		variant := ProcessedImage{
			Width:    targetWidth,
			Height:   targetHeight,
			Path:     relPath,
			URL:      urlPath,
			Size:     size,
			Filename: filename,
		}
		result.Variants = append(result.Variants, variant)

		// First (largest) is the original reference
		if result.Original == "" {
			result.Original = urlPath
		}
	}

	// Generate srcset string
	result.Srcset = p.generateSrcset(result.Variants)

	// Generate shortcode
	result.Shortcode = p.generateShortcode(baseName, result)

	// Generate raw HTML
	result.HTML = p.generateHTML(baseName, result)

	return result, nil
}

// generateSrcset creates the srcset attribute value
func (p *Processor) generateSrcset(variants []ProcessedImage) string {
	var parts []string
	for _, v := range variants {
		parts = append(parts, fmt.Sprintf("%s %dw", v.URL, v.Width))
	}
	return strings.Join(parts, ", ")
}

// generateShortcode creates a responsive image shortcode
func (p *Processor) generateShortcode(baseName string, result *ProcessResult) string {
	if len(result.Variants) == 0 {
		return ""
	}

	// Use the largest variant as the default src
	largest := result.Variants[0]

	// If only one variant, simple shortcode
	if len(result.Variants) == 1 {
		return fmt.Sprintf(`{{< img src="%s" alt="%s" >}}`,
			largest.URL,
			baseName)
	}

	// Multiple variants - include srcset
	return fmt.Sprintf(`{{< img src="%s" alt="%s" srcset="%s" >}}`,
		largest.URL,
		baseName,
		result.Srcset)
}

// generateHTML creates a raw HTML img tag with srcset
func (p *Processor) generateHTML(baseName string, result *ProcessResult) string {
	if len(result.Variants) == 0 {
		return ""
	}

	largest := result.Variants[0]

	if len(result.Variants) == 1 {
		return fmt.Sprintf(`<img src="%s" alt="%s" loading="lazy" decoding="async">`,
			largest.URL,
			baseName)
	}

	return fmt.Sprintf(`<img src="%s" srcset="%s" sizes="(max-width: 640px) 100vw, (max-width: 1024px) 75vw, 50vw" alt="%s" loading="lazy" decoding="async">`,
		largest.URL,
		result.Srcset,
		baseName)
}

// DeleteImage deletes an image and all its variants
func (p *Processor) DeleteImage(imagePath string) error {
	fullPath := filepath.Join(p.projectDir, imagePath)
	return os.Remove(fullPath)
}

// resize uses bilinear interpolation to resize an image
func resize(src image.Image, width, height int) image.Image {
	srcBounds := src.Bounds()
	srcW := srcBounds.Dx()
	srcH := srcBounds.Dy()

	dst := image.NewRGBA(image.Rect(0, 0, width, height))

	xRatio := float64(srcW) / float64(width)
	yRatio := float64(srcH) / float64(height)

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			srcX := float64(x) * xRatio
			srcY := float64(y) * yRatio

			x0 := int(srcX)
			y0 := int(srcY)
			x1 := x0 + 1
			y1 := y0 + 1

			if x1 >= srcW {
				x1 = srcW - 1
			}
			if y1 >= srcH {
				y1 = srcH - 1
			}

			xFrac := srcX - float64(x0)
			yFrac := srcY - float64(y0)

			r00, g00, b00, a00 := src.At(srcBounds.Min.X+x0, srcBounds.Min.Y+y0).RGBA()
			r10, g10, b10, a10 := src.At(srcBounds.Min.X+x1, srcBounds.Min.Y+y0).RGBA()
			r01, g01, b01, a01 := src.At(srcBounds.Min.X+x0, srcBounds.Min.Y+y1).RGBA()
			r11, g11, b11, a11 := src.At(srcBounds.Min.X+x1, srcBounds.Min.Y+y1).RGBA()

			r := bilinear(r00, r10, r01, r11, xFrac, yFrac)
			g := bilinear(g00, g10, g01, g11, xFrac, yFrac)
			b := bilinear(b00, b10, b01, b11, xFrac, yFrac)
			a := bilinear(a00, a10, a01, a11, xFrac, yFrac)

			dst.Set(x, y, color.RGBA{
				R: uint8(r >> 8),
				G: uint8(g >> 8),
				B: uint8(b >> 8),
				A: uint8(a >> 8),
			})
		}
	}

	return dst
}

func bilinear(v00, v10, v01, v11 uint32, xFrac, yFrac float64) uint32 {
	top := float64(v00)*(1-xFrac) + float64(v10)*xFrac
	bottom := float64(v01)*(1-xFrac) + float64(v11)*xFrac
	return uint32(top*(1-yFrac) + bottom*yFrac)
}

// cropAndResize crops to aspect ratio then resizes
func cropAndResize(src image.Image, width, height int) image.Image {
	srcBounds := src.Bounds()
	srcW := srcBounds.Dx()
	srcH := srcBounds.Dy()

	targetRatio := float64(width) / float64(height)
	srcRatio := float64(srcW) / float64(srcH)

	var cropRect image.Rectangle

	if srcRatio > targetRatio {
		cropHeight := srcH
		cropWidth := int(float64(cropHeight) * targetRatio)
		xOffset := (srcW - cropWidth) / 2
		cropRect = image.Rect(srcBounds.Min.X+xOffset, srcBounds.Min.Y,
			srcBounds.Min.X+xOffset+cropWidth, srcBounds.Min.Y+cropHeight)
	} else {
		cropWidth := srcW
		cropHeight := int(float64(cropWidth) / targetRatio)
		yOffset := (srcH - cropHeight) / 2
		cropRect = image.Rect(srcBounds.Min.X, srcBounds.Min.Y+yOffset,
			srcBounds.Min.X+cropWidth, srcBounds.Min.Y+yOffset+cropHeight)
	}

	cropped := image.NewRGBA(image.Rect(0, 0, cropRect.Dx(), cropRect.Dy()))
	draw.Draw(cropped, cropped.Bounds(), src, cropRect.Min, draw.Src)

	return resize(cropped, width, height)
}

func saveImage(img image.Image, path, format string, quality int) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	switch strings.ToLower(format) {
	case "jpeg", "jpg":
		return jpeg.Encode(file, img, &jpeg.Options{Quality: quality})
	case "png":
		return png.Encode(file, img)
	default:
		return jpeg.Encode(file, img, &jpeg.Options{Quality: quality})
	}
}

func getExtension(format string) string {
	switch strings.ToLower(format) {
	case "jpeg", "jpg":
		return ".jpg"
	case "png":
		return ".png"
	case "gif":
		return ".gif"
	case "webp":
		return ".webp"
	default:
		return ".jpg"
	}
}

func sanitizeFilename(name string) string {
	// Replace spaces with hyphens
	name = strings.ReplaceAll(name, " ", "-")

	// Remove or replace problematic characters
	replacer := strings.NewReplacer(
		"á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u",
		"Á", "A", "É", "E", "Í", "I", "Ó", "O", "Ú", "U",
		"ñ", "n", "Ñ", "N", "ü", "u", "Ü", "U",
	)
	name = replacer.Replace(name)

	// Keep only alphanumeric, hyphens, underscores, and dots
	var result strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			result.WriteRune(r)
		}
	}

	return strings.ToLower(result.String())
}
