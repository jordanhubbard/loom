package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jordanhubbard/loom/pkg/models"
)

// SaveOrgChartSnapshot persists an org chart snapshot.
func (d *Database) SaveOrgChartSnapshot(snapshot *models.OrgChartSnapshot) error {
	if snapshot == nil {
		return fmt.Errorf("org chart snapshot is required")
	}
	if snapshot.ID == "" {
		return fmt.Errorf("snapshot ID is required")
	}

	structure, err := json.Marshal(snapshot.Structure)
	if err != nil {
		return fmt.Errorf("marshal structure: %w", err)
	}
	reportLines, err := json.Marshal(snapshot.ReportLines)
	if err != nil {
		return fmt.Errorf("marshal report_lines: %w", err)
	}
	metadata, err := json.Marshal(snapshot.Metadata)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}

	q := `
		INSERT INTO ephemeral_org_chart_snapshots
			(id, timestamp, structure, report_lines, metadata)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT(id) DO UPDATE SET
			timestamp    = EXCLUDED.timestamp,
			structure    = EXCLUDED.structure,
			report_lines = EXCLUDED.report_lines,
			metadata     = EXCLUDED.metadata
	`
	_, err = d.db.Exec(q,
		snapshot.ID, snapshot.Timestamp,
		string(structure), string(reportLines), string(metadata),
	)
	return err
}

// GetOrgChartSnapshot retrieves an org chart snapshot by ID.
func (d *Database) GetOrgChartSnapshot(snapshotID string) (*models.OrgChartSnapshot, error) {
	if snapshotID == "" {
		return nil, fmt.Errorf("snapshot ID is required")
	}

	q := `SELECT id, timestamp, structure, report_lines, metadata
	      FROM ephemeral_org_chart_snapshots WHERE id = $1`
	row := d.db.QueryRow(q, snapshotID)

	var s models.OrgChartSnapshot
	var structureJSON, reportLinesJSON, metadataJSON string
	if err := row.Scan(&s.ID, &s.Timestamp, &structureJSON, &reportLinesJSON, &metadataJSON); err != nil {
		return nil, fmt.Errorf("org chart snapshot not found: %w", err)
	}
	if err := json.Unmarshal([]byte(structureJSON), &s.Structure); err != nil {
		return nil, fmt.Errorf("unmarshal structure: %w", err)
	}
	if err := json.Unmarshal([]byte(reportLinesJSON), &s.ReportLines); err != nil {
		return nil, fmt.Errorf("unmarshal report_lines: %w", err)
	}
	if err := json.Unmarshal([]byte(metadataJSON), &s.Metadata); err != nil {
		return nil, fmt.Errorf("unmarshal metadata: %w", err)
	}
	return &s, nil
}

// SaveAgentGrade persists an agent grade.
func (d *Database) SaveAgentGrade(grade *models.AgentGrade) error {
	if grade == nil {
		return fmt.Errorf("agent grade is required")
	}
	if grade.AgentID == "" {
		return fmt.Errorf("agent ID is required")
	}
	if grade.Grade == "" {
		return fmt.Errorf("grade is required")
	}

	metadata, err := json.Marshal(grade.Metadata)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}

	q := `
		INSERT INTO ephemeral_agent_grades
			(id, agent_id, agent_role, grade, bead_completion_rate, block_rate,
			 iteration_efficiency, review_period, reviewed_at, feedback, metadata)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		ON CONFLICT(id) DO UPDATE SET
			agent_role           = EXCLUDED.agent_role,
			grade                = EXCLUDED.grade,
			bead_completion_rate = EXCLUDED.bead_completion_rate,
			block_rate           = EXCLUDED.block_rate,
			iteration_efficiency = EXCLUDED.iteration_efficiency,
			review_period        = EXCLUDED.review_period,
			reviewed_at          = EXCLUDED.reviewed_at,
			feedback             = EXCLUDED.feedback,
			metadata             = EXCLUDED.metadata
	`
	_, err = d.db.Exec(q,
		grade.ID, grade.AgentID, grade.AgentRole, grade.Grade,
		grade.BeadCompletionRate, grade.BlockRate, grade.IterationEfficiency,
		grade.ReviewPeriod, grade.ReviewedAt, grade.Feedback, string(metadata),
	)
	return err
}

