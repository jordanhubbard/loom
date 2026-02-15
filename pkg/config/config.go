package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jordanhubbard/loom/pkg/secrets"
	"gopkg.in/yaml.v3"
)

const configFileName = ".loom.json"

// Provider represents an AI service provider configuration (file/JSON config).
type Provider struct {
	ID       string `yaml:"id" json:"id"`
	Name     string `yaml:"name" json:"name"`
	Type     string `yaml:"type" json:"type"`
	Endpoint string `yaml:"endpoint" json:"endpoint"`
	APIKey   string `yaml:"api_key" json:"api_key"`
	Model    string `yaml:"model" json:"model"`
	Enabled  bool   `yaml:"enabled" json:"enabled"`
}

// Config represents the main configuration for the loom system.
// It supports both YAML-based configuration (for file-based config using LoadConfigFromFile)
// and JSON-based configuration (for user-specific config using LoadConfig).
type Config struct {
	// YAML/File-based configuration fields
	Server    ServerConfig    `yaml:"server" json:"server,omitempty"`
	Database  DatabaseConfig  `yaml:"database" json:"database,omitempty"`
	Beads     BeadsConfig     `yaml:"beads" json:"beads,omitempty"`
	Agents    AgentsConfig    `yaml:"agents" json:"agents,omitempty"`
	Security  SecurityConfig  `yaml:"security" json:"security,omitempty"`
	Cache     CacheConfig     `yaml:"cache" json:"cache,omitempty"`
	Readiness ReadinessConfig `yaml:"readiness" json:"readiness,omitempty"`
	Dispatch  DispatchConfig  `yaml:"dispatch" json:"dispatch,omitempty"`
	Git       GitConfig       `yaml:"git" json:"git,omitempty"`
	Models    ModelsConfig    `yaml:"models" json:"models,omitempty"`
	Projects  []ProjectConfig `yaml:"projects" json:"projects,omitempty"`
	WebUI     WebUIConfig     `yaml:"web_ui" json:"web_ui,omitempty"`
	Temporal  TemporalConfig  `yaml:"temporal" json:"temporal,omitempty"`
	HotReload HotReloadConfig `yaml:"hot_reload" json:"hot_reload,omitempty"`
	OpenClaw  OpenClawConfig  `yaml:"openclaw" json:"openclaw,omitempty"`

	// JSON/User-specific configuration fields
	Providers   []Provider     `yaml:"providers,omitempty" json:"providers"`
	ServerPort  int            `yaml:"server_port,omitempty" json:"server_port"`
	SecretStore *secrets.Store `yaml:"-" json:"-"`
}

// ServerConfig configures the HTTP/HTTPS server
type ServerConfig struct {
	HTTPPort     int           `yaml:"http_port"`
	HTTPSPort    int           `yaml:"https_port"`
	EnableHTTP   bool          `yaml:"enable_http"`
	EnableHTTPS  bool          `yaml:"enable_https"`
	TLSCertFile  string        `yaml:"tls_cert_file"`
	TLSKeyFile   string        `yaml:"tls_key_file"`
	ReadTimeout  time.Duration `yaml:"read_timeout"`
	WriteTimeout time.Duration `yaml:"write_timeout"`
	IdleTimeout  time.Duration `yaml:"idle_timeout"`
}

// DatabaseConfig configures the local storage
type DatabaseConfig struct {
	Type string `yaml:"type"` // "sqlite", "postgres"
	Path string `yaml:"path"` // For SQLite
	DSN  string `yaml:"dsn"`  // For Postgres
}

// BeadsConfig configures beads integration
type BeadsConfig struct {
	BDPath         string                `yaml:"bd_path"` // Path to bd executable
	AutoSync       bool                  `yaml:"auto_sync"`
	SyncInterval   time.Duration         `yaml:"sync_interval"`
	CompactOldDays int                   `yaml:"compact_old_days"` // Days before compacting closed beads
	Backend        string                `yaml:"backend"`          // "sqlite" or "dolt"
	Federation     BeadsFederationConfig `yaml:"federation"`
}

// BeadsFederationConfig configures peer-to-peer federation via Dolt remotes
type BeadsFederationConfig struct {
	Enabled      bool             `yaml:"enabled"`
	AutoSync     bool             `yaml:"auto_sync"`      // Sync with peers on startup
	SyncInterval time.Duration    `yaml:"sync_interval"`  // Periodic sync interval (0 = disabled)
	SyncStrategy string           `yaml:"sync_strategy"`  // "ours", "theirs", or "" (manual)
	SyncMode     string           `yaml:"sync_mode"`      // "dolt-native" or "belt-and-suspenders"
	Peers        []FederationPeer `yaml:"peers"`
}

