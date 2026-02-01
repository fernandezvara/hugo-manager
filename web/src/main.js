// Hugo Manager - Main Application (ES Module)
import { EditorView } from "@codemirror/view";
import { EditorState } from "@codemirror/state";
import { markdown } from "@codemirror/lang-markdown";
import { oneDark } from "@codemirror/theme-one-dark";
import { lineNumbers } from "@codemirror/view";
import { keymap } from "@codemirror/view";
import { defaultKeymap } from "@codemirror/commands";

// Utility functions
function slugify(text) {
  return text
    .toString()
    .normalize("NFD") // Normalize to decomposed form
    .replace(/[\u0300-\u036f]/g, "") // Remove diacritics (á → a, ñ → n)
    .toLowerCase()
    .trim()
    .replace(/\s+/g, "-") // Replace spaces with -
    .replace(/[^\w\-]+/g, "") // Remove all non-word chars except -
    .replace(/\-\-+/g, "-") // Replace multiple - with single -
    .replace(/^-+/, "") // Trim - from start
    .replace(/-+$/, ""); // Trim - from end
}

export function createApp() {
  return {
    // Configuration
    config: window.APP_CONFIG || {},

    // Initialize method
    init() {
      this.loadConfig();
      this.loadShortcodes();
      this.loadImagePresets();
      this.loadImageFolders();
      this.loadFiles();
      this.startStatusPolling();
    },

    // UI State
    sidebarWidth: 280,
    editorWidth: null,
    showLogs: false,
    showImageModal: false,
    showNewFile: false,
    showFileSelector: false,
    dragOver: false,

    // File Tree
    fileTree: [],
    expandedDirs: new Set(["content", "layouts/shortcodes"]),

    // Editor State
    tabs: [],
    activeTab: null,
    editor: null,
    editorModels: {},
    monacoLoaded: true, // Monaco is already loaded via import

    // Shortcodes
    shortcodes: [],

    // Templates
    showTemplateModal: false,
    selectedTemplate: null,
    templateForm: {},
    templateFilename: "",
    templateDirectory: "",

    // Image Upload
    uploadFromContext: false,

    // Context Menu
    contextMenu: {
      visible: false,
      x: 0,
      y: 0,
      target: null,
      targetPath: "",
      isDirectory: false,
    },

    // Directory Selection
    showDirectoryModal: false,
    directoryCallback: null,
    selectedDirectory: "",
    newDirectoryName: "",

    // Rename Modal
    showRenameModal: false,
    renameCallback: null,
    renameItemName: "",
    renameNewName: "",

    // Confirmation Modal
    showConfirmModal: false,
    confirmCallback: null,
    confirmMessage: "",
    confirmTitle: "",

    // Hugo Status
    hugoStatus: { status: "stopped", message: "" },
    logs: [],
    ws: null,
    previewReady: false,
    previewUrl: "about:blank",
    statusInterval: null,

    // Image Upload
    imageFile: null,
    imagePreview: null,
    imageFolders: [],
    imagePresets: [],
    uploadOptions: {
      folder: "",
      filename: "",
      quality: 85,
      preset: "Full responsive",
      customWidths: "",
    },
    uploadResult: null,
    uploading: false,

    // New File
    newFilePath: "",
    newFileIsDir: false,

    // File Selector (for shortcode params)
    fileSelectorSearch: "",
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
        this.loadImagePresets(),
      ]);

      // Connect WebSocket for logs
      this.connectWebSocket();

      // Periodic status check
      if (this.statusInterval) {
        clearInterval(this.statusInterval);
      }
      this.statusInterval = setInterval(() => this.loadHugoStatus(), 5000);
    },

    // File Operations
    async refreshFiles() {
      try {
        const res = await fetch("/api/files");
        this.fileTree = await res.json();
      } catch (err) {
        this.showToast("Failed to load files", "error");
      }
    },

    async openFile(path, type) {
      // Check if already open
      const existingTab = this.tabs.find((t) => t.path === path);
      if (existingTab) {
        this.switchTab(path);
        return;
      }

      try {
        const res = await fetch(`/api/files/${encodeURIComponent(path)}`);
        const data = await res.json();

        if (data.error) {
          this.showToast(data.error, "error");
          return;
        }

        const tab = {
          path,
          name: path.split("/").pop(),
          type: type || this.getFileType(path),
          content: data.content,
          originalContent: data.content,
          modified: false,
        };

        this.tabs.push(tab);
        this.switchTab(path);
        this.updatePreviewUrl(path);
      } catch (err) {
        this.showToast("Failed to open file", "error");
      }
    },

    switchTab(path) {
      console.log("[DEBUG] switchTab called with path:", path);

      // Save current editor content to the current tab before switching
      if (this.editor && this.activeTab) {
        const currentTab = this.tabs.find((t) => t.path === this.activeTab);
        if (currentTab) {
          currentTab.content = this.editor.state.doc.toString();
        }
      }

      this.activeTab = path;
      const tab = this.tabs.find((t) => t.path === path);
      if (!tab) {
        console.log("[DEBUG] No tab found, returning");
        return;
      }

      console.log("[DEBUG] Tab found, creating CodeMirror editor...");

      // Wait for Alpine to update the DOM (show the container) before creating editor
      this.$nextTick(() => {
        console.log("[DEBUG] In nextTick callback");
        // Create editor if it doesn't exist yet
        if (!this.editor) {
          console.log("[DEBUG] Creating CodeMirror editor...");
          const container = document.getElementById("monaco-editor");
          console.log(
            "[DEBUG] Container element:",
            container,
            "display:",
            container?.style?.display,
          );

          const startState = EditorState.create({
            doc: tab.content,
            extensions: [
              lineNumbers(),
              markdown(),
              oneDark,
              keymap.of(defaultKeymap),
              EditorView.theme({
                "&": {
                  fontSize: this.config.editor?.fontSize || "14px",
                  fontFamily: "'SF Mono', 'Fira Code', 'Consolas', monospace",
                  height: "100%",
                },
                ".cm-content": {
                  padding: "12px",
                },
                ".cm-scroller": {
                  overflow: "auto",
                },
                ".cm-focused": {
                  outline: "none",
                },
              }),
              EditorView.updateListener.of((update) => {
                if (update.docChanged) {
                  const tab = this.tabs.find((t) => t.path === this.activeTab);
                  if (tab) {
                    tab.content = update.state.doc.toString();
                    tab.modified =
                      update.state.doc.toString() !== tab.originalContent;
                  }
                }
              }),
            ],
          });

          this.editor = new EditorView({
            state: startState,
            parent: container,
          });
          console.log("[DEBUG] CodeMirror editor created:", this.editor);
        } else {
          // Update existing editor content
          console.log("[DEBUG] Updating CodeMirror content...");
          this.editor.dispatch({
            changes: {
              from: 0,
              to: this.editor.state.doc.length,
              insert: tab.content,
            },
          });
        }

        console.log("[DEBUG] Updating preview URL...");
        this.updatePreviewUrl(path);
        console.log("[DEBUG] switchTab complete");
      });
    },

    closeTab(path) {
      const tab = this.tabs.find((t) => t.path === path);
      if (tab?.modified) {
        if (!confirm("Discard unsaved changes?")) return;
      }

      const index = this.tabs.findIndex((t) => t.path === path);
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
      const tab = this.tabs.find((t) => t.path === this.activeTab);
      if (!tab) return;

      const content = this.editor.state.doc.toString();

      try {
        const res = await fetch(`/api/files/${encodeURIComponent(tab.path)}`, {
          method: "PUT",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ content }),
        });

        const data = await res.json();
        if (data.error) {
          this.showToast(data.error, "error");
          return;
        }

        tab.content = content;
        tab.originalContent = content;
        tab.modified = false;

        this.showToast("File saved", "success");
        this.refreshPreview();
      } catch (err) {
        this.showToast("Failed to save file", "error");
      }
    },

    get currentTabModified() {
      const tab = this.tabs.find((t) => t.path === this.activeTab);
      return tab?.modified || false;
    },

    showNewFileDialog() {
      this.newFilePath = "content/";
      this.newFileIsDir = false;
      this.showNewFile = true;
    },

    async createNewFile() {
      if (!this.newFilePath) return;

      try {
        const res = await fetch(
          `/api/files/${encodeURIComponent(this.newFilePath)}`,
          {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({
              content: this.getDefaultContent(this.newFilePath),
              isDir: this.newFileIsDir,
            }),
          },
        );

        const data = await res.json();
        if (data.error) {
          this.showToast(data.error, "error");
          return;
        }

        this.showNewFile = false;
        await this.refreshFiles();

        if (!this.newFileIsDir) {
          this.openFile(this.newFilePath);
        }

        this.showToast("File created", "success");
      } catch (err) {
        this.showToast("Failed to create file", "error");
      }
    },

    getDefaultContent(path) {
      if (path.endsWith(".md")) {
        return `---
title: "New Page"
date: ${new Date().toISOString().split("T")[0]}
draft: true
---

Content goes here...
`;
      }
      return "";
    },

    // Shortcodes
    async loadShortcodes() {
      try {
        const res = await fetch("/api/shortcodes");
        this.shortcodes = await res.json();
      } catch (err) {
        console.error("Failed to load shortcodes:", err);
      }
    },

    insertShortcode(sc) {
      if (!this.editor || !this.activeTab) {
        this.showToast("Open a file first", "info");
        return;
      }

      const view = this.editor;
      view.dispatch(
        view.state.changeByRange((range) => {
          return {
            changes: { from: range.from, to: range.to, insert: sc.template },
            range: {
              from: range.from + sc.template.length,
              to: range.from + sc.template.length,
            },
          };
        }),
      );
      view.focus();
    },

    insertShortcodeByName(name) {
      if (!name) return;
      const sc = this.shortcodes.find((s) => s.name === name);
      if (sc) this.insertShortcode(sc);
    },

    // Formatting
    formatBold() {
      this.wrapSelection("**", "**");
    },

    formatItalic() {
      this.wrapSelection("*", "*");
    },

    formatStrike() {
      this.wrapSelection("~~", "~~");
    },

    formatUnderline() {
      this.wrapSelection("<u>", "</u>");
    },

    formatHeader(level) {
      const prefix = "#".repeat(level) + " ";
      this.prefixLine(prefix);
    },

    formatLink() {
      if (!this.editor) return;

      const view = this.editor;
      view.dispatch(
        view.state.changeByRange((range) => {
          const text = view.state.sliceDoc(range.from, range.to) || "link text";
          const insert = `[${text}](url)`;
          return {
            changes: { from: range.from, to: range.to, insert },
            range: {
              from: range.from + text.length + 3,
              to: range.from + text.length + 6,
            },
          };
        }),
      );
    },

    formatImage() {
      if (!this.editor) return;

      const view = this.editor;
      view.dispatch(
        view.state.changeByRange((range) => {
          const insert = "![alt text](/images/)";
          return {
            changes: { from: range.from, to: range.to, insert },
            range: { from: range.from + 11, to: range.from + 20 },
          };
        }),
      );
    },

    formatCode() {
      if (!this.editor) return;

      const text = this.editor.state.sliceDoc(
        this.editor.state.selection.main.from,
        this.editor.state.selection.main.to,
      );
      const isMultiline = text.includes("\n");

      if (isMultiline) {
        this.wrapSelection("```\n", "\n```");
      } else {
        this.wrapSelection("`", "`");
      }
    },

    formatList() {
      this.prefixLine("- ");
    },

    formatQuote() {
      this.prefixLine("> ");
    },

    wrapSelection(before, after) {
      if (!this.editor) return;

      const view = this.editor;
      view.dispatch(
        view.state.changeByRange((range) => {
          const text = view.state.sliceDoc(range.from, range.to);
          if (text) {
            // Wrap selected text
            return {
              changes: {
                from: range.from,
                to: range.to,
                insert: before + text + after,
              },
              range: {
                from: range.from,
                to: range.from + before.length + text.length + after.length,
              },
            };
          } else {
            // Insert at cursor and place cursor between wrappers
            return {
              changes: { from: range.from, insert: before + after },
              range: {
                from: range.from + before.length,
                to: range.from + before.length,
              },
            };
          }
        }),
      );
    },

    prefixLine(prefix) {
      if (!this.editor) return;

      const state = this.editor.state;
      const doc = state.doc;

      // Handle empty document
      if (doc.length === 0) {
        this.editor.dispatch({
          changes: { from: 0, to: 0, insert: prefix },
        });
        this.editor.focus();
        return;
      }

      // Get line boundaries for selection
      const fromLine = doc.lineAt(state.selection.main.from);
      const toLine = doc.lineAt(state.selection.main.to);

      const changes = [];
      for (let lineNum = fromLine.number; lineNum <= toLine.number; lineNum++) {
        const line = doc.line(lineNum);
        changes.push({
          from: line.from,
          to: line.from,
          insert: prefix,
        });
      }

      this.editor.dispatch({ changes });
      this.editor.focus();
    },

    // Template Operations
    openTemplateModal() {
      this.showTemplateModal = true;
      this.selectedTemplate = null;
      this.templateForm = {};
      this.templateFilename = "";
    },

    closeTemplateModal() {
      this.showTemplateModal = false;
      this.selectedTemplate = null;
      this.templateForm = {};
      this.templateFilename = "";
    },

    selectTemplate(templateName) {
      this.selectedTemplate = templateName;
      this.templateForm = {};
      // Make the app available to Alpine.js
      window.app = this;
      window.init = function () {
        // Initialize app - this will be called by Alpine.js
      };
      // Initialize form with defaults
      const template = this.config.templates[templateName];
      if (template) {
        for (const [fieldName, field] of Object.entries(template)) {
          if (field.type === "date" && !field.default) {
            // Default to today's date if no default specified
            this.templateForm[fieldName] = new Date()
              .toISOString()
              .split("T")[0];
          } else {
            this.templateForm[fieldName] = field.default || "";
          }
        }
      }
    },

    updateFilename() {
      if (this.templateForm.title) {
        this.templateFilename = slugify(this.templateForm.title) + ".md";
      }
    },

    async createFromTemplate() {
      if (!this.selectedTemplate || !this.templateFilename) {
        this.showToast(
          "Please select a template and enter a filename",
          "error",
        );
        return;
      }

      // Build full path with directory
      const fullPath = this.templateDirectory
        ? `${this.templateDirectory}/${this.templateFilename}`
        : this.templateFilename;

      try {
        const response = await fetch(
          `/api/files/${encodeURIComponent(fullPath)}`,
          {
            method: "POST",
            headers: {
              "Content-Type": "application/json",
            },
            body: JSON.stringify({
              template: this.selectedTemplate,
              data: this.templateForm,
            }),
          },
        );

        if (!response.ok) {
          const error = await response.text();
          if (response.status === 409) {
            this.showToast(
              "File already exists. Please choose a different filename.",
              "error",
            );
            return;
          }
          throw new Error(error);
        }

        this.showToast("File created successfully", "success");
        this.closeTemplateModal();
        await this.refreshFiles();

        // Open the created file
        await this.openFile(fullPath);
      } catch (error) {
        console.error("Error creating file from template:", error);
        this.showToast("Failed to create file: " + error.message, "error");
      }
    },

    // Context Menu Operations
    showContextMenu(event, item, isDirectory = false) {
      event.preventDefault();
      this.contextMenu.visible = true;
      this.contextMenu.x = event.clientX;
      this.contextMenu.y = event.clientY;
      this.contextMenu.target = item;
      this.contextMenu.targetPath = item.path;
      this.contextMenu.isDirectory = isDirectory;

      // Hide context menu when clicking elsewhere
      document.addEventListener("click", this.hideContextMenu);
    },

    hideContextMenu() {
      this.contextMenu.visible = false;
      document.removeEventListener("click", this.hideContextMenu);
    },

    contextMenuCreateFile() {
      this.hideContextMenu();
      this.templateDirectory = this.contextMenu.targetPath;
      this.openTemplateModal();
    },

    contextMenuCreateDirectory() {
      this.hideContextMenu();
      this.directoryCallback = (dirName) => this.createDirectory(dirName);
      this.selectedDirectory = this.contextMenu.targetPath;
      this.showDirectoryModal = true;
    },

    contextMenuRename() {
      this.hideContextMenu();
      this.renameItemName = this.contextMenu.target.name;
      this.renameNewName = this.contextMenu.target.name;
      this.renameCallback = (newName) =>
        this.renameItem(this.contextMenu.targetPath, newName);
      this.showRenameModal = true;
    },

    contextMenuDelete() {
      this.hideContextMenu();
      const itemType = this.contextMenu.isDirectory ? "directory" : "file";
      this.showConfirmation(
        `Delete ${itemType}`,
        `Are you sure you want to delete "${this.contextMenu.target.name}"?`,
        () => this.deleteItem(this.contextMenu.targetPath),
      );
    },

    contextMenuUploadImage() {
      this.hideContextMenu();
      // Use context directly - no API call needed
      this.uploadOptions.folder = this.contextMenu.targetPath;
      this.uploadFromContext = true;
      this.showImageModal = true;
    },

    // Directory Selection
    openDirectoryModal(callback, currentDir = "") {
      this.directoryCallback = callback;
      this.selectedDirectory = currentDir;
      this.newDirectoryName = "";
      this.showDirectoryModal = true;
    },

    closeDirectoryModal() {
      this.showDirectoryModal = false;
      this.directoryCallback = null;
      this.selectedDirectory = "";
      this.newDirectoryName = "";
    },

    selectDirectory(dir) {
      this.selectedDirectory = dir;
    },

    confirmDirectorySelection() {
      if (this.newDirectoryName.trim()) {
        const fullPath = this.selectedDirectory
          ? `${this.selectedDirectory}/${this.newDirectoryName.trim()}`
          : this.newDirectoryName.trim();
        if (this.directoryCallback) {
          this.directoryCallback(fullPath);
        }
        this.closeDirectoryModal();
      }
    },

    async createDirectory(dirPath) {
      try {
        const response = await fetch(
          `/api/files/${encodeURIComponent(dirPath)}`,
          {
            method: "POST",
            headers: {
              "Content-Type": "application/json",
            },
            body: JSON.stringify({
              isDir: true,
            }),
          },
        );

        if (!response.ok) {
          const errorText = await response.text();
          if (response.status === 409) {
            this.showToast("Directory already exists", "error");
          } else if (response.status === 400) {
            this.showToast("Invalid directory path", "error");
          } else {
            this.showToast("Failed to create directory: " + errorText, "error");
          }
          return;
        }

        this.showToast("Directory created successfully", "success");
        await this.refreshFiles();
      } catch (error) {
        console.error("Error creating directory:", error);
        this.showToast("Failed to create directory", "error");
      }
    },

    // Rename Modal
    openRenameModal(itemName, currentName, callback) {
      this.renameItemName = itemName;
      this.renameNewName = currentName;
      this.renameCallback = callback;
      this.showRenameModal = true;
    },

    closeRenameModal() {
      this.showRenameModal = false;
      this.renameCallback = null;
      this.renameItemName = "";
      this.renameNewName = "";
    },

    confirmRename() {
      if (
        this.renameNewName.trim() &&
        this.renameNewName !== this.renameItemName
      ) {
        if (this.renameCallback) {
          this.renameCallback(this.renameNewName.trim());
        }
        this.closeRenameModal();
      }
    },

    // Confirmation Modal
    showConfirmation(title, message, callback) {
      this.confirmTitle = title;
      this.confirmMessage = message;
      this.confirmCallback = callback;
      this.showConfirmModal = true;
    },

    closeConfirmModal() {
      this.showConfirmModal = false;
      this.confirmCallback = null;
      this.confirmMessage = "";
      this.confirmTitle = "";
    },

    confirmAction() {
      if (this.confirmCallback) {
        this.confirmCallback();
      }
      this.closeConfirmModal();
    },

    // File Operations
    async renameItem(oldPath, newName) {
      try {
        const response = await fetch(
          `/api/files/${encodeURIComponent(oldPath)}`,
          {
            method: "PUT",
            headers: {
              "Content-Type": "application/json",
            },
            body: JSON.stringify({ newName }),
          },
        );

        if (!response.ok) {
          const errorText = await response.text();
          if (response.status === 409) {
            this.showToast("Destination already exists", "error");
          } else if (response.status === 404) {
            this.showToast("Source does not exist", "error");
          } else if (response.status === 400) {
            this.showToast("Invalid path or name", "error");
          } else {
            this.showToast("Failed to rename: " + errorText, "error");
          }
          return;
        }

        this.showToast("Renamed successfully", "success");
        await this.refreshFiles();
      } catch (error) {
        console.error("Error renaming item:", error);
        this.showToast("Failed to rename", "error");
      }
    },

    async deleteItem(path) {
      try {
        const response = await fetch(`/api/files/${encodeURIComponent(path)}`, {
          method: "DELETE",
        });

        if (!response.ok) {
          const errorText = await response.text();
          if (response.status === 404) {
            this.showToast("File or directory does not exist", "error");
          } else if (response.status === 409) {
            this.showToast("Directory not empty", "error");
          } else if (response.status === 400) {
            this.showToast("Invalid path", "error");
          } else {
            this.showToast("Failed to delete: " + errorText, "error");
          }
          return;
        }

        this.showToast("Deleted successfully", "success");
        await this.refreshFiles();

        // Close tab if deleted file was open
        if (this.tabs.find((tab) => tab.path === path)) {
          this.closeTab(path);
        }
      } catch (error) {
        console.error("Error deleting item:", error);
        this.showToast("Failed to delete", "error");
      }
    },

    // Hugo Control
    async loadHugoStatus() {
      try {
        const res = await fetch("/api/hugo/status");
        this.hugoStatus = await res.json();

        if (this.hugoStatus.status === "running" && !this.previewReady) {
          this.previewReady = true;
          this.previewUrl = `http://localhost:${this.config.hugoPort}/`;
        }
      } catch (err) {
        console.error("Failed to get Hugo status:", err);
      }
    },

    async hugoStart() {
      await fetch("/api/hugo/start", { method: "POST" });
      setTimeout(() => this.loadHugoStatus(), 1000);
    },

    async hugoStop() {
      await fetch("/api/hugo/stop", { method: "POST" });
      this.previewReady = false;
      this.previewUrl = "about:blank";
      this.loadHugoStatus();
    },

    async hugoRestart() {
      await fetch("/api/hugo/restart", { method: "POST" });
      this.previewReady = false;
      setTimeout(() => this.loadHugoStatus(), 2000);
    },

    getStatusText() {
      switch (this.hugoStatus.status) {
        case "running":
          return `Running on :${this.config.hugoPort}`;
        case "starting":
          return "Starting...";
        case "stopped":
          return "Stopped";
        case "error":
          return "Error";
        default:
          return this.hugoStatus.status;
      }
    },

    // WebSocket for logs
    connectWebSocket() {
      const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
      this.ws = new WebSocket(
        `${protocol}//${window.location.host}/api/hugo/ws`,
      );

      this.ws.onmessage = (event) => {
        const log = JSON.parse(event.data);
        this.logs.push(log);
        if (this.logs.length > 500) {
          this.logs = this.logs.slice(-500);
        }

        // Auto-scroll logs
        this.$nextTick(() => {
          const container = document.getElementById("logs-container");
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
        .replace(/^content\//, "/")
        .replace(/\.md$/, "/")
        .replace(/_index\/$/, "");

      if (!url.startsWith("/")) url = "/" + url;
      if (!url.endsWith("/")) url += "/";

      this.previewUrl = `http://localhost:${this.config.hugoPort}${url}`;
    },

    refreshPreview() {
      const iframe = document.getElementById("preview-frame");
      if (iframe) {
        iframe.src = iframe.src;
      }
    },

    previewLoaded() {
      // Preview loaded
    },

    openPreviewExternal() {
      window.open(this.previewUrl, "_blank");
    },

    // Image Upload
    async loadImageFolders() {
      try {
        const res = await fetch("/api/images/folders");
        this.imageFolders = await res.json();
        if (this.imageFolders.length > 0) {
          this.uploadOptions.folder = this.imageFolders[0].path; // Use full path instead of name
        }
      } catch (err) {
        console.error("Failed to load image folders:", err);
      }
    },

    async loadImagePresets() {
      try {
        const res = await fetch("/api/images/presets");
        this.imagePresets = await res.json();
      } catch (err) {
        console.error("Failed to load image presets:", err);
      }
    },

    openImageUpload() {
      this.imageFile = null;
      this.imagePreview = null;
      this.uploadResult = null;
      this.uploadFromContext = false; // Mark as opened from button
      this.showImageModal = true;
    },

    handleImageSelect(event) {
      const file = event.target.files[0];
      if (file) this.setImageFile(file);
    },

    handleDrop(event) {
      this.dragOver = false;
      const file = event.dataTransfer.files[0];
      if (file && file.type.startsWith("image/")) {
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
      formData.append("image", this.imageFile);
      formData.append("folder", this.uploadOptions.folder);
      formData.append("quality", this.uploadOptions.quality);
      formData.append("filename", this.uploadOptions.filename);

      // Get widths from preset
      const preset = this.imagePresets.find(
        (p) => p.name === this.uploadOptions.preset,
      );
      let widths = preset?.widths || [];

      if (
        this.uploadOptions.preset === "Custom" &&
        this.uploadOptions.customWidths
      ) {
        widths = this.uploadOptions.customWidths
          .split(",")
          .map((w) => parseInt(w.trim()))
          .filter((w) => w > 0);
      }

      if (widths.length > 0) {
        formData.append("widths", JSON.stringify(widths));
      }

      try {
        const res = await fetch("/api/images/upload", {
          method: "POST",
          body: formData,
        });

        const data = await res.json();
        if (data.error) {
          this.showToast(data.error, "error");
        } else {
          this.uploadResult = data;
          this.showToast("Image processed successfully", "success");
        }
      } catch (err) {
        this.showToast("Failed to upload image", "error");
      } finally {
        this.uploading = false;
      }
    },

    // File Selector (for shortcode file params)
    async openFileSelector(dataType, callback) {
      this.fileSelectorSearch = "";
      this.fileSelectorCallback = callback;

      try {
        const res = await fetch(`/api/data/${dataType}`);
        this.dataFiles = await res.json();
        this.showFileSelector = true;
      } catch (err) {
        this.showToast("Failed to load files", "error");
      }
    },

    get filteredDataFiles() {
      const search = this.fileSelectorSearch.toLowerCase();
      if (!search) return this.dataFiles;
      return this.dataFiles.filter(
        (f) =>
          f.name.toLowerCase().includes(search) ||
          f.path.toLowerCase().includes(search),
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
      if ((event.ctrlKey || event.metaKey) && event.key === "s") {
        event.preventDefault();
        this.saveCurrentFile();
      }
      // Ctrl/Cmd + B = Bold
      if ((event.ctrlKey || event.metaKey) && event.key === "b") {
        event.preventDefault();
        this.formatBold();
      }
      // Ctrl/Cmd + I = Italic
      if ((event.ctrlKey || event.metaKey) && event.key === "i") {
        event.preventDefault();
        this.formatItalic();
      }
      // Ctrl/Cmd + K = Link
      if ((event.ctrlKey || event.metaKey) && event.key === "k") {
        event.preventDefault();
        this.formatLink();
      }
    },

    // Resize handling
    startResize(event, type) {
      const startX = event.clientX;
      const startWidth =
        type === "sidebar"
          ? this.sidebarWidth
          : document.querySelector(".editor-panel").offsetWidth;

      const onMouseMove = (e) => {
        const delta = e.clientX - startX;
        if (type === "sidebar") {
          this.sidebarWidth = Math.max(200, Math.min(400, startWidth + delta));
        } else {
          this.editorWidth = Math.max(300, startWidth + delta);
        }
      };

      const onMouseUp = () => {
        document.removeEventListener("mousemove", onMouseMove);
        document.removeEventListener("mouseup", onMouseUp);
      };

      document.addEventListener("mousemove", onMouseMove);
      document.addEventListener("mouseup", onMouseUp);
    },

    // Tree rendering
    renderTreeItem(item, depth) {
      const indent = depth * 16;
      const isExpanded = this.expandedDirs.has(item.path);
      const isActive = this.activeTab === item.path;

      let html = `<div class="tree-item" style="padding-left: ${indent}px">`;

      if (item.isDir) {
        html += `
          <div class="tree-item-content" @click="toggleDir('${item.path}')" @contextmenu="showContextMenu($event, ${JSON.stringify(item).replace(/"/g, "&quot;")}, true)">
            <span class="tree-toggle ${isExpanded ? "expanded" : ""}">
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
          html += `<div class="tree-children ${isExpanded ? "expanded" : ""}">`;
          for (const child of item.children) {
            html += this.renderTreeItem(child, depth + 1);
          }
          html += "</div>";
        }
      } else {
        const iconClass = `file-${item.type || "text"}`;
        html += `
          <div class="tree-item-content ${isActive ? "active" : ""}" @click="openFile('${item.path}', '${item.type}')" @contextmenu="showContextMenu($event, ${JSON.stringify(item).replace(/"/g, "&quot;")}, false)">
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

      html += "</div>";
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
        markdown:
          '<svg viewBox="0 0 24 24" fill="currentColor" width="16" height="16"><path d="M14 2H6a2 2 0 00-2 2v16a2 2 0 002 2h12a2 2 0 002-2V8l-6-6zm-1 2l5 5h-5V4zM9.5 11.5l1.5 2 1.5-2H15l-2.25 3L15 17.5h-2.5l-1.5-2-1.5 2H7l2.25-3L7 11.5h2.5z"/></svg>',
        html: '<svg viewBox="0 0 24 24" fill="currentColor" width="16" height="16"><path d="M12 17.56l4.07-1.13.55-6.1H9.38L9.2 8.3h7.6l.2-2.03H7l.56 6.01h6.89l-.23 2.58-2.22.6-2.22-.6-.14-1.66h-2l.29 3.19L12 17.56M3 2h18l-1.64 18L12 22l-7.36-2L3 2z"/></svg>',
        yaml: '<svg viewBox="0 0 24 24" fill="currentColor" width="16" height="16"><path d="M14 2H6a2 2 0 00-2 2v16a2 2 0 002 2h12a2 2 0 002-2V8l-6-6zM6 20V4h7v5h5v11H6z"/></svg>',
        image:
          '<svg viewBox="0 0 24 24" fill="currentColor" width="16" height="16"><path d="M21 19V5c0-1.1-.9-2-2-2H5c-1.1 0-2 .9-2 2v14c0 1.1.9 2 2 2h14c1.1 0 2-.9 2-2zM8.5 13.5l2.5 3.01L14.5 12l4.5 6H5l3.5-4.5z"/></svg>',
        default:
          '<svg viewBox="0 0 24 24" fill="currentColor" width="16" height="16"><path d="M14 2H6a2 2 0 00-2 2v16a2 2 0 002 2h12a2 2 0 002-2V8l-6-6zm4 18H6V4h7v5h5v11z"/></svg>',
      };
      return icons[type] || icons.default;
    },

    getFileIcon(type) {
      return this.getFileIconSvg(type);
    },

    getFileType(path) {
      const ext = path.split(".").pop().toLowerCase();
      const types = {
        md: "markdown",
        html: "html",
        yaml: "yaml",
        yml: "yaml",
        toml: "toml",
        json: "json",
        js: "javascript",
        css: "css",
        jpg: "image",
        jpeg: "image",
        png: "image",
        gif: "image",
        svg: "image",
      };
      return types[ext] || "text";
    },

    getMonacoLanguage(type) {
      const languages = {
        markdown: "markdown",
        html: "html",
        yaml: "yaml",
        toml: "ini",
        json: "json",
        javascript: "javascript",
        css: "css",
      };
      return languages[type] || "plaintext";
    },

    // Toasts
    showToast(message, type = "info") {
      const id = ++this.toastId;
      const toast = { id, message, type, visible: true };
      this.toasts.push(toast);

      setTimeout(() => {
        toast.visible = false;
        setTimeout(() => {
          this.toasts = this.toasts.filter((t) => t.id !== id);
        }, 300);
      }, 3000);
    },

    // Utilities
    formatBytes(bytes) {
      if (bytes === 0) return "0 B";
      const k = 1024;
      const sizes = ["B", "KB", "MB", "GB"];
      const i = Math.floor(Math.log(bytes) / Math.log(k));
      return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + " " + sizes[i];
    },

    async copyToClipboard(text) {
      try {
        await navigator.clipboard.writeText(text);
        this.showToast("Copied to clipboard", "success");
      } catch (err) {
        this.showToast("Failed to copy", "error");
      }
    },
  };
}
