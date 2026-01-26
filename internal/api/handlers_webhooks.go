package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jordanhubbard/agenticorp/internal/motivation"
	"github.com/jordanhubbard/agenticorp/internal/temporal/eventbus"
)

// GitHubWebhookPayload represents a generic GitHub webhook payload
type GitHubWebhookPayload struct {
	Action     string            `json:"action"`
	Issue      *GitHubIssue      `json:"issue,omitempty"`
	PullRequest *GitHubPullRequest `json:"pull_request,omitempty"`
	Comment    *GitHubComment    `json:"comment,omitempty"`
	Repository *GitHubRepository `json:"repository,omitempty"`
	Sender     *GitHubUser       `json:"sender,omitempty"`
	Release    *GitHubRelease    `json:"release,omitempty"`
}

// GitHubIssue represents a GitHub issue
type GitHubIssue struct {
	ID        int64       `json:"id"`
	Number    int         `json:"number"`
	Title     string      `json:"title"`
	Body      string      `json:"body"`
	State     string      `json:"state"`
	URL       string      `json:"html_url"`
	User      *GitHubUser `json:"user,omitempty"`
	Labels    []GitHubLabel `json:"labels,omitempty"`
	CreatedAt string      `json:"created_at"`
	UpdatedAt string      `json:"updated_at"`
}

// GitHubPullRequest represents a GitHub pull request
type GitHubPullRequest struct {
	ID        int64       `json:"id"`
	Number    int         `json:"number"`
	Title     string      `json:"title"`
	Body      string      `json:"body"`
	State     string      `json:"state"`
	URL       string      `json:"html_url"`
	User      *GitHubUser `json:"user,omitempty"`
	Head      *GitHubRef  `json:"head,omitempty"`
	Base      *GitHubRef  `json:"base,omitempty"`
	Draft     bool        `json:"draft"`
	Merged    bool        `json:"merged"`
	CreatedAt string      `json:"created_at"`
	UpdatedAt string      `json:"updated_at"`
}

// GitHubComment represents a GitHub comment
type GitHubComment struct {
	ID        int64       `json:"id"`
	Body      string      `json:"body"`
	URL       string      `json:"html_url"`
	User      *GitHubUser `json:"user,omitempty"`
	CreatedAt string      `json:"created_at"`
	UpdatedAt string      `json:"updated_at"`
}

// GitHubRepository represents a GitHub repository
type GitHubRepository struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	FullName string `json:"full_name"`
	URL      string `json:"html_url"`
	Private  bool   `json:"private"`
}

// GitHubUser represents a GitHub user
type GitHubUser struct {
	ID    int64  `json:"id"`
	Login string `json:"login"`
	URL   string `json:"html_url"`
}

// GitHubLabel represents a GitHub label
type GitHubLabel struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

// GitHubRef represents a git reference
type GitHubRef struct {
	Ref  string          `json:"ref"`
	SHA  string          `json:"sha"`
	Repo *GitHubRepository `json:"repo,omitempty"`
}

// GitHubRelease represents a GitHub release
type GitHubRelease struct {
	ID         int64       `json:"id"`
	TagName    string      `json:"tag_name"`
	Name       string      `json:"name"`
	Body       string      `json:"body"`
	Draft      bool        `json:"draft"`
	Prerelease bool        `json:"prerelease"`
	URL        string      `json:"html_url"`
	Author     *GitHubUser `json:"author,omitempty"`
	CreatedAt  string      `json:"created_at"`
	PublishedAt string     `json:"published_at"`
}

// WebhookEvent represents a processed webhook event for the motivation system
type WebhookEvent struct {
	ID          string                 `json:"id"`
	Type        string                 `json:"type"` // github_issue_opened, github_pr_opened, etc.
	Source      string                 `json:"source"`
	Repository  string                 `json:"repository"`
	Action      string                 `json:"action"`
	Data        map[string]interface{} `json:"data"`
	ReceivedAt  time.Time              `json:"received_at"`
	Processed   bool                   `json:"processed"`
}

