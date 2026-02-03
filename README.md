# Hugo Manager

A web-based management interface for Hugo static sites. Edit content, manage images, and preview changes in real-time.

![Hugo Manager Interface](docs/screenshot.png)

## Features

- **CodeMirror 6** - Lightweight, modern code editor with syntax highlighting for Markdown, YAML, HTML, and more
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
    - '--bind'
    - '0.0.0.0'

# Editor settings
editor:
  theme: one-dark # one-dark, light
  font_size: 14
  tab_size: 2
  word_wrap: true
  line_numbers: true

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

# Metadata templates (frontmatter schemas)
templates:
  blog_post:
    title:
      type: text
      default: 'New Blog Post'
    summary:
      type: textarea
      default: ''
    date:
      type: date
    author:
      type: text
      default: ''
    image:
      type: image
    categories:
      type: array
      default: []
    draft:
      type: bool
      default: true
    tags:
      type: array
    featured_image:
      type: image
    weight:
      type: number
    layout:
      type: text
      default: 'single'

  persona:
    prefix:
      type: text
      default: ''
    name:
      type: text
      default: '<set the name>'
    suffix:
      type: text
      default: ''
    position:
      type: text
      default: ''
    institution:
      type: text
      default: ''
    bio:
      type: textarea
      default: '<set the bio>'
    email:
      type: text
      default: ''
    phone:
      type: text
      default: ''
    linkedin:
      type: text
      default: ''
    photo:
      type: image

  content_page:
    title:
      type: text
      default: 'Page Title'
    weight:
      type: number
    layout:
      type: text
      default: 'single'
```

## Metadata Templates

Hugo Manager supports configurable metadata templates to standardize and simplify frontmatter editing. Templates define the structure, types, and defaults for your content’s frontmatter.

### How It Works

1. **Template Detection** – When you open the Metadata modal, Hugo Manager automatically detects the appropriate template by matching existing frontmatter keys to your template definitions.
2. **Dynamic Forms** – The modal renders a form based on the selected template’s schema, showing appropriate inputs for each field type.
3. **Preservation** – Unknown frontmatter keys are preserved unchanged; only keys defined in the template are updated.
4. **No Auto-Save** – Changes are applied to the editor buffer and the tab is marked modified; you save manually.

### Field Types

| Type       | UI Component                       | Stored As                | Notes                      |
| ---------- | ---------------------------------- | ------------------------ | -------------------------- | --- |
| `text`     | Text input                         | String                   | Single-line text           |
| `textarea` | Textarea                           | Multiline string         | Supports multiline YAML (` | `)  |
| `number`   | Number input                       | Number                   | Integer values             |
| `date`     | Date input                         | String (YYYY-MM-DD)      | HTML5 date picker          |
| `bool`     | Checkbox                           | Boolean (`true`/`false`) | Toggles true/false         |
| `image`    | Text input + “Select” button       | String (relative path)   | Opens image browser modal  |
| `array`    | List of text inputs (+ Add/Remove) | YAML list                | Dynamic list of strings    |

### Template Configuration

Each top-level key under `templates` defines a template name. Inside a template, each key represents a frontmatter field with the following options:

- `type` (required): One of the field types above.
- `default` (optional): Default value when creating new content.
- `label` (optional): Human-readable label (defaults to the key name).

#### Example: Blog Post Template

```yaml
templates:
  blog_post:
    title:
      type: text
      default: 'New Blog Post'
    summary:
      type: textarea
      default: ''
    date:
      type: date
    author:
      type: text
      default: ''
    image:
      type: image
    categories:
      type: array
      default: []
    draft:
      type: bool
      default: true
    tags:
      type: array
    featured_image:
      type: image
    weight:
      type: number
    layout:
      type: text
      default: 'single'
```

### Using the Metadata Modal

1. Open a content file in the editor.
2. Click the **Metadata** button in the editor toolbar (or press `Ctrl/Cmd+M`).
3. If a template is auto-detected, its fields appear; otherwise, select one manually.
4. Edit fields. For images, click **Select** to browse existing images.
5. Click **Save** to update the frontmatter in the editor buffer.
6. Save the file normally to persist changes.

### Template Detection Logic

Hugo Manager uses a simple key-matching strategy:

- It scans the current frontmatter keys.
- For each template, it checks if any frontmatter key matches a template key (case-insensitive).
- If exactly one template matches, it’s selected automatically.
- If multiple or no templates match, you can choose manually.

### YAML Output Examples

#### Basic Types

```yaml
title: My Post
draft: true
weight: 42
date: 2026-02-03
```

#### Arrays

```yaml
categories:
  - tech
  - hugo
tags:
  - metadata
  - templates
```

#### Multiline Textarea

```yaml
summary: |
  This is a multiline summary.
  It preserves line breaks.
```

#### Images

```yaml
image: /images/blog/my-post.jpg
featured_image: /images/blog/my-post-featured.jpg
```

### Tips

- Use `default: []` for arrays to ensure an empty list is created on new content.
- Use `default: ''` for optional text fields to avoid `null` values.
- Image fields store paths relative to your `images.base_dir` (usually `static/images`).
- Required fields are indicated with a `*` in the modal (fields without a `default`).
- The modal preserves any unknown frontmatter keys (e.g., custom Hugo params).

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
{{/* These patterns are detected: */}} {{ $show := .Get "show" | default true }} → boolean, optional, default: true {{
with .Get "file" }} → required parameter {{ $title := .Get "title" }} → string parameter {{ .Inner }} → shortcode
accepts inner content
```

## Keyboard Shortcuts

| Shortcut       | Action              |
| -------------- | ------------------- |
| `Ctrl/Cmd + S` | Save file           |
| `Ctrl/Cmd + M` | Open Metadata modal |
| `Ctrl/Cmd + B` | Bold                |
| `Ctrl/Cmd + I` | Italic              |
| `Ctrl/Cmd + K` | Insert link         |
| `Ctrl/Cmd + U` | Underline           |
| `Escape`       | Close topmost modal |

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
