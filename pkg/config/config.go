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
	BDPath         string        `yaml:"bd_path"`           // Path to bd executable
	AutoSync       bool          `yaml:"auto_sync"`
	SyncInterval   time.Duration `yaml:"sync_interval"`
	CompactOldDays int           `yaml:"compact_old_days"`  // Days before compacting closed beads
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

// ProjectConfig represents a project configuration
type ProjectConfig struct {
	ID          string            `yaml:"id"`
	Name        string            `yaml:"name"`
	GitRepo     string            `yaml:"git_repo"`
	Branch      string            `yaml:"branch"`
	BeadsPath   string            `yaml:"beads_path"`
	IsPerpetual bool              `yaml:"is_perpetual"` // If true, project never closes
	Context     map[string]string `yaml:"context"`
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
		WebUI: WebUIConfig{
			Enabled:         true,
			StaticPath:      "./web/static",
			RefreshInterval: 5,
		},
	}
}

const (
	configFileName = ".arbiter.json"
)

// Provider represents an AI service provider configuration
type Provider struct {
	Name     string `json:"name"`
	Endpoint string `json:"endpoint"`
}

// NewConfig creates a new configuration with default values
func NewConfig() *Config {
	return &Config{
		Providers:   []Provider{},
		ServerPort:  8080,
		SecretStore: secrets.NewStore(),
	}
}

// SaveConfig saves configuration to the config file
func SaveConfig(cfg *Config) error {
	configPath, err := getConfigPath()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(configPath, data, 0600); err != nil {
		return err
	}

	// Save secrets separately
	if cfg.SecretStore != nil {
		if err := cfg.SecretStore.Save(); err != nil {
			return fmt.Errorf("failed to save secrets: %w", err)
		}
	}

	return nil
}

// getConfigPath returns the path to the configuration file
func getConfigPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, configFileName), nil
}

// LookupProviderEndpoint attempts to find the API endpoint for a provider
func LookupProviderEndpoint(providerName string) (string, error) {
	// Map of known providers to their default endpoints
	knownProviders := map[string]string{
		"claude":      "https://api.anthropic.com/v1",
		"openai":      "https://api.openai.com/v1",
		"cursor":      "https://api.cursor.sh/v1",
		"factory":     "https://api.factory.ai/v1",
		"cohere":      "https://api.cohere.ai/v1",
		"huggingface": "https://api-inference.huggingface.co",
		"replicate":   "https://api.replicate.com/v1",
		"together":    "https://api.together.xyz/v1",
		"mistral":     "https://api.mistral.ai/v1",
		"perplexity":  "https://api.perplexity.ai",
	}

	// Check if we have a known endpoint
	if endpoint, ok := knownProviders[providerName]; ok {
		return endpoint, nil
	}

	// For unknown providers, we would use Google's Custom Search API here
	// For now, return an error to prompt for manual entry
	return "", fmt.Errorf("unknown provider: %s", providerName)
}
