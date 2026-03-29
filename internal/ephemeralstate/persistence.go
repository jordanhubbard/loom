// Package ephemeralstate handles in-process state that is persisted to the
// database for durability across restarts.
package ephemeralstate

import (
	"fmt"

	"github.com/jordanhubbard/loom/pkg/models"
)

// Persistence wires the ephemeral-state layer to a database Store.
type Persistence struct {
	db Store
}

// NewPersistence creates a new Persistence.
// Pass a *database.Database (which implements Store) as db.
func NewPersistence(db Store) *Persistence {
	return &Persistence{db: db}
}

// --- Org chart snapshots ---

// SaveOrgChartSnapshot persists an org chart snapshot.
func (p *Persistence) SaveOrgChartSnapshot(snapshot *models.OrgChartSnapshot) error {
	if snapshot == nil {
		return fmt.Errorf("org chart snapshot is required")
	}
	if snapshot.ID == "" {
		return fmt.Errorf("snapshot ID is required")
	}
	if p.db == nil {
		return fmt.Errorf("database not configured")
	}
	return p.db.SaveOrgChartSnapshot(snapshot)
}

// GetOrgChartSnapshot retrieves an org chart snapshot from the database.
func (p *Persistence) GetOrgChartSnapshot(snapshotID string) (*models.OrgChartSnapshot, error) {
	if snapshotID == "" {
		return nil, fmt.Errorf("snapshot ID is required")
	}
	if p.db == nil {
		return nil, fmt.Errorf("database not configured")
	}
	return p.db.GetOrgChartSnapshot(snapshotID)
}

// --- Agent grades ---

// SaveAgentGrade persists an agent grade.
func (p *Persistence) SaveAgentGrade(grade *models.AgentGrade) error {
	if grade == nil {
		return fmt.Errorf("agent grade is required")
	}
	if grade.AgentID == "" {
		return fmt.Errorf("agent ID is required")
	}
	if grade.Grade == "" {
		return fmt.Errorf("grade is required")
	}
	if p.db == nil {
		return fmt.Errorf("database not configured")
	}
	return p.db.SaveAgentGrade(grade)
}

// GetAgentGrades retrieves all grades for an agent, newest first.
func (p *Persistence) GetAgentGrades(agentID string) ([]*models.AgentGrade, error) {
	if agentID == "" {
		return nil, fmt.Errorf("agent ID is required")
	}
	if p.db == nil {
		return nil, fmt.Errorf("database not configured")
	}
	return p.db.GetAgentGrades(agentID)
}

// GetLatestAgentGrade retrieves the most recent grade for an agent.
func (p *Persistence) GetLatestAgentGrade(agentID string) (*models.AgentGrade, error) {
	if agentID == "" {
		return nil, fmt.Errorf("agent ID is required")
	}
	if p.db == nil {
		return nil, fmt.Errorf("database not configured")
	}
	return p.db.GetLatestAgentGrade(agentID)
}

// --- Status board ---

// SaveStatusBoardEntry persists a status board entry.
func (p *Persistence) SaveStatusBoardEntry(entry *models.StatusBoardEntry) error {
	if entry == nil {
		return fmt.Errorf("status board entry is required")
	}
	if entry.AuthorID == "" {
		return fmt.Errorf("author ID is required")
	}
	if entry.Title == "" {
		return fmt.Errorf("title is required")
	}
	if p.db == nil {
		return fmt.Errorf("database not configured")
	}
	return p.db.SaveStatusBoardEntry(entry)
}

// GetStatusBoardEntries retrieves status board entries, optionally filtered by category.
// Pass empty string for category to get all. limit=0 means no limit.
func (p *Persistence) GetStatusBoardEntries(category string, limit int) ([]*models.StatusBoardEntry, error) {
	if p.db == nil {
		return nil, fmt.Errorf("database not configured")
	}
	return p.db.GetStatusBoardEntries(category, limit)
}

// --- Meeting summaries ---

// SaveMeetingSummary persists a meeting summary.
func (p *Persistence) SaveMeetingSummary(summary *models.MeetingSummary) error {
	if summary == nil {
		return fmt.Errorf("meeting summary is required")
	}
	if summary.MeetingID == "" {
		return fmt.Errorf("meeting ID is required")
	}
	if summary.Title == "" {
		return fmt.Errorf("title is required")
	}
	if p.db == nil {
		return fmt.Errorf("database not configured")
	}
	return p.db.SaveMeetingSummary(summary)
}

// GetMeetingSummary retrieves a meeting summary by meeting ID.
func (p *Persistence) GetMeetingSummary(meetingID string) (*models.MeetingSummary, error) {
	if meetingID == "" {
		return nil, fmt.Errorf("meeting ID is required")
	}
	if p.db == nil {
		return nil, fmt.Errorf("database not configured")
	}
	return p.db.GetMeetingSummary(meetingID)
}

// GetMeetingSummariesByAttendee retrieves all meeting summaries for an attendee.
func (p *Persistence) GetMeetingSummariesByAttendee(attendeeID string) ([]*models.MeetingSummary, error) {
	if attendeeID == "" {
		return nil, fmt.Errorf("attendee ID is required")
	}
	if p.db == nil {
		return nil, fmt.Errorf("database not configured")
	}
	return p.db.GetMeetingSummariesByAttendee(attendeeID)
}