// FederationPeer represents a federation peer configuration
type FederationPeer struct {
	Name        string `yaml:"name"`
	RemoteURL   string `yaml:"remote_url"`
	Enabled     bool   `yaml:"enabled"`
	Description string `yaml:"description,omitempty"`
}

// AgentsConfig configures agent behavior
type AgentsConfig struct {
	MaxConcurrent      int           `yaml:"max_concurrent"`
	DefaultPersonaPath string        `yaml:"default_persona_path"`
	HeartbeatInterval  time.Duration `yaml:"heartbeat_interval"`
	FileLockTimeout    time.Duration `yaml:"file_lock_timeout"`
	CorpProfile        string        `yaml:"corp_profile" json:"corp_profile,omitempty"`
	AllowedRoles       []string      `yaml:"allowed_roles" json:"allowed_roles,omitempty"`
}

// ReadinessConfig controls readiness gating behavior
type ReadinessConfig struct {
	Mode string `yaml:"mode" json:"mode,omitempty"`
}

// DispatchConfig controls dispatcher guardrails
type DispatchConfig struct {
	MaxHops int `yaml:"max_hops" json:"max_hops,omitempty"`
}

// GitConfig controls git-related settings
type GitConfig struct {
	ProjectKeyDir string `yaml:"project_key_dir" json:"project_key_dir,omitempty"`
}

// ModelsConfig configures model preferences for provider negotiation
type ModelsConfig struct {
	PreferredModels []PreferredModel `yaml:"preferred_models" json:"preferred_models,omitempty"`
}

// PreferredModel represents a model preference for negotiation with providers.
// When a provider returns multiple models, Loom selects the best match from this list.
type PreferredModel struct {
	Name      string `yaml:"name" json:"name"`                               // Full model name (e.g., "Qwen/Qwen2.5-Coder-32B-Instruct")
	Rank      int    `yaml:"rank" json:"rank"`                               // Priority rank (1 = most preferred)
	Tier      string `yaml:"tier" json:"tier,omitempty"`                     // Complexity tier: "extended", "complex", "medium", "simple"
	MinVRAMGB int    `yaml:"min_vram_gb" json:"min_vram_gb,omitempty"`       // Minimum VRAM required (0 = cloud/unknown)
	Notes     string `yaml:"notes" json:"notes,omitempty"`                   // Human-readable notes about the model
}

