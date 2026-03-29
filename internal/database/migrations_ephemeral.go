package database

// migrateEphemeralState adds tables for ephemeral state persistence:
// org chart snapshots, agent grades, status board entries, meeting summaries.
func (d *Database) migrateEphemeralState() error {
	schema := `
	CREATE TABLE IF NOT EXISTS ephemeral_org_chart_snapshots (
		id          TEXT PRIMARY KEY,
		timestamp   TIMESTAMP NOT NULL,
		structure   TEXT NOT NULL,
		report_lines TEXT NOT NULL,
		metadata    TEXT NOT NULL DEFAULT '{}',
		created_at  TIMESTAMP NOT NULL DEFAULT NOW()
	);

	CREATE TABLE IF NOT EXISTS ephemeral_agent_grades (
		id                    TEXT PRIMARY KEY,
		agent_id              TEXT NOT NULL,
		agent_role            TEXT NOT NULL,
		grade                 TEXT NOT NULL,
		bead_completion_rate  REAL NOT NULL DEFAULT 0,
		block_rate            REAL NOT NULL DEFAULT 0,
		iteration_efficiency  REAL NOT NULL DEFAULT 0,
		review_period         TEXT NOT NULL,
		reviewed_at           TIMESTAMP NOT NULL,
		feedback              TEXT NOT NULL DEFAULT '',
		metadata              TEXT NOT NULL DEFAULT '{}',
		created_at            TIMESTAMP NOT NULL DEFAULT NOW()
	);

	CREATE INDEX IF NOT EXISTS idx_agent_grades_agent_id ON ephemeral_agent_grades(agent_id);
	CREATE INDEX IF NOT EXISTS idx_agent_grades_reviewed_at ON ephemeral_agent_grades(reviewed_at DESC);

	CREATE TABLE IF NOT EXISTS ephemeral_status_board (
		id          TEXT PRIMARY KEY,
		author_id   TEXT NOT NULL,
		author_role TEXT NOT NULL,
		title       TEXT NOT NULL,
		content     TEXT NOT NULL,
		category    TEXT NOT NULL,
		priority    TEXT NOT NULL DEFAULT 'P3',
		posted_at   TIMESTAMP NOT NULL,
		updated_at  TIMESTAMP NOT NULL,
		metadata    TEXT NOT NULL DEFAULT '{}',
		created_at  TIMESTAMP NOT NULL DEFAULT NOW()
	);

	CREATE INDEX IF NOT EXISTS idx_status_board_category ON ephemeral_status_board(category);
	CREATE INDEX IF NOT EXISTS idx_status_board_posted_at ON ephemeral_status_board(posted_at DESC);

	CREATE TABLE IF NOT EXISTS ephemeral_meeting_summaries (
		id           TEXT PRIMARY KEY,
		meeting_id   TEXT NOT NULL,
		title        TEXT NOT NULL,
		attendees    TEXT NOT NULL DEFAULT '[]',
		start_time   TIMESTAMP NOT NULL,
		end_time     TIMESTAMP NOT NULL,
		agenda       TEXT NOT NULL DEFAULT '',
		summary      TEXT NOT NULL DEFAULT '',
		decisions    TEXT NOT NULL DEFAULT '[]',
		action_items TEXT NOT NULL DEFAULT '[]',
		next_meeting TIMESTAMP,
		recorded_at  TIMESTAMP NOT NULL,
		metadata     TEXT NOT NULL DEFAULT '{}',
		created_at   TIMESTAMP NOT NULL DEFAULT NOW()
	);

	CREATE INDEX IF NOT EXISTS idx_meeting_summaries_meeting_id ON ephemeral_meeting_summaries(meeting_id);
	CREATE INDEX IF NOT EXISTS idx_meeting_summaries_recorded_at ON ephemeral_meeting_summaries(recorded_at DESC);
	`

	if _, err := d.db.Exec(schema); err != nil {
		return err
	}
	return nil
}
