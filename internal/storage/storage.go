package storage

import (
	"fmt"
	"sync"
	"time"

	"github.com/jordanhubbard/arbiter/internal/models"
)

// Storage provides in-memory storage for the arbiter service
type Storage struct {
	works          map[string]*models.Work
	agents         map[string]*models.WorkAgent
	communications []models.AgentCommunication
	services       map[string]*models.ServiceEndpoint
	traffic        map[string][]models.Traffic
	mu             sync.RWMutex
}

// New creates a new Storage instance
func New() *Storage {
	s := &Storage{
		works:          make(map[string]*models.Work),
		agents:         make(map[string]*models.WorkAgent),
		communications: make([]models.AgentCommunication, 0),
		services:       make(map[string]*models.ServiceEndpoint),
		traffic:        make(map[string][]models.Traffic),
	}
	
	// Initialize with some example services
	s.initializeDefaultServices()
	
	return s
}

func (s *Storage) initializeDefaultServices() {
	// Example self-hosted services (fixed cost)
	s.services["ollama-local"] = &models.ServiceEndpoint{
		ID:           "ollama-local",
		Name:         "Local Ollama",
		URL:          "http://localhost:11434",
		Type:         "ollama",
		IsActive:     false,
		CostType:     models.CostTypeFixed,
		FixedCost:    0, // Free self-hosted
		TokensUsed:   0,
		TotalCost:    0,
		RequestCount: 0,
		CreatedAt:    time.Now(),
	}
	
	s.services["vllm-local"] = &models.ServiceEndpoint{
		ID:           "vllm-local",
		Name:         "Local vLLM",
		URL:          "http://localhost:8000",
		Type:         "vllm",
		IsActive:     false,
		CostType:     models.CostTypeFixed,
		FixedCost:    0, // Free self-hosted
		TokensUsed:   0,
		TotalCost:    0,
		RequestCount: 0,
		CreatedAt:    time.Now(),
	}
	
	// Example cloud services (variable cost)
	s.services["openai-gpt4"] = &models.ServiceEndpoint{
		ID:           "openai-gpt4",
		Name:         "OpenAI GPT-4",
		URL:          "https://api.openai.com/v1",
		Type:         "openai",
		IsActive:     false,
		CostType:     models.CostTypeVariable,
		CostPerToken: 0.00003, // $0.03 per 1K tokens (example)
		TokensUsed:   0,
		TotalCost:    0,
		RequestCount: 0,
		CreatedAt:    time.Now(),
	}
}

// Work operations

func (s *Storage) CreateWork(work *models.Work) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if _, exists := s.works[work.ID]; exists {
		return fmt.Errorf("work with ID %s already exists", work.ID)
	}
	
	s.works[work.ID] = work
	return nil
}

func (s *Storage) GetWork(id string) (*models.Work, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	work, exists := s.works[id]
	if !exists {
		return nil, fmt.Errorf("work with ID %s not found", id)
	}
	
	return work, nil
}

func (s *Storage) ListWorks() []*models.Work {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	works := make([]*models.Work, 0, len(s.works))
	for _, work := range s.works {
		works = append(works, work)
	}
	
	return works
}

func (s *Storage) ListInProgressWorks() []*models.Work {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	works := make([]*models.Work, 0)
	for _, work := range s.works {
		if work.Status == models.WorkStatusInProgress || work.Status == models.WorkStatusPending {
			works = append(works, work)
		}
	}
	
	return works
}

func (s *Storage) UpdateWork(work *models.Work) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if _, exists := s.works[work.ID]; !exists {
		return fmt.Errorf("work with ID %s not found", work.ID)
	}
	
	work.UpdatedAt = time.Now()
	s.works[work.ID] = work
	return nil
}

// Agent operations

func (s *Storage) CreateAgent(agent *models.WorkAgent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if _, exists := s.agents[agent.ID]; exists {
		return fmt.Errorf("agent with ID %s already exists", agent.ID)
	}
	
	s.agents[agent.ID] = agent
	return nil
}

func (s *Storage) GetAgent(id string) (*models.WorkAgent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	agent, exists := s.agents[id]
	if !exists {
		return nil, fmt.Errorf("agent with ID %s not found", id)
	}
	
	return agent, nil
}

func (s *Storage) ListAgents() []*models.WorkAgent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	agents := make([]*models.WorkAgent, 0, len(s.agents))
	for _, agent := range s.agents {
		agents = append(agents, agent)
	}
	
	return agents
}

// Communication operations

func (s *Storage) AddCommunication(comm models.AgentCommunication) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	s.communications = append(s.communications, comm)
}

func (s *Storage) ListCommunications() []models.AgentCommunication {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	// Return a copy to avoid race conditions
	comms := make([]models.AgentCommunication, len(s.communications))
	copy(comms, s.communications)
	
	return comms
}

func (s *Storage) GetRecentCommunications(limit int) []models.AgentCommunication {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	total := len(s.communications)
	start := 0
	if total > limit {
		start = total - limit
	}
	
	comms := make([]models.AgentCommunication, total-start)
	copy(comms, s.communications[start:])
	
	return comms
}

// Service operations

func (s *Storage) GetService(id string) (*models.ServiceEndpoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	service, exists := s.services[id]
	if !exists {
		return nil, fmt.Errorf("service with ID %s not found", id)
	}
	
	return service, nil
}

func (s *Storage) ListServices() []*models.ServiceEndpoint {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	services := make([]*models.ServiceEndpoint, 0, len(s.services))
	for _, service := range s.services {
		services = append(services, service)
	}
	
	return services
}

func (s *Storage) ListActiveServices() []*models.ServiceEndpoint {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	services := make([]*models.ServiceEndpoint, 0)
	for _, service := range s.services {
		if service.IsActive {
			services = append(services, service)
		}
	}
	
	return services
}

func (s *Storage) UpdateService(service *models.ServiceEndpoint) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if _, exists := s.services[service.ID]; !exists {
		return fmt.Errorf("service with ID %s not found", service.ID)
	}
	
	s.services[service.ID] = service
	return nil
}

func (s *Storage) UpdateServiceCosts(id string, costType models.CostType, costPerToken, fixedCost *float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	service, exists := s.services[id]
	if !exists {
		return fmt.Errorf("service with ID %s not found", id)
	}
	
	service.CostType = costType
	if costPerToken != nil {
		service.CostPerToken = *costPerToken
	}
	if fixedCost != nil {
		service.FixedCost = *fixedCost
	}
	
	return nil
}

func (s *Storage) RecordServiceUsage(id string, tokensUsed int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	service, exists := s.services[id]
	if !exists {
		return fmt.Errorf("service with ID %s not found", id)
	}
	
	service.TokensUsed += tokensUsed
	service.RequestCount++
	service.IsActive = true
	service.LastActive = time.Now()
	
	// Update total cost for variable cost services
	if service.CostType == models.CostTypeVariable {
		service.TotalCost = float64(service.TokensUsed) * service.CostPerToken
	}
	
	return nil
}

// GetPreferredServices returns services sorted by preference (fixed-cost first)
func (s *Storage) GetPreferredServices() []*models.ServiceEndpoint {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	fixedCost := make([]*models.ServiceEndpoint, 0)
	variableCost := make([]*models.ServiceEndpoint, 0)
	
	for _, service := range s.services {
		if service.CostType == models.CostTypeFixed {
			fixedCost = append(fixedCost, service)
		} else {
			variableCost = append(variableCost, service)
		}
	}
	
	// Return fixed-cost services first (prioritized)
	return append(fixedCost, variableCost...)
}
