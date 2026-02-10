package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/jordanhubbard/loom/internal/loom"
	"github.com/jordanhubbard/loom/internal/api"
	"github.com/jordanhubbard/loom/internal/auth"
	"github.com/jordanhubbard/loom/internal/hotreload"
	"github.com/jordanhubbard/loom/internal/keymanager"
	"github.com/jordanhubbard/loom/pkg/config"
)

const version = "0.1.0"

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	configPath := flag.String("config", "config.yaml", "Path to configuration file")
	showVersion := flag.Bool("version", false, "Show version information")
	showHelp := flag.Bool("help", false, "Show help message")
	flag.Parse()

	if *showHelp {
		printHelp()
		return
	}

	if *showVersion {
		fmt.Printf("Loom v%s\n", version)
		return
	}

	cfg, err := config.LoadConfigFromFile(*configPath)
	if err != nil {
		log.Fatalf("failed to load config from %s: %v", *configPath, err)
	}

	// Override with environment variables if set
	if temporalHost := os.Getenv("TEMPORAL_HOST"); temporalHost != "" {
		cfg.Temporal.Host = temporalHost
		log.Printf("Using Temporal host from environment: %s", temporalHost)
	}
	if temporalNamespace := os.Getenv("TEMPORAL_NAMESPACE"); temporalNamespace != "" {
		cfg.Temporal.Namespace = temporalNamespace
		log.Printf("Using Temporal namespace from environment: %s", temporalNamespace)
	}

	arb, err := loom.New(cfg)
	if err != nil {
		log.Fatalf("failed to create loom: %v", err)
	}

	// Initialize key manager before Loom.Initialize() so Temporal activities
	// can use it for provider API key retrieval during heartbeats.
	keyStorePath := filepath.Join(".", ".keys.json")
	km := keymanager.NewKeyManager(keyStorePath)

	password := loadPassword()
	if password == "" {
		log.Printf("Warning: No password found. Using default password. Set LOOM_PASSWORD environment variable or create .env file")
		password = "loom-default-password"
	}

	if err := km.Unlock(password); err != nil {
		log.Printf("Password unlock failed: %v. Trying default password...", err)
		if err := km.Unlock("loom-default-password"); err != nil {
			log.Fatalf("Failed to unlock key manager with both passwords: %v", err)
		}
	}

	arb.SetKeyManager(km)

	runCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := arb.Initialize(runCtx); err != nil {
		log.Fatalf("failed to initialize loom: %v", err)
	}

	// Initialize hot-reload for development
	var hrManager *hotreload.Manager
	if cfg.HotReload.Enabled {
		hrManager, err = hotreload.NewManager(
			cfg.HotReload.Enabled,
			cfg.HotReload.WatchDirs,
			cfg.HotReload.Patterns,
		)
		if err != nil {
			log.Printf("Hot-reload initialization failed: %v", err)
		} else {
			defer hrManager.Close()
		}
	}

	go arb.StartMaintenanceLoop(runCtx)

	// Fallback Ralph Loop: drain all dispatchable work when Temporal is unavailable.
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-runCtx.Done():
				return
			case <-ticker.C:
				if arb.GetTemporalManager() == nil {
					for i := 0; i < 50; i++ {
						dr, err := arb.GetDispatcher().DispatchOnce(runCtx, "")
						if err != nil {
							log.Printf("[Ralph-fallback] Dispatch error: %v", err)
							break
						}
						if dr == nil || !dr.Dispatched {
							break
						}
					}
				}
			}
		}
	}()

	// Initialize auth manager (JWT + API key support)
	authManager := auth.NewManager(cfg.Security.JWTSecret)

	apiServer := api.NewServer(arb, km, authManager, cfg)
	handler := apiServer.SetupRoutes()

	// Add hot-reload WebSocket endpoint if enabled
	if hrManager != nil && hrManager.IsEnabled() {
		mux := http.NewServeMux()
		mux.Handle("/", handler)
		mux.HandleFunc("/ws/hotreload", hrManager.GetServer().HandleWebSocket)
		mux.HandleFunc("/api/v1/hotreload/status", hrManager.GetServer().HandleStatus)
		handler = mux
		log.Println("[HotReload] WebSocket endpoint registered at /ws/hotreload")
	}

	httpSrv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Server.HTTPPort),
		Handler:      handler,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}

	go func() {
		log.Printf("Loom API listening on %s", httpSrv.Addr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http server error: %v", err)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	cancel()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_ = httpSrv.Shutdown(shutdownCtx)
	arb.Shutdown()

}

func loadPassword() string {
	// First, check environment variable
	if pwd := os.Getenv("LOOM_PASSWORD"); pwd != "" {
		return pwd
	}

	// Second, try to load from .env file
	if envData, err := os.ReadFile(".env"); err == nil {
		lines := strings.Split(string(envData), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			// Skip comments and empty lines
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			// Look for LOOM_PASSWORD=value
			if strings.HasPrefix(line, "LOOM_PASSWORD=") {
				pwd := strings.TrimPrefix(line, "LOOM_PASSWORD=")
				pwd = strings.Trim(pwd, "\"'")
				return pwd
			}
		}
	}

	return ""
}

func printHelp() {
	fmt.Println("Usage: loom [flags]")
	fmt.Println()
	fmt.Println("Flags:")
	fmt.Println("  -config   Path to configuration file (default: config.yaml)")
	fmt.Println("  -version  Show version information")
	fmt.Println("  -help     Show help message")
	fmt.Println()
	fmt.Println("Environment:")
	fmt.Println("  LOOM_PASSWORD  Master password for UI login and key encryption")
}