// handleGitHubWebhook handles incoming GitHub webhook events
// POST /api/v1/webhooks/github
func (s *Server) handleGitHubWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Read the body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "Failed to read request body")
		return
	}
	defer r.Body.Close()

	// Verify webhook signature if secret is configured
	if s.config != nil && s.config.Security.WebhookSecret != "" {
		signature := r.Header.Get("X-Hub-Signature-256")
		if !verifyGitHubSignature(body, signature, s.config.Security.WebhookSecret) {
			s.respondError(w, http.StatusUnauthorized, "Invalid webhook signature")
			return
		}
	}

	// Get the event type
	eventType := r.Header.Get("X-GitHub-Event")
	if eventType == "" {
		s.respondError(w, http.StatusBadRequest, "Missing X-GitHub-Event header")
		return
	}

	// Parse the payload
	var payload GitHubWebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		s.respondError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	// Process the event
	webhookEvent := s.processGitHubEvent(eventType, &payload)
	if webhookEvent == nil {
		// Event type not relevant to motivation system
		s.respondJSON(w, http.StatusOK, map[string]string{"status": "ignored"})
		return
	}

	// Publish event to event bus
	if s.agenticorp != nil {
		if eb := s.agenticorp.GetEventBus(); eb != nil {
			eventData := map[string]interface{}{
				"webhook_id":    webhookEvent.ID,
				"webhook_type":  webhookEvent.Type,
				"repository":    webhookEvent.Repository,
				"action":        webhookEvent.Action,
			}
			for k, v := range webhookEvent.Data {
				eventData[k] = v
			}

			// Map webhook type to event bus type
			var ebEventType eventbus.EventType
			switch webhookEvent.Type {
			case "github_issue_opened":
				ebEventType = eventbus.EventType("external.github_issue")
			case "github_pr_opened":
				ebEventType = eventbus.EventType("external.github_pr")
			case "github_comment_added":
				ebEventType = eventbus.EventType("external.github_comment")
			case "release_published":
				ebEventType = eventbus.EventType("external.release")
			default:
				ebEventType = eventbus.EventType("external.webhook")
			}

			_ = eb.Publish(&eventbus.Event{
				Type:   ebEventType,
				Source: "github-webhook",
				Data:   eventData,
			})
		}
	}

	// Store external event for motivation system to pick up
	if s.agenticorp != nil {
		s.storeExternalEvent(webhookEvent)
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"status": "received",
		"event":  webhookEvent,
	})
}

