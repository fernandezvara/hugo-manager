# Hugo Manager

A web-based management interface for Hugo static sites. Edit content, manage images, and preview changes in real-time.

![Hugo Manager Interface](docs/screenshot.png)

## Features

- **Monaco Editor** - Full-featured code editor with syntax highlighting for Markdown, YAML, HTML, and more
- **Live Preview** - See your changes instantly in the integrated Hugo preview
- **Image Processing** - Upload images and automatically generate responsive srcset variants
- **Shortcode Detection** - Automatically detects your Hugo shortcodes and provides insertion helpers with parameter hints
- **File Management** - Browse, create, edit, and organize your content files
- **Hugo Control** - Start, stop, and restart Hugo server directly from the interface
- **Live Logs** - View Hugo server logs in real-time via WebSocket
- **Per-Project Config** - Customize settings per project with `hugo-manager.yaml`

## Installation

### From Source

```bash
# Clone the repository
git clone https://github.com/fernandezvara/hugo-manager.git
cd hugo-manager

# Build the binary
go build -o hugo-manager ./cmd/hugo-manager

# Or use the Makefile
make build # or make build-small
```

### Pre-built Binaries

Download from the [Releases](https://github.com/fernandezvara/hugo-manager/releases) page.

## Usage

```bash
# Navigate to your Hugo project
cd /path/to/your/hugo/site

# Run hugo-manager
hugo-manager

# Or specify the project directory
hugo-manager --dir /path/to/your/hugo/site
```

Open your browser at `http://localhost:8080`

### Command Line Options

```
Usage: hugo-manager [options]

Options:
  -port int        Port for the web interface (default 8080)
  -hugo-port int   Port for Hugo server (default 1313)
  -dir string      Hugo project directory (default ".")
  -init            Initialize hugo-manager.yaml config file
  -version         Show version
```

## Configuration

Hugo Manager can be configured per-project using `hugo-manager.yaml` in your project root.

Create a default config:

```bash
hugo-manager --init
```

### Configuration Options

```yaml
# Server settings
server:
  port: 8080

# Hugo server settings
hugo:
  port: 1313
  auto_start: true
  disable_fast_render: true
  additional_args:
    - "--bind"
    - "0.0.0.0"

# Editor settings
editor:
  theme: vs-dark # Monaco theme
  font_size: 14
  tab_size: 2
  word_wrap: true
  minimap: false

# Image processing
images:
  base_dir: static/images
  default_quality: 85
  output_format: jpg
  folders:
    - personas
    - blog
    - general
  presets:
    - name: Full responsive
      widths: [320, 640, 1024, 1920]
    - name: Mobile only
      widths: [320, 640]
    - name: Desktop only
      widths: [1024, 1920]
    - name: Thumbnail
      widths: [150, 300]
    - name: Social media
      widths: [1200]
    - name: Custom
      widths: []

# File tree configuration
file_tree:
  show_dirs:
    - content
    - layouts/shortcodes
    - static
    - data
  hidden_files:
    - .DS_Store
    - Thumbs.db
  hidden_dirs:
    - .git
    - node_modules
    - public
    - resources
```

## Responsive Images

Hugo Manager includes a responsive image workflow:

1. **Upload** - Drag and drop or select images
2. **Choose Preset** - Select from configured size presets
3. **Process** - Images are resized to multiple widths
4. **Copy Shortcode** - Get ready-to-use shortcode with srcset

### Image Shortcode

Add the included shortcode to your Hugo project:

```bash
cp examples/shortcodes/img.html layouts/shortcodes/
```

Usage in your content:

```markdown
{{< img src="/images/personas/john-doe" alt="John Doe" >}}
```

This generates:

```html
<img
  src="/images/personas/john-doe.1920x1080.jpg"
  srcset="
    /images/personas/john-doe.320x180.jpg    320w,
    /images/personas/john-doe.640x360.jpg    640w,
    /images/personas/john-doe.1024x576.jpg  1024w,
    /images/personas/john-doe.1920x1080.jpg 1920w
  "
  sizes="(max-width: 640px) 100vw, (max-width: 1024px) 75vw, 50vw"
  alt="John Doe"
  loading="lazy"
  decoding="async"
/>
```

## Shortcode Detection

Hugo Manager automatically detects shortcodes from your `layouts/shortcodes/` directory and:

- Parses parameters from `.Get "paramName"` calls
- Detects required vs optional parameters
- Infers parameter types (boolean, string, file, number)
- Identifies file parameters for dropdown selection
- Generates ready-to-use templates with placeholders

### Supported Parameter Detection

```html
{{/* These patterns are detected: */}} {{ $show := .Get "show" | default true }}
→ boolean, optional, default: true {{ with .Get "file" }} → required parameter
{{ $title := .Get "title" }} → string parameter {{ .Inner }} → shortcode accepts
inner content
```

## Keyboard Shortcuts

| Shortcut       | Action      |
| -------------- | ----------- |
| `Ctrl/Cmd + S` | Save file   |
| `Ctrl/Cmd + B` | Bold        |
| `Ctrl/Cmd + I` | Italic      |
| `Ctrl/Cmd + K` | Insert link |

## API Endpoints

Hugo Manager exposes a REST API:

| Method | Endpoint              | Description              |
| ------ | --------------------- | ------------------------ |
| GET    | `/api/files`          | List file tree           |
| GET    | `/api/files/{path}`   | Read file                |
| PUT    | `/api/files/{path}`   | Save file                |
| POST   | `/api/files/{path}`   | Create file              |
| DELETE | `/api/files/{path}`   | Delete file              |
| GET    | `/api/shortcodes`     | List detected shortcodes |
| POST   | `/api/images/upload`  | Upload and process image |
| GET    | `/api/images/folders` | List image folders       |
| GET    | `/api/images/presets` | List image presets       |
| GET    | `/api/hugo/status`    | Hugo server status       |
| POST   | `/api/hugo/start`     | Start Hugo               |
| POST   | `/api/hugo/stop`      | Stop Hugo                |
| POST   | `/api/hugo/restart`   | Restart Hugo             |
| WS     | `/api/hugo/ws`        | WebSocket for logs       |

## Requirements

- Go 1.25+ (for building)
- Hugo v0.154+ (installed and in PATH)
- Modern web browser (Chrome, Firefox, Safari, Edge)

## Development

```bash
# Run in development mode
go run ./cmd/hugo-manager --dir /path/to/test/hugo/site

# Build for production
make build-small

# Build for all platforms
make release
```

## License

MIT License - see [LICENSE](LICENSE) for details.
