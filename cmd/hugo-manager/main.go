package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/fernandezvara/hugo-manager/internal/config"
	"github.com/fernandezvara/hugo-manager/internal/hugo"
	"github.com/fernandezvara/hugo-manager/internal/server"
	"github.com/fernandezvara/hugo-manager/web"
)

var version = "0.1.0"

func main() {
	// Command line flags
	port := flag.Int("port", 8080, "Port for the web interface")
	hugoPort := flag.Int("hugo-port", 1313, "Port for Hugo server")
	projectDir := flag.String("dir", ".", "Hugo project directory")
	showVersion := flag.Bool("version", false, "Show version")
	initConfig := flag.Bool("init", false, "Initialize hugo-manager.yaml config file")
	flag.Parse()

	if *showVersion {
		fmt.Printf("github.com/fernandezvara/hugo-manager v%s\n", version)
		os.Exit(0)
	}

	// Resolve project directory to absolute path
	absProjectDir, err := filepath.Abs(*projectDir)
	if err != nil {
		log.Fatalf("Failed to resolve project directory: %v", err)
	}

	// Verify it's a Hugo project
	if !isHugoProject(absProjectDir) {
		log.Fatalf("Directory %s doesn't appear to be a Hugo project (no hugo.toml, hugo.yaml, or config.toml found)", absProjectDir)
	}

	// Load or create configuration
	cfg, err := config.Load(absProjectDir)
	if err != nil {
		fmt.Printf("Config load error: %v\n", err)
		if *initConfig {
			cfg = config.Default()
			if err := config.Save(absProjectDir, cfg); err != nil {
				log.Fatalf("Failed to create config: %v", err)
			}
			fmt.Println("Created hugo-manager.yaml with default configuration")
			os.Exit(0)
		}
		// Use default config if none exists
		fmt.Printf("Using default config due to load error\n")
		cfg = config.Default()
	}

	if *initConfig {
		if err := config.Save(absProjectDir, cfg); err != nil {
			log.Fatalf("Failed to save config: %v", err)
		}
		fmt.Println("Configuration saved to hugo-manager.yaml")
		os.Exit(0)
	}

	// Override ports from command line if specified
	if *port != 8080 {
		cfg.Server.Port = *port
	}
	if *hugoPort != 1313 {
		cfg.Hugo.Port = *hugoPort
	}

	log.Printf("Starting hugo-manager v%s", version)
	log.Printf("Project directory: %s", absProjectDir)

	// Create Hugo manager
	hugoMgr := hugo.NewManager(absProjectDir, cfg.Hugo)

	// Create and start the web server
	srv := server.New(absProjectDir, cfg, hugoMgr, web.FS)

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("\nShutting down...")
		hugoMgr.Stop()
		os.Exit(0)
	}()

	// Auto-start Hugo if configured
	if cfg.Hugo.AutoStart {
		if err := hugoMgr.Start(); err != nil {
			log.Printf("Warning: Failed to auto-start Hugo: %v", err)
		}
	}

	// Start the web server
	addr := fmt.Sprintf("localhost:%d", cfg.Server.Port)
	log.Printf("Web interface available at http://%s", addr)
	log.Printf("Hugo server will run at http://localhost:%d", cfg.Hugo.Port)

	if err := srv.Start(addr); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

func isHugoProject(dir string) bool {
	configFiles := []string{
		"hugo.toml",
		"hugo.yaml",
		"hugo.json",
		"config.toml",
		"config.yaml",
		"config.json",
	}

	for _, f := range configFiles {
		if _, err := os.Stat(filepath.Join(dir, f)); err == nil {
			return true
		}
	}
	return false
}
