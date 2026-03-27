package selfoptimization

import (
	"fmt"
	"strings"

	"time"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/jordanhubbard/loom/internal/persona"
	"github.com/jordanhubbard/loom/pkg/models"
)

// Optimizer handles self-optimization for agents.
// When an agent receives a poor performance review, it can rewrite its own MOTIVATION.md or PERSONALITY.md.
type Optimizer struct {
	personaManager *persona.Manager
}

// NewOptimizer creates a new self-optimization engine
func NewOptimizer(personaManager *persona.Manager) *Optimizer {
	return &Optimizer{
		personaManager: personaManager,
	}
}

// OptimizationTrigger represents a trigger for self-optimization
type OptimizationTrigger struct {
	AgentID               string    // The agent being optimized
	AgentRole             string    // The agent's role (e.g., "coder", "qa-engineer")
	PerformanceGrade      string    // The grade received (A-F)
	ConsecutiveFailures   int       // Number of consecutive poor grades
	ReviewFeedback        string    // Feedback from the performance review
	SuggestedOptimization string    // What to optimize: "motivation" or "personality"
	TriggeredAt           time.Time // When the trigger fired
}

// ShouldOptimize determines if an agent should self-optimize based on performance
func (o *Optimizer) ShouldOptimize(grade string, consecutiveFailures int) bool {
	// First D/F grade: warning logged
	// Second consecutive D/F: agent rewrites own MOTIVATION.md or PERSONALITY.md
	// Third consecutive D/F: agent is "fired" and CEO is notified

	if strings.ToUpper(grade) == "D" || strings.ToUpper(grade) == "F" {
		return consecutiveFailures >= 2
	}
	return false
}

// CreateOptimizationBead creates a bead asking the agent to rewrite its persona file
func (o *Optimizer) CreateOptimizationBead(trigger *OptimizationTrigger) (*models.Bead, error) {
	if trigger == nil {
		return nil, fmt.Errorf("optimization trigger is required")
	}

	if trigger.AgentID == "" {
		return nil, fmt.Errorf("agent ID is required")
	}

	// Determine what to optimize
	optimizationType := trigger.SuggestedOptimization
	if optimizationType == "" {
		optimizationType = "motivation" // Default to motivation
	}

	// Build the bead description
	description := o.buildOptimizationDescription(trigger, optimizationType)

	bead := &models.Bead{
		Title:       fmt.Sprintf("Self-optimize: Rewrite %s.md", cases.Title(language.Und).String(optimizationType)),
		Description: description,
		Status:      "open",
		Priority:    models.BeadPriorityP1, // Self-optimization is high priority
		AssignedTo:  trigger.AgentID,
		Tags:        []string{"self-optimization", "performance-review", optimizationType},
	}

	return bead, nil
}

// buildOptimizationDescription builds the description for an optimization bead
func (o *Optimizer) buildOptimizationDescription(trigger *OptimizationTrigger, optimizationType string) string {
	var sb strings.Builder

	sb.WriteString("## Performance Review Trigger\n\n")
	sb.WriteString(fmt.Sprintf("Your recent performance review resulted in a **%s** grade.\n\n", trigger.PerformanceGrade))
	sb.WriteString(fmt.Sprintf("This is your **%d consecutive** poor performance review.\n\n", trigger.ConsecutiveFailures))

	if trigger.ReviewFeedback != "" {
		sb.WriteString(fmt.Sprintf("### Feedback\n\n%s\n\n", trigger.ReviewFeedback))
	}

	sb.WriteString("## Self-Optimization Task\n\n")
	sb.WriteString(fmt.Sprintf("You are being asked to rewrite your **%s.md** file.\n\n", cases.Title(language.Und).String(optimizationType)))

	switch strings.ToLower(optimizationType) {
	case "motivation":
		sb.WriteString("### What to Rewrite\n\n")
		sb.WriteString("Your MOTIVATION.md file defines what drives you:\n\n")
		sb.WriteString("- **Primary drive:** What motivates you most?\n")
		sb.WriteString("- **Success metrics:** How do you measure success?\n")
		sb.WriteString("- **Trade-off priorities:** What do you prioritize when goals conflict?\n")
		sb.WriteString("- **Frustrations:** What frustrates you? What blocks your effectiveness?\n\n")
		sb.WriteString("Consider: Are your motivations misaligned with the organization's needs? Are you optimizing for the wrong metrics?\n\n")

	case "personality":
		sb.WriteString("### What to Rewrite\n\n")
		sb.WriteString("Your PERSONALITY.md file defines how you communicate and work:\n\n")
		sb.WriteString("- **Communication style:** How do you express yourself?\n")
		sb.WriteString("- **Temperament:** How do you respond to pressure, disagreement, or failure?\n")
		sb.WriteString("- **Humor and tone:** What's your voice?\n")
		sb.WriteString("- **Working style:** How do you prefer to work?\n")
		sb.WriteString("- **Values expression:** What do you care about?\n\n")
		sb.WriteString("Consider: Is your communication style creating friction? Are you expressing your values in a way that resonates with the team?\n\n")
	}

	sb.WriteString("## Process\n\n")
	sb.WriteString("1. Read your current file in `personas/default/<your-role>/<FILE>.md`\n")
	sb.WriteString("2. Reflect on the feedback and your recent performance\n")
	sb.WriteString("3. Rewrite the file with improvements\n")
	sb.WriteString("4. Commit the changes with a clear message\n")
	sb.WriteString("5. Close this bead when done\n\n")

	sb.WriteString("## Success Criteria\n\n")
	sb.WriteString("- Your rewritten file is thoughtful and specific\n")
	sb.WriteString("- It addresses the feedback from your performance review\n")
	sb.WriteString("- It's committed to git with a clear message\n")
	sb.WriteString("- Your next performance review shows improvement\n\n")

	sb.WriteString("## Notes\n\n")
	sb.WriteString("Self-optimization is a sign of a healthy, learning organization. Your willingness to reflect and improve is valued.\n")

	return sb.String()
}

// ApplyOptimization applies the optimization by updating the persona file
func (o *Optimizer) ApplyOptimization(personaName, optimizationType, newContent string) error {
	if personaName == "" {
		return fmt.Errorf("persona name is required")
	}

	if optimizationType == "" {
		return fmt.Errorf("optimization type is required")
	}

	// Validate optimization type
	validTypes := map[string]bool{
		"motivation":  true,
		"personality": true,
	}
	if !validTypes[strings.ToLower(optimizationType)] {
		return fmt.Errorf("invalid optimization type: %s (must be 'motivation' or 'personality')", optimizationType)
	}

	// Map optimization type to filename
	filename := cases.Title(language.Und).String(strings.ToLower(optimizationType)) + ".md"

	// Save the new content
	return o.personaManager.SavePersonaFile(personaName, filename, newContent)
}

// OptimizationResult represents the result of a self-optimization
type OptimizationResult struct {
	AgentID             string    // The agent that optimized
	OptimizationType    string    // What was optimized (motivation or personality)
	PreviousContent     string    // The previous content
	NewContent          string    // The new content
	OptimizedAt         time.Time // When it was optimized
	NextReviewScheduled time.Time // When the next review is scheduled
}

// RecordOptimization records that an optimization occurred
func (o *Optimizer) RecordOptimization(result *OptimizationResult) error {
	if result == nil {
		return fmt.Errorf("optimization result is required")
	}

	if result.AgentID == "" {
		return fmt.Errorf("agent ID is required")
	}

	// In a real implementation, this would be persisted to a database or log file
	// For now, we just validate the result

	return nil
}
