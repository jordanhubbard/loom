package ephemeralstate

import "github.com/jordanhubbard/loom/pkg/models"

// Store is the interface the Persistence layer uses for DB operations.
// *database.Database implements this interface.
type Store interface {
	SaveOrgChartSnapshot(snapshot *models.OrgChartSnapshot) error
	GetOrgChartSnapshot(snapshotID string) (*models.OrgChartSnapshot, error)

	SaveAgentGrade(grade *models.AgentGrade) error
	GetAgentGrades(agentID string) ([]*models.AgentGrade, error)
	GetLatestAgentGrade(agentID string) (*models.AgentGrade, error)

	SaveStatusBoardEntry(entry *models.StatusBoardEntry) error
	GetStatusBoardEntries(category string, limit int) ([]*models.StatusBoardEntry, error)

	SaveMeetingSummary(summary *models.MeetingSummary) error
	GetMeetingSummary(meetingID string) (*models.MeetingSummary, error)
	GetMeetingSummariesByAttendee(attendeeID string) ([]*models.MeetingSummary, error)
}
