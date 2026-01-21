package analytics

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestDailyBudgetAlert(t *testing.T) {
	storage := NewInMemoryStorage()
	ctx := context.Background()

	// Add logs that exceed daily budget
	now := time.Now()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	for i := 0; i < 5; i++ {
		storage.SaveLog(ctx, &RequestLog{
			ID:        fmt.Sprintf("log-%d", i),
			Timestamp: startOfDay.Add(time.Duration(i) * time.Hour),
			UserID:    "user-test",
			CostUSD:   25.0, // Total: $125, exceeds $100 budget
		})
	}

	config := &AlertConfig{
		UserID:         "user-test",
		DailyBudgetUSD: 100.0,
	}

	checker := NewAlertChecker(storage, config)
	alerts, err := checker.CheckAlerts(ctx)

	if err != nil {
		t.Fatalf("CheckAlerts failed: %v", err)
	}

	if len(alerts) == 0 {
		t.Fatal("Expected daily budget alert, got none")
	}

	alert := alerts[0]
	if alert.Type != "budget_exceeded" {
		t.Errorf("Expected type 'budget_exceeded', got '%s'", alert.Type)
	}
	if alert.Severity != "warning" {
		t.Errorf("Expected severity 'warning', got '%s'", alert.Severity)
	}
	if alert.CurrentCost != 125.0 {
		t.Errorf("Expected current cost 125.0, got %.2f", alert.CurrentCost)
	}
}

func TestMonthlyBudgetAlert(t *testing.T) {
	storage := NewInMemoryStorage()
	ctx := context.Background()

	now := time.Now()
	startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

	// Add logs throughout the month that exceed budget
	for i := 0; i < 10; i++ {
		storage.SaveLog(ctx, &RequestLog{
			ID:        fmt.Sprintf("log-%d", i),
			Timestamp: startOfMonth.Add(time.Duration(i*24) * time.Hour),
			UserID:    "user-test",
			CostUSD:   250.0, // Total: $2500, exceeds $2000 budget
		})
	}

	config := &AlertConfig{
		UserID:           "user-test",
		MonthlyBudgetUSD: 2000.0,
	}

	checker := NewAlertChecker(storage, config)
	alerts, err := checker.CheckAlerts(ctx)

	if err != nil {
		t.Fatalf("CheckAlerts failed: %v", err)
	}

	if len(alerts) == 0 {
		t.Fatal("Expected monthly budget alert, got none")
	}

	alert := alerts[0]
	if alert.Type != "budget_exceeded" {
		t.Errorf("Expected type 'budget_exceeded', got '%s'", alert.Type)
	}
	if alert.Severity != "critical" {
		t.Errorf("Expected severity 'critical', got '%s'", alert.Severity)
	}
}

func TestAnomalyDetection(t *testing.T) {
	storage := NewInMemoryStorage()
	ctx := context.Background()

	now := time.Now()
	startOfToday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	// Add historical logs (last 7 days) with normal spending
	for i := 1; i <= 7; i++ {
		day := startOfToday.Add(-time.Duration(i*24) * time.Hour)
		storage.SaveLog(ctx, &RequestLog{
			ID:        fmt.Sprintf("log-hist-%d", i),
			Timestamp: day,
			UserID:    "user-test",
			CostUSD:   10.0, // $10/day average
		})
	}

	// Add today's logs with unusual spending
	for i := 0; i < 5; i++ {
		storage.SaveLog(ctx, &RequestLog{
			ID:        fmt.Sprintf("log-today-%d", i),
			Timestamp: startOfToday.Add(time.Duration(i) * time.Hour),
			UserID:    "user-test",
			CostUSD:   5.0, // Total: $25 today vs $10 average (2.5x)
		})
	}

	config := &AlertConfig{
		UserID:           "user-test",
		AnomalyThreshold: 2.0, // Alert if 2x normal
	}

	checker := NewAlertChecker(storage, config)
	alerts, err := checker.CheckAlerts(ctx)

	if err != nil {
		t.Fatalf("CheckAlerts failed: %v", err)
	}

	if len(alerts) == 0 {
		t.Fatal("Expected anomaly alert, got none")
	}

	alert := alerts[0]
	if alert.Type != "anomaly_detected" {
		t.Errorf("Expected type 'anomaly_detected', got '%s'", alert.Type)
	}
	if alert.CurrentCost != 25.0 {
		t.Errorf("Expected current cost 25.0, got %.2f", alert.CurrentCost)
	}
}

func TestNoAlertsWhenWithinBudget(t *testing.T) {
	storage := NewInMemoryStorage()
	ctx := context.Background()

	now := time.Now()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	// Add logs within budget
	storage.SaveLog(ctx, &RequestLog{
		ID:        "log-1",
		Timestamp: startOfDay,
		UserID:    "user-test",
		CostUSD:   50.0, // Under $100 budget
	})

	config := &AlertConfig{
		UserID:         "user-test",
		DailyBudgetUSD: 100.0,
	}

	checker := NewAlertChecker(storage, config)
	alerts, err := checker.CheckAlerts(ctx)

	if err != nil {
		t.Fatalf("CheckAlerts failed: %v", err)
	}

	if len(alerts) > 0 {
		t.Errorf("Expected no alerts, got %d", len(alerts))
	}
}
