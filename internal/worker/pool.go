package worker

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/jordanhubbard/arbiter/internal/provider"
	"github.com/jordanhubbard/arbiter/pkg/models"
)

// Pool manages a pool of workers
type Pool struct {
	workers  map[string]*Worker
	registry *provider.Registry
	mu       sync.RWMutex
	maxWorkers int
}

// NewPool creates a new worker pool
func NewPool(registry *provider.Registry, maxWorkers int) *Pool {
	return &Pool{
		workers:    make(map[string]*Worker),
		registry:   registry,
		maxWorkers: maxWorkers,
	}
}

// SpawnWorker creates and starts a new worker for an agent
func (p *Pool) SpawnWorker(agent *models.Agent, providerID string) (*Worker, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	// Check if max workers reached
	if len(p.workers) >= p.maxWorkers {
		return nil, fmt.Errorf("maximum number of workers (%d) reached", p.maxWorkers)
	}
	
	// Check if worker already exists for this agent
	if _, exists := p.workers[agent.ID]; exists {
		return nil, fmt.Errorf("worker already exists for agent %s", agent.ID)
	}
	
	// Get provider from registry
	registeredProvider, err := p.registry.Get(providerID)
	if err != nil {
		return nil, fmt.Errorf("failed to get provider: %w", err)
	}
	
	// Create worker
	workerID := fmt.Sprintf("worker-%s-%d", agent.ID, time.Now().Unix())
	worker := NewWorker(workerID, agent, registeredProvider)
	
	// Start worker
	if err := worker.Start(); err != nil {
		return nil, fmt.Errorf("failed to start worker: %w", err)
	}
	
	// Add to pool
	p.workers[agent.ID] = worker
	
	log.Printf("Spawned worker %s for agent %s", workerID, agent.Name)
	
	return worker, nil
}

// StopWorker stops and removes a worker
func (p *Pool) StopWorker(agentID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	worker, exists := p.workers[agentID]
	if !exists {
		return fmt.Errorf("worker not found for agent %s", agentID)
	}
	
	// Stop the worker
	worker.Stop()
	
	// Remove from pool
	delete(p.workers, agentID)
	
	log.Printf("Stopped worker for agent %s", agentID)
	
	return nil
}

// GetWorker retrieves a worker by agent ID
func (p *Pool) GetWorker(agentID string) (*Worker, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	
	worker, exists := p.workers[agentID]
	if !exists {
		return nil, fmt.Errorf("worker not found for agent %s", agentID)
	}
	
	return worker, nil
}

// ListWorkers returns all workers in the pool
func (p *Pool) ListWorkers() []*Worker {
	p.mu.RLock()
	defer p.mu.RUnlock()
	
	workers := make([]*Worker, 0, len(p.workers))
	for _, worker := range p.workers {
		workers = append(workers, worker)
	}
	
	return workers
}

// GetIdleWorkers returns all idle workers
func (p *Pool) GetIdleWorkers() []*Worker {
	p.mu.RLock()
	defer p.mu.RUnlock()
	
	workers := make([]*Worker, 0)
	for _, worker := range p.workers {
		if worker.GetStatus() == WorkerStatusIdle {
			workers = append(workers, worker)
		}
	}
	
	return workers
}

// ExecuteTask assigns a task to an available worker
func (p *Pool) ExecuteTask(ctx context.Context, task *Task, agentID string) (*TaskResult, error) {
	// Get the worker for the specified agent
	worker, err := p.GetWorker(agentID)
	if err != nil {
		return nil, err
	}
	
	// Execute the task
	return worker.ExecuteTask(ctx, task)
}

// GetPoolStats returns statistics about the pool
func (p *Pool) GetPoolStats() PoolStats {
	p.mu.RLock()
	defer p.mu.RUnlock()
	
	stats := PoolStats{
		TotalWorkers: len(p.workers),
		MaxWorkers:   p.maxWorkers,
	}
	
	for _, worker := range p.workers {
		switch worker.GetStatus() {
		case WorkerStatusIdle:
			stats.IdleWorkers++
		case WorkerStatusWorking:
			stats.WorkingWorkers++
		case WorkerStatusError:
			stats.ErrorWorkers++
		case WorkerStatusStopped:
			stats.StoppedWorkers++
		}
	}
	
	return stats
}

// StopAll stops all workers in the pool
func (p *Pool) StopAll() {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	for agentID, worker := range p.workers {
		worker.Stop()
		delete(p.workers, agentID)
	}
	
	log.Println("Stopped all workers in pool")
}

// PoolStats contains statistics about the worker pool
type PoolStats struct {
	TotalWorkers   int
	IdleWorkers    int
	WorkingWorkers int
	ErrorWorkers   int
	StoppedWorkers int
	MaxWorkers     int
}
