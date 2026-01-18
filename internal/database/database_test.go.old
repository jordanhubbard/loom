package database

import (
	"path/filepath"
	"testing"

	"github.com/jordanhubbard/arbiter/internal/models"
)

func TestDatabase(t *testing.T) {
	// Create temporary database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := New(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Test provider CRUD operations
	t.Run("Provider CRUD", func(t *testing.T) {
		provider := &models.Provider{
			ID:          "test-provider-1",
			Name:        "Test Provider",
			Type:        "openai",
			Endpoint:    "https://api.example.com",
			Description: "Test provider",
			RequiresKey: true,
			KeyID:       "key-123",
			Status:      "active",
		}

		// Create
		if err := db.CreateProvider(provider); err != nil {
			t.Fatalf("Failed to create provider: %v", err)
		}

		// Read
		retrieved, err := db.GetProvider(provider.ID)
		if err != nil {
			t.Fatalf("Failed to get provider: %v", err)
		}

		if retrieved.Name != provider.Name {
			t.Errorf("Name mismatch: got %s, want %s", retrieved.Name, provider.Name)
		}

		// Update
		provider.Name = "Updated Provider"
		if err := db.UpdateProvider(provider); err != nil {
			t.Fatalf("Failed to update provider: %v", err)
		}

		updated, err := db.GetProvider(provider.ID)
		if err != nil {
			t.Fatalf("Failed to get updated provider: %v", err)
		}

		if updated.Name != "Updated Provider" {
			t.Errorf("Name not updated: got %s, want %s", updated.Name, "Updated Provider")
		}

		// List
		providers, err := db.ListProviders()
		if err != nil {
			t.Fatalf("Failed to list providers: %v", err)
		}

		if len(providers) != 1 {
			t.Errorf("Expected 1 provider, got %d", len(providers))
		}

		// Delete
		if err := db.DeleteProvider(provider.ID); err != nil {
			t.Fatalf("Failed to delete provider: %v", err)
		}

		_, err = db.GetProvider(provider.ID)
		if err == nil {
			t.Error("Expected error when getting deleted provider")
		}
	})

	// Test agent CRUD operations
	t.Run("Agent CRUD", func(t *testing.T) {
		// First create a provider
		provider := &models.Provider{
			ID:          "test-provider-2",
			Name:        "Test Provider 2",
			Type:        "anthropic",
			Endpoint:    "https://api.anthropic.com",
			Description: "Test provider for agents",
			RequiresKey: true,
			Status:      "active",
		}

		if err := db.CreateProvider(provider); err != nil {
			t.Fatalf("Failed to create provider: %v", err)
		}

		agent := &models.Agent{
			ID:          "test-agent-1",
			Name:        "Test Agent",
			Description: "Test agent",
			ProviderID:  provider.ID,
			Status:      "active",
			Config:      `{"model": "claude-3"}`,
		}

		// Create
		if err := db.CreateAgent(agent); err != nil {
			t.Fatalf("Failed to create agent: %v", err)
		}

		// Read
		retrieved, err := db.GetAgent(agent.ID)
		if err != nil {
			t.Fatalf("Failed to get agent: %v", err)
		}

		if retrieved.Name != agent.Name {
			t.Errorf("Name mismatch: got %s, want %s", retrieved.Name, agent.Name)
		}

		// Update
		agent.Name = "Updated Agent"
		if err := db.UpdateAgent(agent); err != nil {
			t.Fatalf("Failed to update agent: %v", err)
		}

		updated, err := db.GetAgent(agent.ID)
		if err != nil {
			t.Fatalf("Failed to get updated agent: %v", err)
		}

		if updated.Name != "Updated Agent" {
			t.Errorf("Name not updated: got %s, want %s", updated.Name, "Updated Agent")
		}

		// List by provider
		agents, err := db.ListAgentsByProvider(provider.ID)
		if err != nil {
			t.Fatalf("Failed to list agents by provider: %v", err)
		}

		if len(agents) != 1 {
			t.Errorf("Expected 1 agent, got %d", len(agents))
		}

		// List all
		allAgents, err := db.ListAgents()
		if err != nil {
			t.Fatalf("Failed to list agents: %v", err)
		}

		if len(allAgents) != 1 {
			t.Errorf("Expected 1 agent, got %d", len(allAgents))
		}

		// Delete
		if err := db.DeleteAgent(agent.ID); err != nil {
			t.Fatalf("Failed to delete agent: %v", err)
		}

		_, err = db.GetAgent(agent.ID)
		if err == nil {
			t.Error("Expected error when getting deleted agent")
		}
	})

	// Test foreign key constraint
	t.Run("Foreign Key Constraint", func(t *testing.T) {
		agent := &models.Agent{
			ID:          "orphan-agent",
			Name:        "Orphan Agent",
			Description: "Agent with no provider",
			ProviderID:  "non-existent-provider",
			Status:      "active",
		}

		err := db.CreateAgent(agent)
		if err == nil {
			t.Error("Expected error when creating agent with non-existent provider")
		}
	})
}