// GetAgentGrades retrieves all grades for an agent, newest first.
func (d *Database) GetAgentGrades(agentID string) ([]*models.AgentGrade, error) {
	if agentID == "" {
		return nil, fmt.Errorf("agent ID is required")
	}

	q := `SELECT id, agent_id, agent_role, grade, bead_completion_rate, block_rate,
	             iteration_efficiency, review_period, reviewed_at, feedback, metadata
	      FROM ephemeral_agent_grades WHERE agent_id = $1
	      ORDER BY reviewed_at DESC`
	rows, err := d.db.Query(q, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var grades []*models.AgentGrade
	for rows.Next() {
		g := &models.AgentGrade{}
		var metadataJSON string
		if err := rows.Scan(
			&g.ID, &g.AgentID, &g.AgentRole, &g.Grade,
			&g.BeadCompletionRate, &g.BlockRate, &g.IterationEfficiency,
			&g.ReviewPeriod, &g.ReviewedAt, &g.Feedback, &metadataJSON,
		); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(metadataJSON), &g.Metadata); err != nil {
			return nil, fmt.Errorf("unmarshal metadata: %w", err)
		}
		grades = append(grades, g)
	}
	return grades, rows.Err()
}

// GetLatestAgentGrade retrieves the most recent grade for an agent.
func (d *Database) GetLatestAgentGrade(agentID string) (*models.AgentGrade, error) {
	if agentID == "" {
		return nil, fmt.Errorf("agent ID is required")
	}

	q := `SELECT id, agent_id, agent_role, grade, bead_completion_rate, block_rate,
	             iteration_efficiency, review_period, reviewed_at, feedback, metadata
	      FROM ephemeral_agent_grades WHERE agent_id = $1
	      ORDER BY reviewed_at DESC LIMIT 1`
	row := d.db.QueryRow(q, agentID)

	g := &models.AgentGrade{}
	var metadataJSON string
	if err := row.Scan(
		&g.ID, &g.AgentID, &g.AgentRole, &g.Grade,
		&g.BeadCompletionRate, &g.BlockRate, &g.IterationEfficiency,
		&g.ReviewPeriod, &g.ReviewedAt, &g.Feedback, &metadataJSON,
	); err != nil {
		return nil, fmt.Errorf("agent grade not found: %w", err)
	}
	if err := json.Unmarshal([]byte(metadataJSON), &g.Metadata); err != nil {
		return nil, fmt.Errorf("unmarshal metadata: %w", err)
	}
	return g, nil
}

// SaveStatusBoardEntry persists a status board entry.
func (d *Database) SaveStatusBoardEntry(entry *models.StatusBoardEntry) error {
	if entry == nil {
		return fmt.Errorf("status board entry is required")
	}
	if entry.AuthorID == "" {
		return fmt.Errorf("author ID is required")
	}
	if entry.Title == "" {
		return fmt.Errorf("title is required")
	}

	metadata, err := json.Marshal(entry.Metadata)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}
	if entry.PostedAt.IsZero() {
		entry.PostedAt = time.Now()
	}
	if entry.UpdatedAt.IsZero() {
		entry.UpdatedAt = time.Now()
	}

	q := `
		INSERT INTO ephemeral_status_board
			(id, author_id, author_role, title, content, category, priority, posted_at, updated_at, metadata)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		ON CONFLICT(id) DO UPDATE SET
			title      = EXCLUDED.title,
			content    = EXCLUDED.content,
			category   = EXCLUDED.category,
			priority   = EXCLUDED.priority,
			updated_at = EXCLUDED.updated_at,
			metadata   = EXCLUDED.metadata
	`
	_, err = d.db.Exec(q,
		entry.ID, entry.AuthorID, entry.AuthorRole, entry.Title,
		entry.Content, entry.Category, entry.Priority,
		entry.PostedAt, entry.UpdatedAt, string(metadata),
	)
	return err
}

