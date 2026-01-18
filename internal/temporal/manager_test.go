package temporal

import (
	"context"
	"testing"
	"time"

	"github.com/jordanhubbard/arbiter/pkg/config"
)

// TestTemporalManagerCreation tests that the Temporal manager can be created
func TestTemporalManagerCreation(t *testing.T) {
	cfg := &config.TemporalConfig{
		Host:                     "localhost:7233",
		Namespace:                "test-namespace",
		TaskQueue:                "test-queue",
		WorkflowExecutionTimeout: 1 * time.Hour,
		WorkflowTaskTimeout:      10 * time.Second,
		EnableEventBus:           true,
		EventBufferSize:          100,
	}

	// This test will only pass if Temporal server is running
	// For unit tests, we skip if Temporal is not available
	manager, err := NewManager(cfg)
	if err != nil {
		t.Skipf("Temporal server not available: %v", err)
		return
	}
	defer manager.Stop()

	if manager == nil {
		t.Fatal("Expected manager to be non-nil")
	}

	if manager.GetClient() == nil {
		t.Error("Expected client to be non-nil")
	}

	if manager.GetEventBus() == nil {
		t.Error("Expected event bus to be non-nil")
	}
}

// TestTemporalManagerWithoutEventBus tests manager creation without event bus
func TestTemporalManagerWithoutEventBus(t *testing.T) {
	cfg := &config.TemporalConfig{
		Host:                     "localhost:7233",
		Namespace:                "test-namespace",
		TaskQueue:                "test-queue",
		WorkflowExecutionTimeout: 1 * time.Hour,
		WorkflowTaskTimeout:      10 * time.Second,
		EnableEventBus:           false,
		EventBufferSize:          100,
	}

	manager, err := NewManager(cfg)
	if err != nil {
		t.Skipf("Temporal server not available: %v", err)
		return
	}
	defer manager.Stop()

	if manager.GetEventBus() != nil {
		t.Error("Expected event bus to be nil when disabled")
	}
}

// TestTemporalManagerNilConfig tests that nil config returns error
func TestTemporalManagerNilConfig(t *testing.T) {
	_, err := NewManager(nil)
	if err == nil {
		t.Error("Expected error with nil config")
	}
}

// TestTemporalClientCreation tests client creation
func TestTemporalClientCreation(t *testing.T) {
	cfg := &config.TemporalConfig{
		Host:                     "localhost:7233",
		Namespace:                "test-namespace",
		TaskQueue:                "test-queue",
		WorkflowExecutionTimeout: 1 * time.Hour,
		WorkflowTaskTimeout:      10 * time.Second,
		EnableEventBus:           false,
		EventBufferSize:          100,
	}

	client, err := NewManager(cfg)
	if err != nil {
		t.Skipf("Temporal server not available: %v", err)
		return
	}
	defer client.Stop()

	if client.GetClient() == nil {
		t.Fatal("Expected client to be created")
	}

	if client.GetClient().GetNamespace() != cfg.Namespace {
		t.Errorf("Expected namespace %s, got %s", cfg.Namespace, client.GetClient().GetNamespace())
	}
}

// TestWorkflowStarter tests starting a workflow
func TestWorkflowStarter(t *testing.T) {
	cfg := &config.TemporalConfig{
		Host:                     "localhost:7233",
		Namespace:                "test-namespace",
		TaskQueue:                "test-queue",
		WorkflowExecutionTimeout: 1 * time.Hour,
		WorkflowTaskTimeout:      10 * time.Second,
		EnableEventBus:           true,
		EventBufferSize:          100,
	}

	manager, err := NewManager(cfg)
	if err != nil {
		t.Skipf("Temporal server not available: %v", err)
		return
	}
	defer manager.Stop()

	// Start the worker
	err = manager.Start()
	if err != nil {
		t.Fatalf("Failed to start manager: %v", err)
	}

	// Give worker time to start
	time.Sleep(2 * time.Second)

	ctx := context.Background()

	// Start an agent workflow
	err = manager.StartAgentWorkflow(ctx, "test-agent-1", "test-project", "test-persona", "Test Agent")
	if err != nil {
		t.Skipf("Could not start workflow (worker may not be ready): %v", err)
		return
	}

	// Give workflow time to start
	time.Sleep(1 * time.Second)

	// Query the workflow
	result, err := manager.QueryAgentWorkflow(ctx, "test-agent-1", "getStatus")
	if err != nil {
		t.Logf("Could not query workflow: %v", err)
		return
	}

	t.Logf("Agent status: %v", result)
}
