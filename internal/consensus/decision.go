package consensus

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// VoteChoice represents a vote option
type VoteChoice string

const (
	VoteApprove VoteChoice = "approve"
	VoteReject  VoteChoice = "reject"
	VoteAbstain VoteChoice = "abstain"
)

// DecisionStatus represents the current status of a decision
type DecisionStatus string

const (
	StatusPending   DecisionStatus = "pending"   // Waiting for votes
	StatusApproved  DecisionStatus = "approved"  // Consensus reached - approved
	StatusRejected  DecisionStatus = "rejected"  // Consensus reached - rejected
	StatusTimeout   DecisionStatus = "timeout"   // Deadline passed without consensus
	StatusCancelled DecisionStatus = "cancelled" // Decision cancelled
)

// ConsensusDecision represents a decision requiring consensus from multiple agents
type ConsensusDecision struct {
	ID              string                 `json:"id"`
	Title           string                 `json:"title"`
	Description     string                 `json:"description"`
	Question        string                 `json:"question"`
	Context         map[string]interface{} `json:"context,omitempty"`
	CreatedBy       string                 `json:"created_by"`
	CreatedAt       time.Time              `json:"created_at"`
	Deadline        time.Time              `json:"deadline"`
	Status          DecisionStatus         `json:"status"`
	RequiredAgents  []string               `json:"required_agents"` // Agents who must vote
	QuorumThreshold float64                `json:"quorum_threshold"` // 0.0-1.0, default 0.67 (2/3)
	Votes           map[string]Vote        `json:"votes"`            // agentID -> Vote
	Result          *DecisionResult        `json:"result,omitempty"`
	mu              sync.RWMutex
}

// Vote represents a single agent's vote
type Vote struct {
	AgentID   string     `json:"agent_id"`
	Choice    VoteChoice `json:"choice"`
	Rationale string     `json:"rationale,omitempty"`
	Confidence float64   `json:"confidence"` // 0.0-1.0
	Timestamp time.Time  `json:"timestamp"`
}

// DecisionResult contains the final outcome
type DecisionResult struct {
	FinalStatus     DecisionStatus `json:"final_status"`
	ApproveCount    int            `json:"approve_count"`
	RejectCount     int            `json:"reject_count"`
	AbstainCount    int            `json:"abstain_count"`
	TotalVotes      int            `json:"total_votes"`
	QuorumMet       bool           `json:"quorum_met"`
	ApprovalRate    float64        `json:"approval_rate"` // approve / (approve + reject)
	ParticipationRate float64      `json:"participation_rate"` // voted / required
	ResolvedAt      time.Time      `json:"resolved_at"`
}

// DecisionManager manages consensus decisions
type DecisionManager struct {
	decisions map[string]*ConsensusDecision
	mu        sync.RWMutex
	timeoutCh chan string // Channel for timeout notifications
}

// NewDecisionManager creates a new decision manager
func NewDecisionManager() *DecisionManager {
	dm := &DecisionManager{
		decisions: make(map[string]*ConsensusDecision),
		timeoutCh: make(chan string, 100),
	}

	// Start timeout monitor
	go dm.monitorTimeouts()

	return dm
}

// CreateDecision creates a new consensus decision
func (dm *DecisionManager) CreateDecision(ctx context.Context, title, description, question, createdBy string, requiredAgents []string, deadline time.Time, quorumThreshold float64) (*ConsensusDecision, error) {
	if title == "" {
		return nil, fmt.Errorf("title is required")
	}

	if question == "" {
		return nil, fmt.Errorf("question is required")
	}

	if len(requiredAgents) == 0 {
		return nil, fmt.Errorf("at least one required agent must be specified")
	}

	if quorumThreshold <= 0 || quorumThreshold > 1 {
		quorumThreshold = 0.67 // Default 2/3 majority
	}

	if deadline.IsZero() {
		deadline = time.Now().Add(24 * time.Hour) // Default 24h
	}

	decision := &ConsensusDecision{
		ID:              fmt.Sprintf("decision-%d", time.Now().UnixNano()),
		Title:           title,
		Description:     description,
		Question:        question,
		CreatedBy:       createdBy,
		CreatedAt:       time.Now(),
		Deadline:        deadline,
		Status:          StatusPending,
		RequiredAgents:  requiredAgents,
		QuorumThreshold: quorumThreshold,
		Votes:           make(map[string]Vote),
	}

	dm.mu.Lock()
	dm.decisions[decision.ID] = decision
	dm.mu.Unlock()

	return decision, nil
}

