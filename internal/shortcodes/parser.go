package shortcodes

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Shortcode represents a detected Hugo shortcode
type Shortcode struct {
	Name        string      `json:"name"`
	File        string      `json:"file"`
	Parameters  []Parameter `json:"parameters"`
	HasInner    bool        `json:"hasInner"`
	InnerHint   string      `json:"innerHint,omitempty"`
	Description string      `json:"description,omitempty"`
	Template    string      `json:"template"`
}

// Parameter represents a shortcode parameter
type Parameter struct {
	Name         string `json:"name"`
	Type         string `json:"type"` // "string", "boolean", "file", "number"
	Required     bool   `json:"required"`
	Default      string `json:"default,omitempty"`
	Description  string `json:"description,omitempty"`
	FileType     string `json:"fileType,omitempty"` // for file parameters: "personas", "institutions", etc.
	Placeholder  string `json:"placeholder,omitempty"`
}

// Parser handles shortcode detection
type Parser struct {
	projectDir string
}

// NewParser creates a new shortcode parser
func NewParser(projectDir string) *Parser {
	return &Parser{projectDir: projectDir}
}

// Regular expressions for parsing Hugo templates
var (
	// Match .Get "param" or .Get `param`
	getParamRe = regexp.MustCompile(`\.Get\s+["'\x60]([^"'\x60]+)["'\x60]`)
	
	// Match | default "value" or | default true/false
	defaultRe = regexp.MustCompile(`\|\s*default\s+["'\x60]?([^"'\x60}\s|]+)["'\x60]?`)
	
	// Match $varName := .Get "param" | default ...
	varAssignRe = regexp.MustCompile(`\$(\w+)\s*:?=\s*\.Get\s+["'\x60]([^"'\x60]+)["'\x60](?:\s*\|\s*default\s+["'\x60]?([^"'\x60}\s]+)["'\x60]?)?`)
	
	// Match .Inner
	innerRe = regexp.MustCompile(`\.Inner`)
	
	// Match {{ with .Get "param" }} patterns (required params)
	withGetRe = regexp.MustCompile(`{{\s*with\s+\.Get\s+["'\x60]([^"'\x60]+)["'\x60]\s*}}`)
	
	// Match {{ if .Get "param" }} patterns  
	ifGetRe = regexp.MustCompile(`{{\s*if\s+\.Get\s+["'\x60]([^"'\x60]+)["'\x60]\s*}}`)
)

// DetectAll scans the shortcodes directory and detects all shortcodes
func (p *Parser) DetectAll() ([]Shortcode, error) {
	shortcodesDir := filepath.Join(p.projectDir, "layouts", "shortcodes")
	
	if _, err := os.Stat(shortcodesDir); os.IsNotExist(err) {
		return []Shortcode{}, nil
	}

	var shortcodes []Shortcode

	entries, err := os.ReadDir(shortcodesDir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		ext := filepath.Ext(name)
		if ext != ".html" {
			continue
		}

		shortcodeName := strings.TrimSuffix(name, ext)
		filePath := filepath.Join(shortcodesDir, name)

		content, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}

		sc := p.parseShortcode(shortcodeName, name, string(content))
		shortcodes = append(shortcodes, sc)
	}

	// Sort alphabetically
	sort.Slice(shortcodes, func(i, j int) bool {
		return shortcodes[i].Name < shortcodes[j].Name
	})

	return shortcodes, nil
}

