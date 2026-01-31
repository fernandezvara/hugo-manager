// Hugo Manager - Main Application (ES Module)
import * as monaco from 'monaco-editor';

// Configure Monaco workers
self.MonacoEnvironment = {
  getWorker: function (workerId, label) {
    const getWorkerModule = (moduleUrl, label) => {
      return new Worker(self.MonacoEnvironment.getWorkerUrl(moduleUrl), {
        name: label,
        type: 'module'
      });
    };
    switch (label) {
      case 'json':
        return getWorkerModule('/monaco-editor/esm/vs/language/json/json.worker?worker', label);
      case 'css':
      case 'scss':
      case 'less':
        return getWorkerModule('/monaco-editor/esm/vs/language/css/css.worker?worker', label);
      case 'html':
      case 'handlebars':
      case 'razor':
        return getWorkerModule('/monaco-editor/esm/vs/language/html/html.worker?worker', label);
      case 'typescript':
      case 'javascript':
        return getWorkerModule('/monaco-editor/esm/vs/language/typescript/ts.worker?worker', label);
      default:
        return getWorkerModule('/monaco-editor/esm/vs/editor/editor.worker?worker', label);
    }
  }
};

// Define custom theme
monaco.editor.defineTheme('hugo-dark', {
  base: 'vs-dark',
  inherit: true,
  rules: [],
  colors: {
    'editor.background': '#1a1d23',
    'editor.foreground': '#e4e7eb',
    'editorLineNumber.foreground': '#6b7280',
    'editorLineNumber.activeForeground': '#9ca3af',
    'editor.selectionBackground': '#3b82f640',
    'editor.lineHighlightBackground': '#22262e'
  }
});

