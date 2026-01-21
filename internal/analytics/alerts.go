package analytics

import (
	"context"
	"fmt"
	"log"
	"time"
)

// AlertConfig defines alerting thresholds and settings
type AlertConfig struct {
	UserID              string  `json:"user_id"`
	DailyBudgetUSD      float64 `json:"daily_budget_usd"`      // Alert if daily spend exceeds
	MonthlyBudgetUSD    float64 `json:"monthly_budget_usd"`    // Alert if monthly spend exceeds
	AnomalyThreshold    float64 `json:"anomaly_threshold"`     // Alert if spend is X times normal (e.g., 2.0 = 2x)
	EnableEmailAlerts   bool    `json:"enable_email_alerts"`
	EnableWebhookAlerts bool    `json:"enable_webhook_alerts"`
	WebhookURL          string  `json:"webhook_url"`
	EmailAddress        string  `json:"email_address"`
}

// Alert represents a triggered alert
type Alert struct {
	ID          string    `json:"id"`
	UserID      string    `json:"user_id"`
	Type        string    `json:"type"`        // "budget_exceeded", "anomaly_detected"
	Severity    string    `json:"severity"`    // "info", "warning", "critical"
	Message     string    `json:"message"`
	CurrentCost float64   `json:"current_cost"`
	Threshold   float64   `json:"threshold"`
	TriggeredAt time.Time `json:"triggered_at"`
	Acknowledged bool     `json:"acknowledged"`
}

// AlertChecker monitors spending and triggers alerts
type AlertChecker struct {
	storage Storage
	config  *AlertConfig
}

// NewAlertChecker creates a new alert checker
func NewAlertChecker(storage Storage, config *AlertConfig) *AlertChecker {
	return &AlertChecker{
		storage: storage,
		config:  config,
	}
}

// CheckAlerts checks for spending anomalies and budget overruns
func (ac *AlertChecker) CheckAlerts(ctx context.Context) ([]*Alert, error) {
	alerts := make([]*Alert, 0)

	// Check daily budget
	if ac.config.DailyBudgetUSD > 0 {
		if alert := ac.checkDailyBudget(ctx); alert != nil {
			alerts = append(alerts, alert)
		}
	}

	// Check monthly budget
	if ac.config.MonthlyBudgetUSD > 0 {
		if alert := ac.checkMonthlyBudget(ctx); alert != nil {
			alerts = append(alerts, alert)
		}
	}

	// Check for anomalies
	if ac.config.AnomalyThreshold > 1.0 {
		if alert := ac.checkAnomalies(ctx); alert != nil {
			alerts = append(alerts, alert)
		}
	}

	// Notify for each alert
	for _, alert := range alerts {
		ac.notify(alert)
	}

	return alerts, nil
}

// checkDailyBudget checks if daily spending exceeds budget
func (ac *AlertChecker) checkDailyBudget(ctx context.Context) *Alert {
	now := time.Now()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	stats, err := ac.storage.GetLogStats(ctx, &LogFilter{
		UserID:    ac.config.UserID,
		StartTime: startOfDay,
		EndTime:   now,
	})
	if err != nil {
		return nil
	}

	if stats.TotalCostUSD > ac.config.DailyBudgetUSD {
		return &Alert{
			ID:          fmt.Sprintf("alert-daily-%d", time.Now().Unix()),
			UserID:      ac.config.UserID,
			Type:        "budget_exceeded",
			Severity:    "warning",
			Message:     fmt.Sprintf("Daily budget exceeded: $%.2f / $%.2f (%.0f%%)", stats.TotalCostUSD, ac.config.DailyBudgetUSD, (stats.TotalCostUSD/ac.config.DailyBudgetUSD)*100),
			CurrentCost: stats.TotalCostUSD,
			Threshold:   ac.config.DailyBudgetUSD,
			TriggeredAt: now,
		}
	}

	return nil
}