// processGitHubEvent converts a GitHub webhook into a motivation-relevant event
func (s *Server) processGitHubEvent(eventType string, payload *GitHubWebhookPayload) *WebhookEvent {
	event := &WebhookEvent{
		ID:         generateEventID(),
		Source:     "github",
		Action:     payload.Action,
		ReceivedAt: time.Now(),
		Data:       make(map[string]interface{}),
	}

	if payload.Repository != nil {
		event.Repository = payload.Repository.FullName
	}

	switch eventType {
	case "issues":
		if payload.Issue == nil {
			return nil
		}
		switch payload.Action {
		case "opened":
			event.Type = "github_issue_opened"
			event.Data["issue_number"] = payload.Issue.Number
			event.Data["issue_title"] = payload.Issue.Title
			event.Data["issue_url"] = payload.Issue.URL
			if payload.Issue.User != nil {
				event.Data["author"] = payload.Issue.User.Login
			}
			labels := make([]string, 0)
			for _, l := range payload.Issue.Labels {
				labels = append(labels, l.Name)
			}
			event.Data["labels"] = labels
		case "closed", "reopened", "edited":
			event.Type = "github_issue_" + payload.Action
			event.Data["issue_number"] = payload.Issue.Number
		default:
			return nil // Not relevant
		}

	case "pull_request":
		if payload.PullRequest == nil {
			return nil
		}
		switch payload.Action {
		case "opened":
			event.Type = "github_pr_opened"
			event.Data["pr_number"] = payload.PullRequest.Number
			event.Data["pr_title"] = payload.PullRequest.Title
			event.Data["pr_url"] = payload.PullRequest.URL
			event.Data["draft"] = payload.PullRequest.Draft
			if payload.PullRequest.User != nil {
				event.Data["author"] = payload.PullRequest.User.Login
			}
			if payload.PullRequest.Head != nil {
				event.Data["head_ref"] = payload.PullRequest.Head.Ref
			}
			if payload.PullRequest.Base != nil {
				event.Data["base_ref"] = payload.PullRequest.Base.Ref
			}
		case "closed":
			event.Type = "github_pr_closed"
			event.Data["pr_number"] = payload.PullRequest.Number
			event.Data["merged"] = payload.PullRequest.Merged
		case "ready_for_review":
			event.Type = "github_pr_ready"
			event.Data["pr_number"] = payload.PullRequest.Number
		default:
			return nil
		}

	case "issue_comment", "pull_request_review_comment":
		if payload.Comment == nil {
			return nil
		}
		if payload.Action != "created" {
			return nil
		}
		event.Type = "github_comment_added"
		event.Data["comment_id"] = payload.Comment.ID
		event.Data["comment_body"] = truncateString(payload.Comment.Body, 500)
		event.Data["comment_url"] = payload.Comment.URL
		if payload.Comment.User != nil {
			event.Data["author"] = payload.Comment.User.Login
		}
		if payload.Issue != nil {
			event.Data["issue_number"] = payload.Issue.Number
		}

	case "release":
		if payload.Release == nil {
			return nil
		}
		if payload.Action != "published" {
			return nil
		}
		event.Type = "release_published"
		event.Data["release_tag"] = payload.Release.TagName
		event.Data["release_name"] = payload.Release.Name
		event.Data["release_url"] = payload.Release.URL
		event.Data["prerelease"] = payload.Release.Prerelease
		if payload.Release.Author != nil {
			event.Data["author"] = payload.Release.Author.Login
		}

	default:
		return nil // Event type not relevant
	}

	return event
}

// storeExternalEvent stores the event for the motivation system to process
func (s *Server) storeExternalEvent(event *WebhookEvent) {
	// Convert to motivation.ExternalEvent
	extEvent := motivation.ExternalEvent{
		ID:        event.ID,
		Type:      event.Type,
		Source:    event.Source,
		Data:      event.Data,
		Timestamp: event.ReceivedAt,
		Processed: false,
	}

	// Store in database if available
	if s.agenticorp != nil && s.agenticorp.GetDatabase() != nil {
		db := s.agenticorp.GetDatabase()
		// Store as JSON in a key-value or dedicated table
		eventJSON, _ := json.Marshal(extEvent)
		_ = db.SetConfigValue("external_event:"+event.ID, string(eventJSON))
	}
}

// handleWebhookStatus handles webhook status checks
// GET /api/v1/webhooks/status
func (s *Server) handleWebhookStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	status := map[string]interface{}{
		"github_webhook_enabled": true,
		"webhook_secret_configured": s.config != nil && s.config.Security.WebhookSecret != "",
	}

	// Check if motivation engine is available
	if s.agenticorp != nil {
		status["motivation_engine_available"] = s.agenticorp.GetMotivationEngine() != nil
	}

	s.respondJSON(w, http.StatusOK, status)
}

// verifyGitHubSignature verifies the GitHub webhook signature
func verifyGitHubSignature(payload []byte, signature, secret string) bool {
	if signature == "" || secret == "" {
		return false
	}

	// Remove "sha256=" prefix
	signature = strings.TrimPrefix(signature, "sha256=")

	// Compute expected signature
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	expected := hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(signature), []byte(expected))
}

// generateEventID generates a unique event ID
func generateEventID() string {
	return time.Now().Format("20060102150405") + "-" + randomString(8)
}

// randomString generates a random string of given length
func randomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[time.Now().UnixNano()%int64(len(letters))]
	}
	return string(b)
}

// truncateString truncates a string to max length
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