// parseShortcode parses a single shortcode template
func (p *Parser) parseShortcode(name, file, content string) Shortcode {
	sc := Shortcode{
		Name:       name,
		File:       file,
		Parameters: []Parameter{},
		HasInner:   innerRe.MatchString(content),
	}

	// Track parameters we've seen
	params := make(map[string]*Parameter)

	// Find all variable assignments with .Get
	varMatches := varAssignRe.FindAllStringSubmatch(content, -1)
	for _, match := range varMatches {
		varName := match[1]
		paramName := match[2]
		defaultVal := ""
		if len(match) > 3 {
			defaultVal = match[3]
		}

		param := &Parameter{
			Name:    paramName,
			Default: defaultVal,
		}

		// Infer type from variable name and default value
		param.Type = inferType(varName, paramName, defaultVal)
		param.FileType = inferFileType(paramName, varName)
		param.Placeholder = generatePlaceholder(paramName, param.Type, param.FileType)
		param.Description = generateDescription(paramName)

		params[paramName] = param
	}

	// Find standalone .Get calls
	getMatches := getParamRe.FindAllStringSubmatch(content, -1)
	for _, match := range getMatches {
		paramName := match[1]
		if _, exists := params[paramName]; !exists {
			param := &Parameter{
				Name:        paramName,
				Type:        inferType("", paramName, ""),
				Placeholder: generatePlaceholder(paramName, "string", ""),
				Description: generateDescription(paramName),
			}
			param.FileType = inferFileType(paramName, "")
			params[paramName] = param
		}
	}

	// Check for required params (used with 'with' statement)
	withMatches := withGetRe.FindAllStringSubmatch(content, -1)
	for _, match := range withMatches {
		paramName := match[1]
		if param, exists := params[paramName]; exists {
			param.Required = true
		}
	}

	// Check for params that have defaults (not required)
	for _, param := range params {
		if param.Default != "" {
			param.Required = false
		}
	}

	// Convert map to slice, sorted by: required first, then alphabetically
	for _, param := range params {
		sc.Parameters = append(sc.Parameters, *param)
	}
	sort.Slice(sc.Parameters, func(i, j int) bool {
		if sc.Parameters[i].Required != sc.Parameters[j].Required {
			return sc.Parameters[i].Required
		}
		return sc.Parameters[i].Name < sc.Parameters[j].Name
	})

	// Generate inner hint if applicable
	if sc.HasInner {
		sc.InnerHint = generateInnerHint(name)
	}

	// Generate template
	sc.Template = p.generateTemplate(sc)

	return sc
}

// inferType infers the parameter type from naming conventions
func inferType(varName, paramName, defaultVal string) string {
	lowerVar := strings.ToLower(varName)
	lowerParam := strings.ToLower(paramName)

	// Check for boolean indicators
	boolPrefixes := []string{"show", "hide", "is", "has", "enable", "disable", "use", "allow"}
	for _, prefix := range boolPrefixes {
		if strings.HasPrefix(lowerVar, prefix) || strings.HasPrefix(lowerParam, prefix) {
			return "boolean"
		}
	}

	// Check default value
	if defaultVal == "true" || defaultVal == "false" {
		return "boolean"
	}

	// Check for file indicators
	fileSuffixes := []string{"file", "path", "src", "image", "photo"}
	for _, suffix := range fileSuffixes {
		if strings.HasSuffix(lowerParam, suffix) || strings.Contains(lowerParam, suffix) {
			return "file"
		}
	}

	// Check for number indicators
	numIndicators := []string{"width", "height", "size", "count", "number", "index", "level"}
	for _, ind := range numIndicators {
		if strings.Contains(lowerParam, ind) {
			return "number"
		}
	}

	return "string"
}

// inferFileType determines what kind of file a parameter expects
func inferFileType(paramName, varName string) string {
	lower := strings.ToLower(paramName + varName)
	
	if strings.Contains(lower, "user") || strings.Contains(lower, "person") || 
	   strings.Contains(lower, "member") || strings.Contains(lower, "author") {
		return "personas"
	}
	if strings.Contains(lower, "institution") || strings.Contains(lower, "org") ||
	   strings.Contains(lower, "company") {
		return "institutions"
	}
	if strings.Contains(lower, "image") || strings.Contains(lower, "photo") ||
	   strings.Contains(lower, "src") {
		return "images"
	}
	
	return ""
}