// GetDecision retrieves a decision by ID
func (dm *DecisionManager) GetDecision(ctx context.Context, decisionID string) (*ConsensusDecision, error) {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	decision, exists := dm.decisions[decisionID]
	if !exists {
		return nil, fmt.Errorf("decision not found: %s", decisionID)
	}

	return decision, nil
}

// CastVote records a vote from an agent
func (dm *DecisionManager) CastVote(ctx context.Context, decisionID, agentID string, choice VoteChoice, rationale string, confidence float64) error {
	dm.mu.Lock()
	decision, exists := dm.decisions[decisionID]
	if !exists {
		dm.mu.Unlock()
		return fmt.Errorf("decision not found: %s", decisionID)
	}
	dm.mu.Unlock()

	decision.mu.Lock()
	defer decision.mu.Unlock()

	// Check if voting is still open
	if decision.Status != StatusPending {
		return fmt.Errorf("decision is %s, voting is closed", decision.Status)
	}

	// Check if deadline passed
	if time.Now().After(decision.Deadline) {
		decision.Status = StatusTimeout
		return fmt.Errorf("voting deadline has passed")
	}

	// Check if agent is in required list
	found := false
	for _, reqAgent := range decision.RequiredAgents {
		if reqAgent == agentID {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("agent %s is not in the required voters list", agentID)
	}

	// Validate choice
	if choice != VoteApprove && choice != VoteReject && choice != VoteAbstain {
		return fmt.Errorf("invalid vote choice: %s", choice)
	}

	// Clamp confidence to 0-1
	if confidence < 0 {
		confidence = 0
	}
	if confidence > 1 {
		confidence = 1
	}

	// Record vote
	decision.Votes[agentID] = Vote{
		AgentID:    agentID,
		Choice:     choice,
		Rationale:  rationale,
		Confidence: confidence,
		Timestamp:  time.Now(),
	}

	// Check if decision can be resolved
	dm.checkAndResolveDecision(decision)

	return nil
}

// checkAndResolveDecision checks if decision has reached consensus (must be called with decision lock held)
func (dm *DecisionManager) checkAndResolveDecision(decision *ConsensusDecision) {
	// Count votes
	approveCount := 0
	rejectCount := 0
	abstainCount := 0

	for _, vote := range decision.Votes {
		switch vote.Choice {
		case VoteApprove:
			approveCount++
		case VoteReject:
			rejectCount++
		case VoteAbstain:
			abstainCount++
		}
	}

	totalVotes := len(decision.Votes)
	requiredVotes := len(decision.RequiredAgents)
	participationRate := float64(totalVotes) / float64(requiredVotes)

	// Check if quorum is met
	quorumMet := participationRate >= decision.QuorumThreshold

	if !quorumMet {
		// Not enough votes yet
		return
	}

	// Calculate approval rate (excluding abstains)
	activeVotes := approveCount + rejectCount
	var approvalRate float64
	if activeVotes > 0 {
		approvalRate = float64(approveCount) / float64(activeVotes)
	}

	// Determine final status
	// Decision is approved if approval rate meets quorum threshold
	var finalStatus DecisionStatus
	if approvalRate >= decision.QuorumThreshold {
		finalStatus = StatusApproved
	} else {
		finalStatus = StatusRejected
	}

	// Set result
	decision.Result = &DecisionResult{
		FinalStatus:       finalStatus,
		ApproveCount:      approveCount,
		RejectCount:       rejectCount,
		AbstainCount:      abstainCount,
		TotalVotes:        totalVotes,
		QuorumMet:         quorumMet,
		ApprovalRate:      approvalRate,
		ParticipationRate: participationRate,
		ResolvedAt:        time.Now(),
	}

	decision.Status = finalStatus
}

// CheckTimeout checks if a decision has timed out and resolves it
func (dm *DecisionManager) CheckTimeout(ctx context.Context, decisionID string) error {
	dm.mu.Lock()
	decision, exists := dm.decisions[decisionID]
	if !exists {
		dm.mu.Unlock()
		return fmt.Errorf("decision not found: %s", decisionID)
	}
	dm.mu.Unlock()

	decision.mu.Lock()
	defer decision.mu.Unlock()

	if decision.Status != StatusPending {
		return nil // Already resolved
	}

	if time.Now().After(decision.Deadline) {
		// Deadline passed - resolve as timeout
		approveCount := 0
		rejectCount := 0
		abstainCount := 0

		for _, vote := range decision.Votes {
			switch vote.Choice {
			case VoteApprove:
				approveCount++
			case VoteReject:
				rejectCount++
			case VoteAbstain:
				abstainCount++
			}
		}

		totalVotes := len(decision.Votes)
		requiredVotes := len(decision.RequiredAgents)
		participationRate := float64(totalVotes) / float64(requiredVotes)

		activeVotes := approveCount + rejectCount
		var approvalRate float64
		if activeVotes > 0 {
			approvalRate = float64(approveCount) / float64(activeVotes)
		}

		decision.Result = &DecisionResult{
			FinalStatus:       StatusTimeout,
			ApproveCount:      approveCount,
			RejectCount:       rejectCount,
			AbstainCount:      abstainCount,
			TotalVotes:        totalVotes,
			QuorumMet:         false,
			ApprovalRate:      approvalRate,
			ParticipationRate: participationRate,
			ResolvedAt:        time.Now(),
		}

		decision.Status = StatusTimeout
	}

	return nil
}

// CancelDecision cancels a pending decision
func (dm *DecisionManager) CancelDecision(ctx context.Context, decisionID string) error {
	dm.mu.Lock()
	decision, exists := dm.decisions[decisionID]
	if !exists {
		dm.mu.Unlock()
		return fmt.Errorf("decision not found: %s", decisionID)
	}
	dm.mu.Unlock()

	decision.mu.Lock()
	defer decision.mu.Unlock()

	if decision.Status != StatusPending {
		return fmt.Errorf("decision is %s, cannot be cancelled", decision.Status)
	}

	decision.Status = StatusCancelled
	decision.Result = &DecisionResult{
		FinalStatus: StatusCancelled,
		ResolvedAt:  time.Now(),
	}

	return nil
}

// ListDecisions returns all decisions, optionally filtered by status
func (dm *DecisionManager) ListDecisions(ctx context.Context, status DecisionStatus) []*ConsensusDecision {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	decisions := []*ConsensusDecision{}
	for _, decision := range dm.decisions {
		decision.mu.RLock()
		if status == "" || decision.Status == status {
			decisions = append(decisions, decision)
		}
		decision.mu.RUnlock()
	}

	return decisions
}

// monitorTimeouts periodically checks for timed out decisions
func (dm *DecisionManager) monitorTimeouts() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		dm.mu.RLock()
		decisionIDs := []string{}
		for id := range dm.decisions {
			decisionIDs = append(decisionIDs, id)
		}
		dm.mu.RUnlock()

		for _, id := range decisionIDs {
			_ = dm.CheckTimeout(context.Background(), id)
		}
	}
}

// Close shuts down the decision manager
func (dm *DecisionManager) Close() {
	close(dm.timeoutCh)
}
