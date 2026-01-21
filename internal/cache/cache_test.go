package cache

import (
	"context"
	"testing"
	"time"
)

func TestCacheBasicOperations(t *testing.T) {
	c := New(DefaultConfig())
	ctx := context.Background()

	// Test Set and Get
	key := "test-key-1"
	response := map[string]interface{}{"result": "test response"}
	metadata := map[string]interface{}{
		"provider_id": "provider-1",
		"model_name":  "gpt-4",
		"total_tokens": int64(100),
	}

	err := c.Set(ctx, key, response, 1*time.Hour, metadata)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Get should return the cached entry
	entry, found := c.Get(ctx, key)
	if !found {
		t.Fatal("Expected cache hit, got miss")
	}

	if entry.ProviderID != "provider-1" {
		t.Errorf("Expected provider_id 'provider-1', got '%s'", entry.ProviderID)
	}

	if entry.ModelName != "gpt-4" {
		t.Errorf("Expected model_name 'gpt-4', got '%s'", entry.ModelName)
	}

	if entry.TokensSaved != 100 {
		t.Errorf("Expected 100 tokens saved, got %d", entry.TokensSaved)
	}
}

func TestCacheMiss(t *testing.T) {
	c := New(DefaultConfig())
	ctx := context.Background()

	_, found := c.Get(ctx, "non-existent-key")
	if found {
		t.Error("Expected cache miss, got hit")
	}

	stats := c.GetStats(ctx)
	if stats.Misses != 1 {
		t.Errorf("Expected 1 miss, got %d", stats.Misses)
	}
}

func TestCacheExpiration(t *testing.T) {
	c := New(DefaultConfig())
	ctx := context.Background()

	key := "test-expire"
	response := map[string]interface{}{"result": "expires soon"}
	
	// Set with 100ms TTL
	err := c.Set(ctx, key, response, 100*time.Millisecond, nil)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Should be cached immediately
	_, found := c.Get(ctx, key)
	if !found {
		t.Fatal("Expected cache hit before expiration")
	}

	// Wait for expiration
	time.Sleep(150 * time.Millisecond)

	// Should be expired now
	_, found = c.Get(ctx, key)
	if found {
		t.Fatal("Expected cache miss after expiration")
	}
}

func TestCacheHitCounter(t *testing.T) {
	c := New(DefaultConfig())
	ctx := context.Background()

	key := "test-hits"
	response := map[string]interface{}{"result": "popular response"}

	err := c.Set(ctx, key, response, 1*time.Hour, nil)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Get multiple times
	for i := 0; i < 5; i++ {
		entry, found := c.Get(ctx, key)
		if !found {
			t.Fatalf("Expected cache hit on iteration %d", i+1)
		}
		if entry.Hits != int64(i+1) {
			t.Errorf("Expected %d hits, got %d", i+1, entry.Hits)
		}
	}

	stats := c.GetStats(ctx)
	if stats.Hits != 5 {
		t.Errorf("Expected 5 hits in stats, got %d", stats.Hits)
	}
}

func TestCacheMaxSize(t *testing.T) {
	config := &Config{
		Enabled:    true,
		DefaultTTL: 1 * time.Hour,
		MaxSize:    3, // Small cache for testing
	}
	c := New(config)
	ctx := context.Background()

	// Add 4 entries (should evict oldest)
	for i := 0; i < 4; i++ {
		key := "key-" + string(rune('0'+i))
		response := map[string]interface{}{"value": i}
		err := c.Set(ctx, key, response, 1*time.Hour, nil)
		if err != nil {
			t.Fatalf("Set failed for key %s: %v", key, err)
		}
		time.Sleep(10 * time.Millisecond) // Ensure different timestamps
	}

	// First entry should be evicted
	_, found := c.Get(ctx, "key-0")
	if found {
		t.Error("Expected key-0 to be evicted")
	}

	// Other entries should exist
	for i := 1; i < 4; i++ {
		key := "key-" + string(rune('0'+i))
		_, found := c.Get(ctx, key)
		if !found {
			t.Errorf("Expected key %s to exist", key)
		}
	}

	stats := c.GetStats(ctx)
	if stats.Evictions != 1 {
		t.Errorf("Expected 1 eviction, got %d", stats.Evictions)
	}
}

