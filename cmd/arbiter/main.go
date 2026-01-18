package main

import (
	"fmt"
	"log"
	"os"

	"github.com/jordanhubbard/arbiter/internal/config"
	"github.com/jordanhubbard/arbiter/internal/database"
	"github.com/jordanhubbard/arbiter/internal/keymanager"
	"github.com/jordanhubbard/arbiter/internal/models"
)

const version = "0.1.0"

// Arbiter is the main orchestrator that manages agents and providers
type Arbiter struct {
	db         *database.Database
	keyManager *keymanager.KeyManager
	config     *config.Config
}

// NewArbiter creates a new arbiter instance
func NewArbiter(cfg *config.Config) (*Arbiter, error) {
	// Initialize database
	db, err := database.New(cfg.DatabasePath)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	// Initialize key manager
	km := keymanager.NewKeyManager(cfg.KeyStorePath)

	// Get password and unlock key store
	password, err := config.GetPassword()
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to get password: %w", err)
	}

	if err := km.Unlock(password); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to unlock key store: %w", err)
	}

	log.Println("Arbiter initialized successfully")
	log.Printf("Database: %s", cfg.DatabasePath)
	log.Printf("Key Store: %s", cfg.KeyStorePath)

	return &Arbiter{
		db:         db,
		keyManager: km,
		config:     cfg,
	}, nil
}

// Close cleans up arbiter resources
func (a *Arbiter) Close() error {
	// Lock the key manager to clear sensitive data
	a.keyManager.Lock()

	// Close database
	if err := a.db.Close(); err != nil {
		return fmt.Errorf("failed to close database: %w", err)
	}

	return nil
}

// CreateProvider creates a new provider with optional credentials
func (a *Arbiter) CreateProvider(provider *models.Provider, apiKey string) error {
	// If provider requires a key and one is provided, store it
	if provider.RequiresKey && apiKey != "" {
		keyID := fmt.Sprintf("key_%s", provider.ID)
		if err := a.keyManager.StoreKey(keyID, provider.Name, "API Key for "+provider.Name, apiKey); err != nil {
			return fmt.Errorf("failed to store provider key: %w", err)
		}
		provider.KeyID = keyID
	}

	// Create provider in database
	if err := a.db.CreateProvider(provider); err != nil {
		return fmt.Errorf("failed to create provider: %w", err)
	}

	log.Printf("Created provider: %s (%s)", provider.Name, provider.ID)
	return nil
}

// GetProviderWithKey retrieves a provider and its decrypted API key
func (a *Arbiter) GetProviderWithKey(id string) (*models.Provider, string, error) {
	provider, err := a.db.GetProvider(id)
	if err != nil {
		return nil, "", err
	}

	var apiKey string
	if provider.RequiresKey && provider.KeyID != "" {
		key, err := a.keyManager.GetKey(provider.KeyID)
		if err != nil {
			return nil, "", fmt.Errorf("failed to retrieve provider key: %w", err)
		}
		apiKey = key
	}

	return provider, apiKey, nil
}

// CreateAgent creates a new agent
func (a *Arbiter) CreateAgent(agent *models.Agent) error {
	// Verify provider exists
	if _, err := a.db.GetProvider(agent.ProviderID); err != nil {
		return fmt.Errorf("provider not found: %w", err)
	}

	// Create agent in database
	if err := a.db.CreateAgent(agent); err != nil {
		return fmt.Errorf("failed to create agent: %w", err)
	}

	log.Printf("Created agent: %s (%s) using provider %s", agent.Name, agent.ID, agent.ProviderID)
	return nil
}

// GetAgentWithProvider retrieves an agent along with its provider information
func (a *Arbiter) GetAgentWithProvider(agentID string) (*models.Agent, *models.Provider, string, error) {
	agent, err := a.db.GetAgent(agentID)
	if err != nil {
		return nil, nil, "", err
	}

	provider, apiKey, err := a.GetProviderWithKey(agent.ProviderID)
	if err != nil {
		return nil, nil, "", err
	}

	return agent, provider, apiKey, nil
}

// ListProviders returns all providers
func (a *Arbiter) ListProviders() ([]*models.Provider, error) {
	return a.db.ListProviders()
}

// ListAgents returns all agents
func (a *Arbiter) ListAgents() ([]*models.Agent, error) {
	return a.db.ListAgents()
}

// UpdateProvider updates a provider
func (a *Arbiter) UpdateProvider(provider *models.Provider) error {
	return a.db.UpdateProvider(provider)
}

// UpdateAgent updates an agent
func (a *Arbiter) UpdateAgent(agent *models.Agent) error {
	return a.db.UpdateAgent(agent)
}

// DeleteProvider deletes a provider and its associated key
func (a *Arbiter) DeleteProvider(id string) error {
	// Get provider to find key ID
	provider, err := a.db.GetProvider(id)
	if err != nil {
		return err
	}

	// Delete associated key if it exists
	if provider.KeyID != "" {
		if err := a.keyManager.DeleteKey(provider.KeyID); err != nil {
			log.Printf("Warning: failed to delete provider key: %v", err)
		}
	}

	// Delete provider from database
	return a.db.DeleteProvider(id)
}

// DeleteAgent deletes an agent
func (a *Arbiter) DeleteAgent(id string) error {
	return a.db.DeleteAgent(id)
}

func main() {
	// Get default configuration
	cfg, err := config.Default()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Create arbiter instance
	arbiter, err := NewArbiter(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize arbiter: %v", err)
	}
	defer arbiter.Close()

	// Example: Create a sample provider
	provider := &models.Provider{
		ID:          "openai-gpt4",
		Name:        "OpenAI GPT-4",
		Type:        "openai",
		Endpoint:    "https://api.openai.com/v1",
		Description: "OpenAI GPT-4 API",
		RequiresKey: true,
		Status:      "active",
	}

	// Note: In real usage, the API key would be provided by the user
	// For this example, we skip creating the provider if no key is provided
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey != "" {
		if err := arbiter.CreateProvider(provider, apiKey); err != nil {
			log.Printf("Note: Could not create example provider: %v", err)
		}
	}

	// List all providers
	providers, err := arbiter.ListProviders()
	if err != nil {
		log.Fatalf("Failed to list providers: %v", err)
	}

	log.Printf("Total providers: %d", len(providers))
	for _, p := range providers {
		log.Printf("  - %s (%s): %s", p.Name, p.Type, p.Status)
	}

	log.Println("Arbiter is ready to orchestrate agents and providers")
func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	fmt.Printf("Arbiter v%s - Agentic Coding Orchestrator\n", version)
	fmt.Println("An agentic based coding orchestrator for both on-prem and off-prem development")
	fmt.Println()

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "version", "--version", "-v":
			fmt.Printf("Version: %s\n", version)
		case "help", "--help", "-h":
			printHelp()
		default:
			fmt.Printf("Unknown command: %s\n", os.Args[1])
			printHelp()
			os.Exit(1)
		}
	} else {
		fmt.Println("Starting arbiter service...")
		fmt.Println("Ready to orchestrate coding tasks")
		// Main service loop would go here
	}
}

func printHelp() {
	fmt.Println("Usage: arbiter [command]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  version    Display version information")
	fmt.Println("  help       Display this help message")
	fmt.Println()
	fmt.Println("When run without commands, arbiter starts in service mode")
}
