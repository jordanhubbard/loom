package models

import (
	"encoding/json"
	"time"
)

// SchemaVersion represents a semantic version for entity schemas
type SchemaVersion string

const (
	// Current schema versions for each entity type
	AgentSchemaVersion    SchemaVersion = "1.0"
	ProjectSchemaVersion  SchemaVersion = "1.0"
	ProviderSchemaVersion SchemaVersion = "1.0"
	OrgChartSchemaVersion SchemaVersion = "1.0"
	PositionSchemaVersion SchemaVersion = "1.0"
	PersonaSchemaVersion  SchemaVersion = "1.0"
	BeadSchemaVersion     SchemaVersion = "1.0"
)

// EntityType identifies the type of entity for migration purposes
type EntityType string

const (
	EntityTypeAgent    EntityType = "agent"
	EntityTypeProject  EntityType = "project"
	EntityTypeProvider EntityType = "provider"
	EntityTypeOrgChart EntityType = "orgchart"
	EntityTypePosition EntityType = "position"
	EntityTypePersona  EntityType = "persona"
	EntityTypeBead     EntityType = "bead"
)

// EntityMetadata contains versioning and extensibility fields for all entities
type EntityMetadata struct {
	// SchemaVersion tracks the schema version when this entity was created/updated
	SchemaVersion SchemaVersion `json:"schema_version,omitempty"`

	// Attributes provides extensible key-value storage for fields that don't
	// require schema changes. Use this for optional, additive fields.
	// Keys should be namespaced (e.g., "ui.color", "metrics.last_run")
	Attributes map[string]any `json:"attributes,omitempty"`

	// MigratedAt records when this entity was last migrated (if ever)
	MigratedAt *time.Time `json:"migrated_at,omitempty"`

	// MigratedFrom records the previous schema version before migration
	MigratedFrom SchemaVersion `json:"migrated_from,omitempty"`
}

// NewEntityMetadata creates metadata with the given schema version
func NewEntityMetadata(version SchemaVersion) EntityMetadata {
	return EntityMetadata{
		SchemaVersion: version,
		Attributes:    make(map[string]any),
	}
}

// GetAttribute retrieves a typed attribute value
func (e *EntityMetadata) GetAttribute(key string) (any, bool) {
	if e.Attributes == nil {
		return nil, false
	}
	val, ok := e.Attributes[key]
	return val, ok
}

// GetStringAttribute retrieves a string attribute with default
func (e *EntityMetadata) GetStringAttribute(key string, defaultVal string) string {
	if e.Attributes == nil {
		return defaultVal
	}
	if val, ok := e.Attributes[key]; ok {
		if s, ok := val.(string); ok {
			return s
		}
	}
	return defaultVal
}

// GetIntAttribute retrieves an int attribute with default
func (e *EntityMetadata) GetIntAttribute(key string, defaultVal int) int {
	if e.Attributes == nil {
		return defaultVal
	}
	if val, ok := e.Attributes[key]; ok {
		switch v := val.(type) {
		case int:
			return v
		case int64:
			return int(v)
		case float64:
			return int(v)
		case json.Number:
			if i, err := v.Int64(); err == nil {
				return int(i)
			}
		}
	}
	return defaultVal
}

// GetBoolAttribute retrieves a bool attribute with default
func (e *EntityMetadata) GetBoolAttribute(key string, defaultVal bool) bool {
	if e.Attributes == nil {
		return defaultVal
	}
	if val, ok := e.Attributes[key]; ok {
		if b, ok := val.(bool); ok {
			return b
		}
	}
	return defaultVal
}

// SetAttribute sets an attribute value
func (e *EntityMetadata) SetAttribute(key string, value any) {
	if e.Attributes == nil {
		e.Attributes = make(map[string]any)
	}
	e.Attributes[key] = value
}

// DeleteAttribute removes an attribute
func (e *EntityMetadata) DeleteAttribute(key string) {
	if e.Attributes != nil {
		delete(e.Attributes, key)
	}
}

// HasAttribute checks if an attribute exists
func (e *EntityMetadata) HasAttribute(key string) bool {
	if e.Attributes == nil {
		return false
	}
	_, ok := e.Attributes[key]
	return ok
}

// MergeAttributes merges new attributes into existing ones (new values win)
func (e *EntityMetadata) MergeAttributes(attrs map[string]any) {
	if e.Attributes == nil {
		e.Attributes = make(map[string]any)
	}
	for k, v := range attrs {
		e.Attributes[k] = v
	}
}

// AttributesJSON returns attributes as JSON bytes for database storage
func (e *EntityMetadata) AttributesJSON() ([]byte, error) {
	if len(e.Attributes) == 0 {
		return []byte("{}"), nil
	}
	return json.Marshal(e.Attributes)
}

// SetAttributesFromJSON parses JSON bytes into attributes
func (e *EntityMetadata) SetAttributesFromJSON(data []byte) error {
	if len(data) == 0 || string(data) == "{}" || string(data) == "null" {
		e.Attributes = make(map[string]any)
		return nil
	}
	return json.Unmarshal(data, &e.Attributes)
}

// VersionedEntity is the interface that all versioned entities must implement
type VersionedEntity interface {
	// GetEntityType returns the type of this entity
	GetEntityType() EntityType

	// GetSchemaVersion returns the current schema version
	GetSchemaVersion() SchemaVersion

	// SetSchemaVersion updates the schema version
	SetSchemaVersion(version SchemaVersion)

	// GetEntityMetadata returns the entity's metadata
	GetEntityMetadata() *EntityMetadata

	// GetID returns the entity's unique identifier
	GetID() string
}

// NeedsMigration checks if an entity needs migration to the target version
func NeedsMigration(entity VersionedEntity, targetVersion SchemaVersion) bool {
	currentVersion := entity.GetSchemaVersion()
	if currentVersion == "" {
		return true // Unversioned entities always need migration
	}
	return currentVersion != targetVersion
}

// Common attribute key constants for consistency
const (
	// UI-related attributes
	AttrUIColor       = "ui.color"
	AttrUIIcon        = "ui.icon"
	AttrUIDisplayName = "ui.display_name"
	AttrUIHidden      = "ui.hidden"

	// Metrics attributes
	AttrMetricsLastRun     = "metrics.last_run"
	AttrMetricsRunCount    = "metrics.run_count"
	AttrMetricsErrorCount  = "metrics.error_count"
	AttrMetricsAvgDuration = "metrics.avg_duration_ms"

	// Feature flags
	AttrFeatureEnabled  = "feature.enabled"
	AttrFeatureBeta     = "feature.beta"
	AttrFeatureInternal = "feature.internal"

	// Custom behavior
	AttrBehaviorPriority  = "behavior.priority"
	AttrBehaviorRetryable = "behavior.retryable"
	AttrBehaviorTimeout   = "behavior.timeout_ms"
)