// SecurityConfig configures authentication and authorization
type SecurityConfig struct {
	EnableAuth     bool     `yaml:"enable_auth"`
	PKIEnabled     bool     `yaml:"pki_enabled"`
	CAFile         string   `yaml:"ca_file"`
	RequireHTTPS   bool     `yaml:"require_https"`
	AllowedOrigins []string `yaml:"allowed_origins"` // CORS
	APIKeys        []string `yaml:"api_keys,omitempty"`
	JWTSecret      string   `yaml:"jwt_secret" json:"jwt_secret,omitempty"`
	WebhookSecret  string   `yaml:"webhook_secret" json:"webhook_secret,omitempty"` // GitHub webhook secret
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

// CacheConfig configures response caching
type CacheConfig struct {
	Enabled       bool          `yaml:"enabled" json:"enabled"`
	Backend       string        `yaml:"backend" json:"backend"` // "memory" or "redis"
	DefaultTTL    time.Duration `yaml:"default_ttl" json:"default_ttl"`
	MaxSize       int           `yaml:"max_size" json:"max_size"`
	MaxMemoryMB   int           `yaml:"max_memory_mb" json:"max_memory_mb"`
	CleanupPeriod time.Duration `yaml:"cleanup_period" json:"cleanup_period"`
	RedisURL      string        `yaml:"redis_url" json:"redis_url,omitempty"` // Redis connection URL
}

// ProjectConfig represents a project configuration
type ProjectConfig struct {
	ID              string            `yaml:"id"`
	Name            string            `yaml:"name"`
	GitRepo         string            `yaml:"git_repo"`
	Branch          string            `yaml:"branch"`
	BeadsPath       string            `yaml:"beads_path"`
	GitAuthMethod   string            `yaml:"git_auth_method" json:"git_auth_method,omitempty"`
	GitStrategy     string            `yaml:"git_strategy" json:"git_strategy,omitempty"`
	GitCredentialID string            `yaml:"git_credential_id" json:"git_credential_id,omitempty"`
	IsPerpetual     bool              `yaml:"is_perpetual" json:"is_perpetual,omitempty"`
	IsSticky        bool              `yaml:"is_sticky" json:"is_sticky,omitempty"`
	Context         map[string]string `yaml:"context"`
}

// WebUIConfig configures the web interface
type WebUIConfig struct {
	Enabled         bool   `yaml:"enabled"`
	StaticPath      string `yaml:"static_path"`
	RefreshInterval int    `yaml:"refresh_interval"` // seconds
}

// HotReloadConfig configures development hot-reload
type HotReloadConfig struct {
	Enabled   bool     `yaml:"enabled"`
	WatchDirs []string `yaml:"watch_dirs"` // Directories to watch
	Patterns  []string `yaml:"patterns"`   // File patterns to watch (e.g. "*.js", "*.css")
}

// OpenClawConfig configures the OpenClaw messaging gateway integration.
// OpenClaw acts as a bidirectional bridge between loom and human messaging
// platforms (WhatsApp, Signal, Slack, Telegram, etc.) for P0 decision escalations.
type OpenClawConfig struct {
	Enabled          bool          `yaml:"enabled" json:"enabled"`
	GatewayURL       string        `yaml:"gateway_url" json:"gateway_url,omitempty"`
	HookToken        string        `yaml:"hook_token" json:"hook_token,omitempty"`         // Bearer token for outbound POST to /hooks/agent
	WebhookSecret    string        `yaml:"webhook_secret" json:"webhook_secret,omitempty"` // HMAC secret for inbound webhook verification
	DefaultChannel   string        `yaml:"default_channel" json:"default_channel,omitempty"`
	DefaultRecipient string        `yaml:"default_recipient" json:"default_recipient,omitempty"`
	AgentID          string        `yaml:"agent_id" json:"agent_id,omitempty"`
	Timeout          time.Duration `yaml:"timeout" json:"timeout,omitempty"`
	RetryAttempts    int           `yaml:"retry_attempts" json:"retry_attempts,omitempty"`
	RetryDelay       time.Duration `yaml:"retry_delay" json:"retry_delay,omitempty"`
	EscalationsOnly  bool          `yaml:"escalations_only" json:"escalations_only"` // Only send P0/CEO-escalated decisions
}

// LoadConfigFromFile loads configuration from a YAML file at the specified path.
// This is typically used for loading system-wide or project-specific configuration.
func LoadConfigFromFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Expand environment variables (e.g. ${NVIDIA_API_KEY}) before parsing YAML
	expanded := os.ExpandEnv(string(data))

	var config Config
	if err := yaml.Unmarshal([]byte(expanded), &config); err != nil {
		return nil, err
	}

	return &config, nil
}

// LoadConfig loads user-specific configuration from the default JSON config file.
// This is typically used for loading user preferences and provider settings.
// The config file is stored at ~/.loom.json
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
			Path: "./loom.db",
		},
		Beads: BeadsConfig{
			BDPath:         "bd",
			AutoSync:       true,
			SyncInterval:   5 * time.Minute,
			CompactOldDays: 90,
			Backend:        "sqlite",
			Federation: BeadsFederationConfig{
				Enabled:  false,
				AutoSync: true,
			},
		},
		Agents: AgentsConfig{
			MaxConcurrent:      10,
			DefaultPersonaPath: "./personas",
			HeartbeatInterval:  30 * time.Second,
			FileLockTimeout:    10 * time.Minute,
			CorpProfile:        "full",
		},
		Readiness: ReadinessConfig{
			Mode: "block",
		},
		Dispatch: DispatchConfig{
			MaxHops: 20,
		},
		Git: GitConfig{
			ProjectKeyDir: "/app/data/projects",
		},
		Security: SecurityConfig{
			EnableAuth:     true,
			PKIEnabled:     false,
			RequireHTTPS:   false,
			AllowedOrigins: []string{"*"},
			JWTSecret:      "",
		},
		Temporal: TemporalConfig{
			Host:                     "localhost:7233",
			Namespace:                "loom-default",
			TaskQueue:                "loom-tasks",
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
		OpenClaw: OpenClawConfig{
			Enabled:         false,
			GatewayURL:      "http://127.0.0.1:18789",
			AgentID:         "loom",
			Timeout:         30 * time.Second,
			RetryAttempts:   3,
			RetryDelay:      2 * time.Second,
			EscalationsOnly: true,
		},
	}
}

func getConfigPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, configFileName), nil
}