// generatePlaceholder creates a helpful placeholder for the parameter
func generatePlaceholder(paramName, paramType, fileType string) string {
	switch paramType {
	case "boolean":
		return "true"
	case "number":
		return "0"
	case "file":
		switch fileType {
		case "personas":
			return "personas/nombre-apellido"
		case "institutions":
			return "instituciones/nombre"
		case "images":
			return "/images/example.jpg"
		default:
			return "path/to/file"
		}
	default:
		// Generate contextual placeholders
		lower := strings.ToLower(paramName)
		switch {
		case strings.Contains(lower, "class"):
			return "css-class"
		case strings.Contains(lower, "type"):
			return "primary"
		case strings.Contains(lower, "href") || strings.Contains(lower, "link") || strings.Contains(lower, "url"):
			return "https://example.com"
		case strings.Contains(lower, "alt"):
			return "Descripción de la imagen"
		case strings.Contains(lower, "title"):
			return "Título"
		case strings.Contains(lower, "caption"):
			return "Pie de imagen"
		default:
			return paramName
		}
	}
}

// generateDescription creates a description for the parameter
func generateDescription(paramName string) string {
	descriptions := map[string]string{
		"file":           "Ruta al archivo de datos",
		"src":            "URL o ruta de la imagen",
		"alt":            "Texto alternativo para accesibilidad",
		"class":          "Clases CSS adicionales",
		"type":           "Tipo de elemento (primary, secondary, etc.)",
		"href":           "URL de destino",
		"link":           "URL de destino",
		"title":          "Título del elemento",
		"caption":        "Pie de imagen o descripción",
		"width":          "Ancho en píxeles",
		"height":         "Alto en píxeles",
		"show_photo":     "Mostrar foto",
		"show_name":      "Mostrar nombre",
		"show_bio":       "Mostrar biografía",
		"show_position":  "Mostrar cargo/posición",
		"show_contact":   "Mostrar información de contacto",
		"show_institution": "Mostrar institución",
		"target":         "Destino del enlace (_blank, _self, etc.)",
		"rel":            "Atributo rel del enlace",
		"loading":        "Estrategia de carga (lazy, eager)",
	}

	if desc, ok := descriptions[strings.ToLower(paramName)]; ok {
		return desc
	}
	return ""
}

// generateInnerHint creates a hint for the inner content
func generateInnerHint(shortcodeName string) string {
	hints := map[string]string{
		"alert":   "Tu mensaje de alerta va aquí...",
		"button":  "Texto del botón",
		"cards":   "Contenido de las tarjetas",
		"figure":  "",
		"note":    "Tu nota va aquí...",
		"warning": "Tu advertencia va aquí...",
		"info":    "Tu información va aquí...",
		"quote":   "Texto de la cita",
		"code":    "// Tu código aquí",
	}

	if hint, ok := hints[shortcodeName]; ok {
		return hint
	}
	return "Contenido..."
}

// generateTemplate creates a ready-to-use shortcode template
func (p *Parser) generateTemplate(sc Shortcode) string {
	var sb strings.Builder

	// Opening tag
	sb.WriteString("{{< ")
	sb.WriteString(sc.Name)

	// Parameters
	for _, param := range sc.Parameters {
		sb.WriteString(fmt.Sprintf(` %s="%s"`, param.Name, param.Placeholder))
	}

	if sc.HasInner {
		sb.WriteString(" >}}")
		if sc.InnerHint != "" {
			sb.WriteString(sc.InnerHint)
		}
		sb.WriteString("{{< /")
		sb.WriteString(sc.Name)
		sb.WriteString(" >}}")
	} else {
		sb.WriteString(" >}}")
	}

	return sb.String()
}

// GetShortcode returns a specific shortcode by name
func (p *Parser) GetShortcode(name string) (*Shortcode, error) {
	shortcodes, err := p.DetectAll()
	if err != nil {
		return nil, err
	}

	for _, sc := range shortcodes {
		if sc.Name == name {
			return &sc, nil
		}
	}

	return nil, fmt.Errorf("shortcode not found: %s", name)
}