export function createApp() {
  return {
    // Configuration
    config: window.APP_CONFIG || {},
    
    // UI State
    sidebarWidth: 280,
    editorWidth: null,
    showLogs: false,
    showImageModal: false,
    showNewFile: false,
    showFileSelector: false,
    shortcodesExpanded: true,
    dragOver: false,
    
    // File Tree
    fileTree: [],
    expandedDirs: new Set(['content', 'layouts/shortcodes']),
    
    // Editor State
    tabs: [],
    activeTab: null,
    editor: null,
    editorModels: {},
    monacoLoaded: true, // Monaco is already loaded via import
    
    // Shortcodes
    shortcodes: [],
    
    // Hugo Status
    hugoStatus: { status: 'stopped', message: '' },
    logs: [],
    ws: null,
    previewReady: false,
    previewUrl: 'about:blank',
    
    // Image Upload
    imageFile: null,
    imagePreview: null,
    imageFolders: [],
    imagePresets: [],
    uploadOptions: {
      folder: '',
      filename: '',
      quality: 85,
      preset: 'Full responsive',
      customWidths: ''
    },
    uploadResult: null,
    uploading: false,
    
    // New File
    newFilePath: '',
    newFileIsDir: false,
    
    // File Selector (for shortcode params)
    fileSelectorSearch: '',
    dataFiles: [],
    fileSelectorCallback: null,
    
    // Toasts
    toasts: [],
    toastId: 0,

    // Initialize
    async init() {
      // Load initial data
      await Promise.all([
        this.refreshFiles(),
        this.loadShortcodes(),
        this.loadHugoStatus(),
        this.loadImageFolders(),
        this.loadImagePresets()
      ]);
      
      // Connect WebSocket for logs
      this.connectWebSocket();
      
      // Periodic status check
      setInterval(() => this.loadHugoStatus(), 5000);
    },

    // File Operations
    async refreshFiles() {
      try {
        const res = await fetch('/api/files');
        this.fileTree = await res.json();
      } catch (err) {
        this.showToast('Failed to load files', 'error');
      }
    },

    async openFile(path, type) {
      // Check if already open
      const existingTab = this.tabs.find(t => t.path === path);
      if (existingTab) {
        this.switchTab(path);
        return;
      }

      try {
        const res = await fetch(`/api/files/${encodeURIComponent(path)}`);
        const data = await res.json();
        
        if (data.error) {
          this.showToast(data.error, 'error');
          return;
        }

        const tab = {
          path,
          name: path.split('/').pop(),
          type: type || this.getFileType(path),
          content: data.content,
          originalContent: data.content,
          modified: false
        };

        this.tabs.push(tab);
        this.switchTab(path);
        this.updatePreviewUrl(path);
      } catch (err) {
        this.showToast('Failed to open file', 'error');
      }
    },

    switchTab(path) {
      this.activeTab = path;
      const tab = this.tabs.find(t => t.path === path);
      if (!tab) return;

      // Create editor if it doesn't exist yet
      if (!this.editor && this.monacoLoaded) {
        this.editor = monaco.editor.create(document.getElementById('monaco-editor'), {
          theme: this.config.editor?.theme || 'hugo-dark',
          fontSize: this.config.editor?.fontSize || 14,
          tabSize: this.config.editor?.tabSize || 2,
          wordWrap: this.config.editor?.wordWrap ? 'on' : 'off',
          minimap: { enabled: this.config.editor?.minimap || false },
          automaticLayout: true,
          scrollBeyondLastLine: false,
          renderWhitespace: 'selection',
          fontFamily: "'SF Mono', 'Fira Code', 'Consolas', monospace",
          fontLigatures: true
        });

        // Track changes
        this.editor.onDidChangeModelContent(() => {
          const tab = this.tabs.find(t => t.path === this.activeTab);
          if (tab && this.editor.getValue() !== tab.originalContent) {
            tab.modified = true;
          }
        });
      }

      // Get or create Monaco model
      let model = this.editorModels[path];
      if (!model) {
        const language = this.getMonacoLanguage(tab.type);
        model = monaco.editor.createModel(tab.content, language);
        this.editorModels[path] = model;
      }

      this.editor.setModel(model);
      this.updatePreviewUrl(path);
    },

    closeTab(path) {
      const tab = this.tabs.find(t => t.path === path);
      if (tab?.modified) {
        if (!confirm('Discard unsaved changes?')) return;
      }

      const index = this.tabs.findIndex(t => t.path === path);
      this.tabs.splice(index, 1);

      // Dispose Monaco model
      if (this.editorModels[path]) {
        this.editorModels[path].dispose();
        delete this.editorModels[path];
      }

      // Switch to another tab
      if (this.activeTab === path) {
        if (this.tabs.length > 0) {
          const newIndex = Math.min(index, this.tabs.length - 1);
          this.switchTab(this.tabs[newIndex].path);
        } else {
          this.activeTab = null;
        }
      }
    },

    async saveCurrentFile() {
      const tab = this.tabs.find(t => t.path === this.activeTab);
      if (!tab) return;

      const content = this.editor.getValue();

      try {
        const res = await fetch(`/api/files/${encodeURIComponent(tab.path)}`, {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ content })
        });

        const data = await res.json();
        if (data.error) {
          this.showToast(data.error, 'error');
          return;
        }

        tab.content = content;
        tab.originalContent = content;
        tab.modified = false;
        
        this.showToast('File saved', 'success');
        this.refreshPreview();
      } catch (err) {
        this.showToast('Failed to save file', 'error');
      }
    },

    get currentTabModified() {
      const tab = this.tabs.find(t => t.path === this.activeTab);
      return tab?.modified || false;
    },

    showNewFileDialog() {
      this.newFilePath = 'content/';
      this.newFileIsDir = false;
      this.showNewFile = true;
    },

    async createNewFile() {
      if (!this.newFilePath) return;

      try {
        const res = await fetch(`/api/files/${encodeURIComponent(this.newFilePath)}`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            content: this.getDefaultContent(this.newFilePath),
            isDir: this.newFileIsDir
          })
        });

        const data = await res.json();
        if (data.error) {
          this.showToast(data.error, 'error');
          return;
        }

        this.showNewFile = false;
        await this.refreshFiles();
        
        if (!this.newFileIsDir) {
          this.openFile(this.newFilePath);
        }
        
        this.showToast('File created', 'success');
      } catch (err) {
        this.showToast('Failed to create file', 'error');
      }
    },

    getDefaultContent(path) {
      if (path.endsWith('.md')) {
        return `---
title: "New Page"
date: ${new Date().toISOString().split('T')[0]}
draft: true
---

Content goes here...
`;
      }
      return '';
    },

    // Shortcodes
    async loadShortcodes() {
      try {
        const res = await fetch('/api/shortcodes');
        this.shortcodes = await res.json();
      } catch (err) {
        console.error('Failed to load shortcodes:', err);
      }
    },

    insertShortcode(sc) {
      if (!this.editor || !this.activeTab) {
        this.showToast('Open a file first', 'info');
        return;
      }

      const selection = this.editor.getSelection();
      this.editor.executeEdits('shortcode', [{
        range: selection,
        text: sc.template,
        forceMoveMarkers: true
      }]);
      this.editor.focus();
    },

    insertShortcodeByName(name) {
      if (!name) return;
      const sc = this.shortcodes.find(s => s.name === name);
      if (sc) this.insertShortcode(sc);
    },

    // Formatting
    formatBold() {
      this.wrapSelection('**', '**');
    },

    formatItalic() {
      this.wrapSelection('*', '*');
    },

    formatStrike() {
      this.wrapSelection('~~', '~~');
    },

    formatHeader(level) {
      const prefix = '#'.repeat(level) + ' ';
      this.prefixLine(prefix);
    },

    formatLink() {
      const selection = this.editor.getSelection();
      const text = this.editor.getModel().getValueInRange(selection) || 'link text';
      this.editor.executeEdits('format', [{
        range: selection,
        text: `[${text}](url)`,
        forceMoveMarkers: true
      }]);
    },

    formatImage() {
      const selection = this.editor.getSelection();
      this.editor.executeEdits('format', [{
        range: selection,
        text: '![alt text](/images/)',
        forceMoveMarkers: true
      }]);
    },

    formatCode() {
      const selection = this.editor.getSelection();
      const text = this.editor.getModel().getValueInRange(selection);
      const isMultiline = text.includes('\n');
      
      if (isMultiline) {
        this.wrapSelection('```\n', '\n```');
      } else {
        this.wrapSelection('`', '`');
      }
    },

    formatList() {
      this.prefixLine('- ');
    },

    formatQuote() {
      this.prefixLine('> ');
    },

    wrapSelection(before, after) {
      if (!this.editor) return;
      const selection = this.editor.getSelection();
      const text = this.editor.getModel().getValueInRange(selection);
      this.editor.executeEdits('format', [{
        range: selection,
        text: before + text + after,
        forceMoveMarkers: true
      }]);
      this.editor.focus();
    },

    prefixLine(prefix) {
      if (!this.editor) return;
      const selection = this.editor.getSelection();
      
      const startLine = selection.startLineNumber;
      const endLine = selection.endLineNumber;
      
      const edits = [];
      for (let line = startLine; line <= endLine; line++) {
        edits.push({
          range: new monaco.Range(line, 1, line, 1),
          text: prefix,
          forceMoveMarkers: true
        });
      }
      
      this.editor.executeEdits('format', edits);
      this.editor.focus();
    },

    // Hugo Control
    async loadHugoStatus() {
      try {
        const res = await fetch('/api/hugo/status');
        this.hugoStatus = await res.json();
        
        if (this.hugoStatus.status === 'running' && !this.previewReady) {
          this.previewReady = true;
          this.previewUrl = `http://localhost:${this.config.hugoPort}/`;
        }
      } catch (err) {
        console.error('Failed to get Hugo status:', err);
      }
    },

    async hugoStart() {
      await fetch('/api/hugo/start', { method: 'POST' });
      setTimeout(() => this.loadHugoStatus(), 1000);
    },

    async hugoStop() {
      await fetch('/api/hugo/stop', { method: 'POST' });
      this.previewReady = false;
      this.previewUrl = 'about:blank';
      this.loadHugoStatus();
    },

    async hugoRestart() {
      await fetch('/api/hugo/restart', { method: 'POST' });
      this.previewReady = false;
      setTimeout(() => this.loadHugoStatus(), 2000);
    },

    getStatusText() {
      switch (this.hugoStatus.status) {
        case 'running': return `Running on :${this.config.hugoPort}`;
        case 'starting': return 'Starting...';
        case 'stopped': return 'Stopped';
        case 'error': return 'Error';
        default: return this.hugoStatus.status;
      }
    },

    // WebSocket for logs
    connectWebSocket() {
      const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
      this.ws = new WebSocket(`${protocol}//${window.location.host}/api/hugo/ws`);
      
      this.ws.onmessage = (event) => {
        const log = JSON.parse(event.data);
        this.logs.push(log);
        if (this.logs.length > 500) {
          this.logs = this.logs.slice(-500);
        }
        
        // Auto-scroll logs
        this.$nextTick(() => {
          const container = document.getElementById('logs-container');
          if (container) {
            container.scrollTop = container.scrollHeight;
          }
        });
      };

      this.ws.onclose = () => {
        setTimeout(() => this.connectWebSocket(), 3000);
      };
    },

    toggleLogs() {
      this.showLogs = !this.showLogs;
    },

    clearLogs() {
      this.logs = [];
    },

    formatTime(time) {
      const date = new Date(time);
      return date.toLocaleTimeString();
    },

    // Preview
    updatePreviewUrl(path) {
      if (!this.previewReady) return;
      
      // Convert file path to URL
      let url = path
        .replace(/^content\//, '/')
        .replace(/\.md$/, '/')
        .replace(/_index\/$/, '');
      
      if (!url.startsWith('/')) url = '/' + url;
      if (!url.endsWith('/')) url += '/';
      
      this.previewUrl = `http://localhost:${this.config.hugoPort}${url}`;
    },

    refreshPreview() {
      const iframe = document.getElementById('preview-frame');
      if (iframe) {
        iframe.src = iframe.src;
      }
    },

    previewLoaded() {
      // Preview loaded
    },

    openPreviewExternal() {
      window.open(this.previewUrl, '_blank');
    },

    // Image Upload
    async loadImageFolders() {
      try {
        const res = await fetch('/api/images/folders');
        this.imageFolders = await res.json();
        if (this.imageFolders.length > 0) {
          this.uploadOptions.folder = this.imageFolders[0].name;
        }
      } catch (err) {
        console.error('Failed to load image folders:', err);
      }
    },

    async loadImagePresets() {
      try {
        const res = await fetch('/api/images/presets');
        this.imagePresets = await res.json();
      } catch (err) {
        console.error('Failed to load image presets:', err);
      }
    },

    openImageUpload() {
      this.imageFile = null;
      this.imagePreview = null;
      this.uploadResult = null;
      this.showImageModal = true;
    },

    handleImageSelect(event) {
      const file = event.target.files[0];
      if (file) this.setImageFile(file);
    },

    handleDrop(event) {
      this.dragOver = false;
      const file = event.dataTransfer.files[0];
      if (file && file.type.startsWith('image/')) {
        this.setImageFile(file);
      }
    },

    setImageFile(file) {
      this.imageFile = file;
      this.uploadOptions.filename = file.name;
      
      // Create preview
      const reader = new FileReader();
      reader.onload = (e) => {
        this.imagePreview = e.target.result;
      };
      reader.readAsDataURL(file);
    },

    applyPreset() {
      // Preset is applied on server based on name
    },

    async uploadImage() {
      if (!this.imageFile) return;

      this.uploading = true;
      this.uploadResult = null;

      const formData = new FormData();
      formData.append('image', this.imageFile);
      formData.append('folder', this.uploadOptions.folder);
      formData.append('quality', this.uploadOptions.quality);
      formData.append('filename', this.uploadOptions.filename);

      // Get widths from preset
      const preset = this.imagePresets.find(p => p.name === this.uploadOptions.preset);
      let widths = preset?.widths || [];
      
      if (this.uploadOptions.preset === 'Custom' && this.uploadOptions.customWidths) {
        widths = this.uploadOptions.customWidths.split(',').map(w => parseInt(w.trim())).filter(w => w > 0);
      }
      
      if (widths.length > 0) {
        formData.append('widths', JSON.stringify(widths));
      }

      try {
        const res = await fetch('/api/images/upload', {
          method: 'POST',
          body: formData
        });

        const data = await res.json();
        if (data.error) {
          this.showToast(data.error, 'error');
        } else {
          this.uploadResult = data;
          this.showToast('Image processed successfully', 'success');
        }
      } catch (err) {
        this.showToast('Failed to upload image', 'error');
      } finally {
        this.uploading = false;
      }
    },

    // File Selector (for shortcode file params)
    async openFileSelector(dataType, callback) {
      this.fileSelectorSearch = '';
      this.fileSelectorCallback = callback;
      
      try {
        const res = await fetch(`/api/data/${dataType}`);
        this.dataFiles = await res.json();
        this.showFileSelector = true;
      } catch (err) {
        this.showToast('Failed to load files', 'error');
      }
    },

    get filteredDataFiles() {
      const search = this.fileSelectorSearch.toLowerCase();
      if (!search) return this.dataFiles;
      return this.dataFiles.filter(f => 
        f.name.toLowerCase().includes(search) || 
        f.path.toLowerCase().includes(search)
      );
    },

    selectDataFile(file) {
      if (this.fileSelectorCallback) {
        this.fileSelectorCallback(file.path);
      }
      this.showFileSelector = false;
    },

    // Keyboard shortcuts
    handleKeydown(event) {
      // Ctrl/Cmd + S = Save
      if ((event.ctrlKey || event.metaKey) && event.key === 's') {
        event.preventDefault();
        this.saveCurrentFile();
      }
      // Ctrl/Cmd + B = Bold
      if ((event.ctrlKey || event.metaKey) && event.key === 'b') {
        event.preventDefault();
        this.formatBold();
      }
      // Ctrl/Cmd + I = Italic
      if ((event.ctrlKey || event.metaKey) && event.key === 'i') {
        event.preventDefault();
        this.formatItalic();
      }
      // Ctrl/Cmd + K = Link
      if ((event.ctrlKey || event.metaKey) && event.key === 'k') {
        event.preventDefault();
        this.formatLink();
      }
    },

    // Resize handling
    startResize(event, type) {
      const startX = event.clientX;
      const startWidth = type === 'sidebar' ? this.sidebarWidth : 
                         document.querySelector('.editor-panel').offsetWidth;

      const onMouseMove = (e) => {
        const delta = e.clientX - startX;
        if (type === 'sidebar') {
          this.sidebarWidth = Math.max(200, Math.min(400, startWidth + delta));
        } else {
          this.editorWidth = Math.max(300, startWidth + delta);
        }
      };

      const onMouseUp = () => {
        document.removeEventListener('mousemove', onMouseMove);
        document.removeEventListener('mouseup', onMouseUp);
      };

      document.addEventListener('mousemove', onMouseMove);
      document.addEventListener('mouseup', onMouseUp);
    },

    // Tree rendering
    renderTreeItem(item, depth) {
      const indent = depth * 16;
      const isExpanded = this.expandedDirs.has(item.path);
      const isActive = this.activeTab === item.path;
      
      let html = `<div class="tree-item" style="padding-left: ${indent}px">`;
      
      if (item.isDir) {
        html += `
          <div class="tree-item-content" @click="toggleDir('${item.path}')">
            <span class="tree-toggle ${isExpanded ? 'expanded' : ''}">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" width="12" height="12">
                <polyline points="9 18 15 12 9 6"/>
              </svg>
            </span>
            <span class="tree-icon folder">
              <svg viewBox="0 0 24 24" fill="currentColor" width="16" height="16">
                <path d="M10 4H4c-1.1 0-2 .9-2 2v12c0 1.1.9 2 2 2h16c1.1 0 2-.9 2-2V8c0-1.1-.9-2-2-2h-8l-2-2z"/>
              </svg>
            </span>
            <span class="tree-name">${item.name}</span>
          </div>
        `;
        
        if (item.children && item.children.length > 0) {
          html += `<div class="tree-children ${isExpanded ? 'expanded' : ''}">`;
          for (const child of item.children) {
            html += this.renderTreeItem(child, depth + 1);
          }
          html += '</div>';
        }
      } else {
        const iconClass = `file-${item.type || 'text'}`;
        html += `
          <div class="tree-item-content ${isActive ? 'active' : ''}" @click="openFile('${item.path}', '${item.type}')">
            <span class="tree-toggle" style="visibility: hidden">
              <svg viewBox="0 0 24 24" width="12" height="12"></svg>
            </span>
            <span class="tree-icon ${iconClass}">
              ${this.getFileIconSvg(item.type)}
            </span>
            <span class="tree-name">${item.name}</span>
          </div>
        `;
      }
      
      html += '</div>';
      return html;
    },

    toggleDir(path) {
      if (this.expandedDirs.has(path)) {
        this.expandedDirs.delete(path);
      } else {
        this.expandedDirs.add(path);
      }
      // Force reactivity
      this.expandedDirs = new Set(this.expandedDirs);
    },

    getFileIconSvg(type) {
      const icons = {
        markdown: '<svg viewBox="0 0 24 24" fill="currentColor" width="16" height="16"><path d="M14 2H6a2 2 0 00-2 2v16a2 2 0 002 2h12a2 2 0 002-2V8l-6-6zm-1 2l5 5h-5V4zM9.5 11.5l1.5 2 1.5-2H15l-2.25 3L15 17.5h-2.5l-1.5-2-1.5 2H7l2.25-3L7 11.5h2.5z"/></svg>',
        html: '<svg viewBox="0 0 24 24" fill="currentColor" width="16" height="16"><path d="M12 17.56l4.07-1.13.55-6.1H9.38L9.2 8.3h7.6l.2-2.03H7l.56 6.01h6.89l-.23 2.58-2.22.6-2.22-.6-.14-1.66h-2l.29 3.19L12 17.56M3 2h18l-1.64 18L12 22l-7.36-2L3 2z"/></svg>',
        yaml: '<svg viewBox="0 0 24 24" fill="currentColor" width="16" height="16"><path d="M14 2H6a2 2 0 00-2 2v16a2 2 0 002 2h12a2 2 0 002-2V8l-6-6zM6 20V4h7v5h5v11H6z"/></svg>',
        image: '<svg viewBox="0 0 24 24" fill="currentColor" width="16" height="16"><path d="M21 19V5c0-1.1-.9-2-2-2H5c-1.1 0-2 .9-2 2v14c0 1.1.9 2 2 2h14c1.1 0 2-.9 2-2zM8.5 13.5l2.5 3.01L14.5 12l4.5 6H5l3.5-4.5z"/></svg>',
        default: '<svg viewBox="0 0 24 24" fill="currentColor" width="16" height="16"><path d="M14 2H6a2 2 0 00-2 2v16a2 2 0 002 2h12a2 2 0 002-2V8l-6-6zm4 18H6V4h7v5h5v11z"/></svg>'
      };
      return icons[type] || icons.default;
    },

    getFileIcon(type) {
      return this.getFileIconSvg(type);
    },

    getFileType(path) {
      const ext = path.split('.').pop().toLowerCase();
      const types = {
        md: 'markdown',
        html: 'html',
        yaml: 'yaml',
        yml: 'yaml',
        toml: 'toml',
        json: 'json',
        js: 'javascript',
        css: 'css',
        jpg: 'image',
        jpeg: 'image',
        png: 'image',
        gif: 'image',
        svg: 'image'
      };
      return types[ext] || 'text';
    },

    getMonacoLanguage(type) {
      const languages = {
        markdown: 'markdown',
        html: 'html',
        yaml: 'yaml',
        toml: 'ini',
        json: 'json',
        javascript: 'javascript',
        css: 'css'
      };
      return languages[type] || 'plaintext';
    },

    // Toasts
    showToast(message, type = 'info') {
      const id = ++this.toastId;
      const toast = { id, message, type, visible: true };
      this.toasts.push(toast);
      
      setTimeout(() => {
        toast.visible = false;
        setTimeout(() => {
          this.toasts = this.toasts.filter(t => t.id !== id);
        }, 300);
      }, 3000);
    },

    // Utilities
    formatBytes(bytes) {
      if (bytes === 0) return '0 B';
      const k = 1024;
      const sizes = ['B', 'KB', 'MB', 'GB'];
      const i = Math.floor(Math.log(bytes) / Math.log(k));
      return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
    },

    async copyToClipboard(text) {
      try {
        await navigator.clipboard.writeText(text);
        this.showToast('Copied to clipboard', 'success');
      } catch (err) {
        this.showToast('Failed to copy', 'error');
      }
    }
  };
}
