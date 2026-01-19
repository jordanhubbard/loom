package models

import "time"

// Provider represents an AI engine running on-prem or in the cloud
// Providers may require credentials (keys) to communicate
type Provider struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Type        string    `json:"type"`     // openai, anthropic, local, etc.
	Endpoint    string    `json:"endpoint"` // URL or path to the provider
	Model       string    `json:"model"`    // Default model for this provider
	Description string    `json:"description"`
	RequiresKey bool      `json:"requires_key"` // Whether this provider needs API credentials
	KeyID       string    `json:"key_id"`       // Reference to encrypted key in key manager
	Status      string    `json:"status"`       // active, inactive, etc.
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