// GetStatusBoardEntries retrieves status board entries, optionally filtered by category.
// Pass empty string for category to get all. limit=0 means no limit.
func (d *Database) GetStatusBoardEntries(category string, limit int) ([]*models.StatusBoardEntry, error) {
	var (
		rows *sql.Rows
		err  error
	)

	base := `SELECT id, author_id, author_role, title, content, category, priority,
	                posted_at, updated_at, metadata
	         FROM ephemeral_status_board`

	if category != "" && limit > 0 {
		rows, err = d.db.Query(base+` WHERE category = $1 ORDER BY posted_at DESC LIMIT $2`, category, limit)
	} else if category != "" {
		rows, err = d.db.Query(base+` WHERE category = $1 ORDER BY posted_at DESC`, category)
	} else if limit > 0 {
		rows, err = d.db.Query(base+` ORDER BY posted_at DESC LIMIT $1`, limit)
	} else {
		rows, err = d.db.Query(base + ` ORDER BY posted_at DESC`)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []*models.StatusBoardEntry
	for rows.Next() {
		e := &models.StatusBoardEntry{}
		var metadataJSON string
		if err := rows.Scan(
			&e.ID, &e.AuthorID, &e.AuthorRole, &e.Title, &e.Content,
			&e.Category, &e.Priority, &e.PostedAt, &e.UpdatedAt, &metadataJSON,
		); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(metadataJSON), &e.Metadata); err != nil {
			return nil, fmt.Errorf("unmarshal metadata: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// SaveMeetingSummary persists a meeting summary.
func (d *Database) SaveMeetingSummary(summary *models.MeetingSummary) error {
	if summary == nil {
		return fmt.Errorf("meeting summary is required")
	}
	if summary.MeetingID == "" {
		return fmt.Errorf("meeting ID is required")
	}
	if summary.Title == "" {
		return fmt.Errorf("title is required")
	}

	attendees, err := json.Marshal(summary.Attendees)
	if err != nil {
		return fmt.Errorf("marshal attendees: %w", err)
	}
	decisions, err := json.Marshal(summary.Decisions)
	if err != nil {
		return fmt.Errorf("marshal decisions: %w", err)
	}
	actionItems, err := json.Marshal(summary.ActionItems)
	if err != nil {
		return fmt.Errorf("marshal action_items: %w", err)
	}
	metadata, err := json.Marshal(summary.Metadata)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}

	q := `
		INSERT INTO ephemeral_meeting_summaries
			(id, meeting_id, title, attendees, start_time, end_time, agenda, summary,
			 decisions, action_items, next_meeting, recorded_at, metadata)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		ON CONFLICT(id) DO UPDATE SET
			title        = EXCLUDED.title,
			attendees    = EXCLUDED.attendees,
			start_time   = EXCLUDED.start_time,
			end_time     = EXCLUDED.end_time,
			agenda       = EXCLUDED.agenda,
			summary      = EXCLUDED.summary,
			decisions    = EXCLUDED.decisions,
			action_items = EXCLUDED.action_items,
			next_meeting = EXCLUDED.next_meeting,
			recorded_at  = EXCLUDED.recorded_at,
			metadata     = EXCLUDED.metadata
	`
	_, err = d.db.Exec(q,
		summary.ID, summary.MeetingID, summary.Title,
		string(attendees), summary.StartTime, summary.EndTime,
		summary.Agenda, summary.Summary,
		string(decisions), string(actionItems),
		summary.NextMeeting, summary.RecordedAt, string(metadata),
	)
	return err
}

// GetMeetingSummary retrieves a meeting summary by meeting ID.
func (d *Database) GetMeetingSummary(meetingID string) (*models.MeetingSummary, error) {
	if meetingID == "" {
		return nil, fmt.Errorf("meeting ID is required")
	}

	q := `SELECT id, meeting_id, title, attendees, start_time, end_time, agenda, summary,
	             decisions, action_items, next_meeting, recorded_at, metadata
	      FROM ephemeral_meeting_summaries WHERE meeting_id = $1
	      ORDER BY recorded_at DESC LIMIT 1`
	row := d.db.QueryRow(q, meetingID)
	return scanMeetingSummary(row)
}

// GetMeetingSummariesByAttendee retrieves all meeting summaries for an attendee.
func (d *Database) GetMeetingSummariesByAttendee(attendeeID string) ([]*models.MeetingSummary, error) {
	if attendeeID == "" {
		return nil, fmt.Errorf("attendee ID is required")
	}

	// Use JSON contains operator to search attendees array
	q := `SELECT id, meeting_id, title, attendees, start_time, end_time, agenda, summary,
	             decisions, action_items, next_meeting, recorded_at, metadata
	      FROM ephemeral_meeting_summaries
	      WHERE attendees::jsonb @> $1::jsonb
	      ORDER BY recorded_at DESC`

	attendeeJSON, err := json.Marshal([]string{attendeeID})
	if err != nil {
		return nil, err
	}

	rows, err := d.db.Query(q, string(attendeeJSON))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var summaries []*models.MeetingSummary
	for rows.Next() {
		s, err := scanMeetingSummaryRow(rows)
		if err != nil {
			return nil, err
		}
		summaries = append(summaries, s)
	}
	return summaries, rows.Err()
}

// scanMeetingSummary scans a single row into a MeetingSummary.
func scanMeetingSummary(row *sql.Row) (*models.MeetingSummary, error) {
	s := &models.MeetingSummary{}
	var attendeesJSON, decisionsJSON, actionItemsJSON, metadataJSON string
	if err := row.Scan(
		&s.ID, &s.MeetingID, &s.Title,
		&attendeesJSON, &s.StartTime, &s.EndTime,
		&s.Agenda, &s.Summary,
		&decisionsJSON, &actionItemsJSON,
		&s.NextMeeting, &s.RecordedAt, &metadataJSON,
	); err != nil {
		return nil, fmt.Errorf("meeting summary not found: %w", err)
	}
	return unmarshalMeetingSummary(s, attendeesJSON, decisionsJSON, actionItemsJSON, metadataJSON)
}

// scanMeetingSummaryRow scans a rows.Next() row into a MeetingSummary.
func scanMeetingSummaryRow(rows *sql.Rows) (*models.MeetingSummary, error) {
	s := &models.MeetingSummary{}
	var attendeesJSON, decisionsJSON, actionItemsJSON, metadataJSON string
	if err := rows.Scan(
		&s.ID, &s.MeetingID, &s.Title,
		&attendeesJSON, &s.StartTime, &s.EndTime,
		&s.Agenda, &s.Summary,
		&decisionsJSON, &actionItemsJSON,
		&s.NextMeeting, &s.RecordedAt, &metadataJSON,
	); err != nil {
		return nil, err
	}
	return unmarshalMeetingSummary(s, attendeesJSON, decisionsJSON, actionItemsJSON, metadataJSON)
}

func unmarshalMeetingSummary(s *models.MeetingSummary, attendeesJSON, decisionsJSON, actionItemsJSON, metadataJSON string) (*models.MeetingSummary, error) {
	if err := json.Unmarshal([]byte(attendeesJSON), &s.Attendees); err != nil {
		return nil, fmt.Errorf("unmarshal attendees: %w", err)
	}
	if err := json.Unmarshal([]byte(decisionsJSON), &s.Decisions); err != nil {
		return nil, fmt.Errorf("unmarshal decisions: %w", err)
	}
	if err := json.Unmarshal([]byte(actionItemsJSON), &s.ActionItems); err != nil {
		return nil, fmt.Errorf("unmarshal action_items: %w", err)
	}
	if err := json.Unmarshal([]byte(metadataJSON), &s.Metadata); err != nil {
		return nil, fmt.Errorf("unmarshal metadata: %w", err)
	}
	return s, nil
}