func TestCacheDelete(t *testing.T) {
	c := New(DefaultConfig())
	ctx := context.Background()

	key := "test-delete"
	response := map[string]interface{}{"result": "to be deleted"}

	err := c.Set(ctx, key, response, 1*time.Hour, nil)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Verify it's cached
	_, found := c.Get(ctx, key)
	if !found {
		t.Fatal("Expected cache hit before delete")
	}

	// Delete it
	c.Delete(ctx, key)

	// Should not be found
	_, found = c.Get(ctx, key)
	if found {
		t.Fatal("Expected cache miss after delete")
	}
}

func TestCacheClear(t *testing.T) {
	c := New(DefaultConfig())
	ctx := context.Background()

	// Add multiple entries
	for i := 0; i < 5; i++ {
		key := "key-" + string(rune('0'+i))
		response := map[string]interface{}{"value": i}
		err := c.Set(ctx, key, response, 1*time.Hour, nil)
		if err != nil {
			t.Fatalf("Set failed: %v", err)
		}
	}

	stats := c.GetStats(ctx)
	if stats.TotalEntries != 5 {
		t.Errorf("Expected 5 entries, got %d", stats.TotalEntries)
	}

	// Clear cache
	c.Clear(ctx)

	stats = c.GetStats(ctx)
	if stats.TotalEntries != 0 {
		t.Errorf("Expected 0 entries after clear, got %d", stats.TotalEntries)
	}
}

func TestGenerateKey(t *testing.T) {
	request1 := map[string]interface{}{
		"messages": []map[string]string{
			{"role": "user", "content": "Hello, world!"},
		},
	}

	request2 := map[string]interface{}{
		"messages": []map[string]string{
			{"role": "user", "content": "Hello, world!"},
		},
	}

	request3 := map[string]interface{}{
		"messages": []map[string]string{
			{"role": "user", "content": "Different message"},
		},
	}

	// Same requests should generate same key
	key1, err := GenerateKey("provider-1", "gpt-4", request1)
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}

	key2, err := GenerateKey("provider-1", "gpt-4", request2)
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}

	if key1 != key2 {
		t.Error("Expected identical requests to generate same key")
	}

	// Different requests should generate different keys
	key3, err := GenerateKey("provider-1", "gpt-4", request3)
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}

	if key1 == key3 {
		t.Error("Expected different requests to generate different keys")
	}

	// Different provider should generate different key
	key4, err := GenerateKey("provider-2", "gpt-4", request1)
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}

	if key1 == key4 {
		t.Error("Expected different provider to generate different key")
	}

	// Different model should generate different key
	key5, err := GenerateKey("provider-1", "gpt-3.5", request1)
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}

	if key1 == key5 {
		t.Error("Expected different model to generate different key")
	}
}

func TestCacheDisabled(t *testing.T) {
	config := &Config{
		Enabled: false,
	}
	c := New(config)
	ctx := context.Background()

	key := "test-disabled"
	response := map[string]interface{}{"result": "should not be cached"}

	// Set should not error but not cache
	err := c.Set(ctx, key, response, 1*time.Hour, nil)
	if err != nil {
		t.Fatalf("Set should not error when disabled: %v", err)
	}

	// Get should return not found
	_, found := c.Get(ctx, key)
	if found {
		t.Error("Expected cache miss when disabled")
	}
}

func TestCacheHitRate(t *testing.T) {
	c := New(DefaultConfig())
	ctx := context.Background()

	key := "test-hitrate"
	response := map[string]interface{}{"result": "test"}

	err := c.Set(ctx, key, response, 1*time.Hour, nil)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// 3 hits, 2 misses
	c.Get(ctx, key)
	c.Get(ctx, key)
	c.Get(ctx, key)
	c.Get(ctx, "non-existent-1")
	c.Get(ctx, "non-existent-2")

	stats := c.GetStats(ctx)
	expectedHitRate := 3.0 / 5.0 // 60%

	if stats.HitRate < 0.59 || stats.HitRate > 0.61 {
		t.Errorf("Expected hit rate ~%.2f, got %.2f", expectedHitRate, stats.HitRate)
	}
}
