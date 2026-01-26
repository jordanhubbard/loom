package database

import (
	"log"
)

// migrateMotivations adds the motivations and milestones tables
func (d *Database) migrateMotivations() error {
	// Motivations table
	motivationsSchema := `
	CREATE TABLE IF NOT EXISTS motivations (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		description TEXT,
		type TEXT NOT NULL,
		condition TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'active',
		agent_role TEXT,
		agent_id TEXT,
		project_id TEXT,
		parameters_json TEXT,
		cooldown_period_ns INTEGER NOT NULL DEFAULT 300000000000,
		last_triggered_at DATETIME,
		next_trigger_at DATETIME,
		trigger_count INTEGER NOT NULL DEFAULT 0,
		priority INTEGER NOT NULL DEFAULT 50,
		create_bead_on_trigger BOOLEAN NOT NULL DEFAULT 0,
		bead_template TEXT,
		wake_agent BOOLEAN NOT NULL DEFAULT 1,
		is_built_in BOOLEAN NOT NULL DEFAULT 0,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		disabled_at DATETIME,
		FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
		FOREIGN KEY (agent_id) REFERENCES agents(id) ON DELETE SET NULL
	);

	CREATE INDEX IF NOT EXISTS idx_motivations_type ON motivations(type);
	CREATE INDEX IF NOT EXISTS idx_motivations_status ON motivations(status);
	CREATE INDEX IF NOT EXISTS idx_motivations_agent_role ON motivations(agent_role);
	CREATE INDEX IF NOT EXISTS idx_motivations_project_id ON motivations(project_id);
	`

	if _, err := d.db.Exec(motivationsSchema); err != nil {
		return err
	}

	// Motivation triggers (history) table
	triggersSchema := `
	CREATE TABLE IF NOT EXISTS motivation_triggers (
		id TEXT PRIMARY KEY,
		motivation_id TEXT NOT NULL,
		triggered_at DATETIME NOT NULL,
		trigger_data_json TEXT,
		result TEXT NOT NULL,
		error TEXT,
		bead_created TEXT,
		agent_woken TEXT,
		workflow_id TEXT,
		FOREIGN KEY (motivation_id) REFERENCES motivations(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_motivation_triggers_motivation_id ON motivation_triggers(motivation_id);
	CREATE INDEX IF NOT EXISTS idx_motivation_triggers_triggered_at ON motivation_triggers(triggered_at);
	`

	if _, err := d.db.Exec(triggersSchema); err != nil {
		return err
	}

	// Milestones table
	milestonesSchema := `
	CREATE TABLE IF NOT EXISTS milestones (
		id TEXT PRIMARY KEY,
		project_id TEXT NOT NULL,
		name TEXT NOT NULL,
		description TEXT,
		type TEXT NOT NULL DEFAULT 'custom',
		status TEXT NOT NULL DEFAULT 'planned',
		due_date DATETIME NOT NULL,
		start_date DATETIME,
		completed_at DATETIME,
		parent_id TEXT,
		tags_json TEXT,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
		FOREIGN KEY (parent_id) REFERENCES milestones(id) ON DELETE SET NULL
	);

	CREATE INDEX IF NOT EXISTS idx_milestones_project_id ON milestones(project_id);
	CREATE INDEX IF NOT EXISTS idx_milestones_due_date ON milestones(due_date);
	CREATE INDEX IF NOT EXISTS idx_milestones_status ON milestones(status);
	`

	if _, err := d.db.Exec(milestonesSchema); err != nil {
		return err
	}

	// Add due_date column to projects if it doesn't exist
	_, _ = d.db.Exec("ALTER TABLE projects ADD COLUMN due_date DATETIME")

	// Add milestone tracking columns to beads (if a beads table exists)
	// Note: beads are typically managed by the bd CLI, but we add columns for completeness
	_, _ = d.db.Exec("ALTER TABLE beads ADD COLUMN due_date DATETIME")
	_, _ = d.db.Exec("ALTER TABLE beads ADD COLUMN milestone_id TEXT")
	_, _ = d.db.Exec("ALTER TABLE beads ADD COLUMN estimated_time INTEGER")

	log.Println("Motivation and milestone tables migrated successfully")
	return nil
}