// checkMonthlyBudget checks if monthly spending exceeds budget
func (ac *AlertChecker) checkMonthlyBudget(ctx context.Context) *Alert {
	now := time.Now()
	startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

	stats, err := ac.storage.GetLogStats(ctx, &LogFilter{
		UserID:    ac.config.UserID,
		StartTime: startOfMonth,
		EndTime:   now,
	})
	if err != nil {
		return nil
	}

	if stats.TotalCostUSD > ac.config.MonthlyBudgetUSD {
		return &Alert{
			ID:          fmt.Sprintf("alert-monthly-%d", time.Now().Unix()),
			UserID:      ac.config.UserID,
			Type:        "budget_exceeded",
			Severity:    "critical",
			Message:     fmt.Sprintf("Monthly budget exceeded: $%.2f / $%.2f (%.0f%%)", stats.TotalCostUSD, ac.config.MonthlyBudgetUSD, (stats.TotalCostUSD/ac.config.MonthlyBudgetUSD)*100),
			CurrentCost: stats.TotalCostUSD,
			Threshold:   ac.config.MonthlyBudgetUSD,
			TriggeredAt: now,
		}
	}

	return nil
}

// checkAnomalies detects unusual spending patterns
func (ac *AlertChecker) checkAnomalies(ctx context.Context) *Alert {
	now := time.Now()

	// Get today's spending
	startOfToday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	todayStats, err := ac.storage.GetLogStats(ctx, &LogFilter{
		UserID:    ac.config.UserID,
		StartTime: startOfToday,
		EndTime:   now,
	})
	if err != nil {
		return nil
	}

	// Get average spending from last 7 days (excluding today)
	sevenDaysAgo := startOfToday.Add(-7 * 24 * time.Hour)
	historicalStats, err := ac.storage.GetLogStats(ctx, &LogFilter{
		UserID:    ac.config.UserID,
		StartTime: sevenDaysAgo,
		EndTime:   startOfToday,
	})
	if err != nil {
		return nil
	}

	// Calculate average daily spending
	avgDailySpend := historicalStats.TotalCostUSD / 7.0

	// Check if today's spending is anomalous
	if avgDailySpend > 0 && todayStats.TotalCostUSD > (avgDailySpend*ac.config.AnomalyThreshold) {
		return &Alert{
			ID:          fmt.Sprintf("alert-anomaly-%d", time.Now().Unix()),
			UserID:      ac.config.UserID,
			Type:        "anomaly_detected",
			Severity:    "warning",
			Message:     fmt.Sprintf("Unusual spending detected: $%.2f today vs $%.2f average (%.0fx increase)", todayStats.TotalCostUSD, avgDailySpend, todayStats.TotalCostUSD/avgDailySpend),
			CurrentCost: todayStats.TotalCostUSD,
			Threshold:   avgDailySpend * ac.config.AnomalyThreshold,
			TriggeredAt: now,
		}
	}

	return nil
}

// notify sends notifications for an alert
func (ac *AlertChecker) notify(alert *Alert) {
	// Log the alert
	log.Printf("[ALERT] %s: %s", alert.Severity, alert.Message)

	// TODO: Implement email notifications if enabled
	if ac.config.EnableEmailAlerts && ac.config.EmailAddress != "" {
		// Send email (requires email service integration)
		log.Printf("[ALERT] Email notification to %s: %s", ac.config.EmailAddress, alert.Message)
	}

	// TODO: Implement webhook notifications if enabled
	if ac.config.EnableWebhookAlerts && ac.config.WebhookURL != "" {
		// Send webhook (requires HTTP client)
		log.Printf("[ALERT] Webhook notification to %s: %s", ac.config.WebhookURL, alert.Message)
	}
}

// DefaultAlertConfig provides sensible defaults
func DefaultAlertConfig(userID string) *AlertConfig {
	return &AlertConfig{
		UserID:           userID,
		DailyBudgetUSD:   100.0,  // $100/day default
		MonthlyBudgetUSD: 2000.0, // $2000/month default
		AnomalyThreshold: 2.0,    // Alert if 2x normal spending
		EnableEmailAlerts:   false,
		EnableWebhookAlerts: false,
	}
}
