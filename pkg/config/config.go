package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jordanhubbard/arbiter/pkg/secrets"
	"gopkg.in/yaml.v3"
)

// Config represents the main configuration for the arbiter system.
// It supports both YAML-based configuration (for file-based config using LoadConfigFromFile)
// and JSON-based configuration (for user-specific config using LoadConfig).
type Config struct {
	// YAML/File-based configuration fields
	Server      ServerConfig      `yaml:"server" json:"server,omitempty"`
	Database    DatabaseConfig    `yaml:"database" json:"database,omitempty"`
	Beads       BeadsConfig       `yaml:"beads" json:"beads,omitempty"`
	Agents      AgentsConfig      `yaml:"agents" json:"agents,omitempty"`
	Security    SecurityConfig    `yaml:"security" json:"security,omitempty"`
	Projects    []ProjectConfig   `yaml:"projects" json:"projects,omitempty"`
	WebUI       WebUIConfig       `yaml:"web_ui" json:"web_ui,omitempty"`
	Temporal    TemporalConfig    `yaml:"temporal" json:"temporal,omitempty"`
	
	// JSON/User-specific configuration fields
	Providers   []Provider        `yaml:"providers,omitempty" json:"providers"`
	ServerPort  int               `yaml:"server_port,omitempty" json:"server_port"`
	SecretStore *secrets.Store    `yaml:"-" json:"-"`
}

// ServerConfig configures the HTTP/HTTPS server
type ServerConfig struct {
	HTTPPort      int           `yaml:"http_port"`
	HTTPSPort     int           `yaml:"https_port"`
	EnableHTTP    bool          `yaml:"enable_http"`
	EnableHTTPS   bool          `yaml:"enable_https"`
	TLSCertFile   string        `yaml:"tls_cert_file"`
	TLSKeyFile    string        `yaml:"tls_key_file"`
	ReadTimeout   time.Duration `yaml:"read_timeout"`
	WriteTimeout  time.Duration `yaml:"write_timeout"`
	IdleTimeout   time.Duration `yaml:"idle_timeout"`
}

// DatabaseConfig configures the local storage
type DatabaseConfig struct {
	Type string `yaml:"type"` // "sqlite", "postgres"
	Path string `yaml:"path"` // For SQLite
	DSN  string `yaml:"dsn"`  // For Postgres
}

// BeadsConfig configures beads integration
type BeadsConfig struct {
	BDPath         string        `yaml:"bd_path"`          // Path to bd executable
	AutoSync       bool          `yaml:"auto_sync"`
	SyncInterval   time.Duration `yaml:"sync_interval"`
	CompactOldDays int           `yaml:"compact_old_days"` // Days before compacting closed beads
}

// AgentsConfig configures agent behavior
type AgentsConfig struct {
	MaxConcurrent      int           `yaml:"max_concurrent"`
	DefaultPersonaPath string        `yaml:"default_persona_path"`
	HeartbeatInterval  time.Duration `yaml:"heartbeat_interval"`
	FileLockTimeout    time.Duration `yaml:"file_lock_timeout"`
}

// SecurityConfig configures authentication and authorization
type SecurityConfig struct {
	EnableAuth     bool     `yaml:"enable_auth"`
	PKIEnabled     bool     `yaml:"pki_enabled"`
	CAFile         string   `yaml:"ca_file"`
	RequireHTTPS   bool     `yaml:"require_https"`
	AllowedOrigins []string `yaml:"allowed_origins"` // CORS
	APIKeys        []string `yaml:"api_keys,omitempty"`
}

// TemporalConfig configures Temporal workflow engine
type TemporalConfig struct {
	Host                     string        `yaml:"host"`
	Namespace                string        `yaml:"namespace"`
	TaskQueue                string        `yaml:"task_queue"`
	WorkflowExecutionTimeout time.Duration `yaml:"workflow_execution_timeout"`
	WorkflowTaskTimeout      time.Duration `yaml:"workflow_task_timeout"`
	EnableEventBus           bool          `yaml:"enable_event_bus"`
	EventBufferSize          int           `yaml:"event_buffer_size"`
}

// ProjectConfig represents a project configuration
type ProjectConfig struct {
	ID        string            `yaml:"id"`
	Name      string            `yaml:"name"`
	GitRepo   string            `yaml:"git_repo"`
	Branch    string            `yaml:"branch"`
	BeadsPath string            `yaml:"beads_path"`
	Context   map[string]string `yaml:"context"`
}

// WebUIConfig configures the web interface
type WebUIConfig struct {
	Enabled         bool   `yaml:"enabled"`
	StaticPath      string `yaml:"static_path"`
	RefreshInterval int    `yaml:"refresh_interval"` // seconds
}

// LoadConfigFromFile loads configuration from a YAML file at the specified path.
// This is typically used for loading system-wide or project-specific configuration.
func LoadConfigFromFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

// LoadConfig loads user-specific configuration from the default JSON config file.
// This is typically used for loading user preferences and provider settings.
// The config file is stored at ~/.arbiter.json
func LoadConfig() (*Config, error) {
	configPath, err := getConfigPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	// Initialize secret store
	cfg.SecretStore = secrets.NewStore()
	if err := cfg.SecretStore.Load(); err != nil {
		return nil, fmt.Errorf("failed to load secrets: %w", err)
	}

	return &cfg, nil
}

// DefaultConfig returns a default configuration
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			HTTPPort:     8080,
			HTTPSPort:    8443,
			EnableHTTP:   true,
			EnableHTTPS:  false,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
			IdleTimeout:  120 * time.Second,
		},
		Database: DatabaseConfig{
			Type: "sqlite",
			Path: "./arbiter.db",
		},
		Beads: BeadsConfig{
			BDPath:         "bd",
			AutoSync:       true,
			SyncInterval:   5 * time.Minute,
			CompactOldDays: 90,
		},
		Agents: AgentsConfig{
			MaxConcurrent:      10,
			DefaultPersonaPath: "./personas",
			HeartbeatInterval:  30 * time.Second,
			FileLockTimeout:    10 * time.Minute,
		},
		Security: SecurityConfig{
			EnableAuth:     true,
			PKIEnabled:     false,
			RequireHTTPS:   false,
			AllowedOrigins: []string{"*"},
		},
		Temporal: TemporalConfig{
			Host:                     "localhost:7233",
			Namespace:                "arbiter-default",
			TaskQueue:                "arbiter-tasks",
			WorkflowExecutionTimeout: 24 * time.Hour,
			WorkflowTaskTimeout:      10 * time.Second,
			EnableEventBus:           true,
			EventBufferSize:          1000,
		},
		WebUI: WebUIConfig{
			Enabled:         true,
			StaticPath:      "./web/static",
			RefreshInterval: 5,
		},
	}
}
