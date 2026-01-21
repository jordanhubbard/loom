package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// Entry represents a cached response
type Entry struct {
	Key         string                 `json:"key"`
	Response    interface{}            `json:"response"`
	Metadata    map[string]interface{} `json:"metadata"`
	CachedAt    time.Time              `json:"cached_at"`
	ExpiresAt   time.Time              `json:"expires_at"`
	Hits        int64                  `json:"hits"`
	ProviderID  string                 `json:"provider_id"`
	ModelName   string                 `json:"model_name"`
	TokensSaved int64                  `json:"tokens_saved"` // Cumulative tokens saved from cache hits
}

// Config defines cache configuration
type Config struct {
	Enabled       bool          `json:"enabled"`
	DefaultTTL    time.Duration `json:"default_ttl"`     // Default time-to-live for cache entries
	MaxSize       int           `json:"max_size"`        // Maximum number of entries
	MaxMemoryMB   int           `json:"max_memory_mb"`   // Maximum memory usage (approximate)
	CleanupPeriod time.Duration `json:"cleanup_period"`  // How often to run cleanup
}

// DefaultConfig returns sensible defaults for caching
func DefaultConfig() *Config {
	return &Config{
		Enabled:       true,
		DefaultTTL:    1 * time.Hour,  // 1 hour default
		MaxSize:       10000,           // 10K entries
		MaxMemoryMB:   500,             // 500MB max
		CleanupPeriod: 5 * time.Minute, // Cleanup every 5 minutes
	}
}

// Cache provides intelligent response caching
type Cache struct {
	config  *Config
	entries map[string]*Entry
	mu      sync.RWMutex
	stats   *Stats
}

// Stats tracks cache performance
type Stats struct {
	Hits           int64   `json:"hits"`
	Misses         int64   `json:"misses"`
	Evictions      int64   `json:"evictions"`
	TotalEntries   int64   `json:"total_entries"`
	HitRate        float64 `json:"hit_rate"`
	TokensSaved    int64   `json:"tokens_saved"`
	CostSavedUSD   float64 `json:"cost_saved_usd"`
	AvgLatencySavedMs int64 `json:"avg_latency_saved_ms"`
}

// New creates a new cache instance
func New(config *Config) *Cache {
	if config == nil {
		config = DefaultConfig()
	}

	c := &Cache{
		config:  config,
		entries: make(map[string]*Entry),
		stats:   &Stats{},
	}

	// Start background cleanup goroutine
	if config.Enabled && config.CleanupPeriod > 0 {
		go c.cleanupLoop()
	}

	return c
}

// GenerateKey creates a cache key from request parameters
func GenerateKey(providerID, model string, request interface{}) (string, error) {
	// Serialize request to JSON for consistent hashing
	reqBytes, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create a compound key: provider + model + request hash
	hasher := sha256.New()
	hasher.Write([]byte(providerID))
	hasher.Write([]byte(":"))
	hasher.Write([]byte(model))
	hasher.Write([]byte(":"))
	hasher.Write(reqBytes)

	hash := hex.EncodeToString(hasher.Sum(nil))
	return hash, nil
}

// Get retrieves a cached response if available and not expired
func (c *Cache) Get(ctx context.Context, key string) (*Entry, bool) {
	if !c.config.Enabled {
		return nil, false
	}

	c.mu.RLock()
	entry, exists := c.entries[key]
	c.mu.RUnlock()

	if !exists {
		c.updateStats(false, 0, 0)
		return nil, false
	}

	// Check expiration
	if time.Now().After(entry.ExpiresAt) {
		// Expired - remove it
		c.mu.Lock()
		delete(c.entries, key)
		c.mu.Unlock()
		c.updateStats(false, 0, 0)
		return nil, false
	}

	// Cache hit - update hit counter
	c.mu.Lock()
	entry.Hits++
	c.mu.Unlock()

	c.updateStats(true, entry.TokensSaved, 0)
	return entry, true
}

// Set stores a response in the cache
func (c *Cache) Set(ctx context.Context, key string, response interface{}, ttl time.Duration, metadata map[string]interface{}) error {
	if !c.config.Enabled {
		return nil
	}

	// Use default TTL if not specified
	if ttl == 0 {
		ttl = c.config.DefaultTTL
	}

	entry := &Entry{
		Key:        key,
		Response:   response,
		Metadata:   metadata,
		CachedAt:   time.Now(),
		ExpiresAt:  time.Now().Add(ttl),
		Hits:       0,
		ProviderID: getStringFromMap(metadata, "provider_id"),
		ModelName:  getStringFromMap(metadata, "model_name"),
		TokensSaved: getInt64FromMap(metadata, "total_tokens"),
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if we need to evict entries
	if len(c.entries) >= c.config.MaxSize {
		c.evictOldest()
	}

	c.entries[key] = entry
	return nil
}

// Delete removes an entry from the cache
func (c *Cache) Delete(ctx context.Context, key string) {
	if !c.config.Enabled {
		return
	}

	c.mu.Lock()
	delete(c.entries, key)
	c.mu.Unlock()
}

// Clear removes all entries from the cache
func (c *Cache) Clear(ctx context.Context) {
	if !c.config.Enabled {
		return
	}

	c.mu.Lock()
	c.entries = make(map[string]*Entry)
	c.mu.Unlock()
}

// GetStats returns current cache statistics
func (c *Cache) GetStats(ctx context.Context) *Stats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	stats := *c.stats
	stats.TotalEntries = int64(len(c.entries))

	// Calculate hit rate
	total := stats.Hits + stats.Misses
	if total > 0 {
		stats.HitRate = float64(stats.Hits) / float64(total)
	}

	return &stats
}

// cleanupLoop periodically removes expired entries
func (c *Cache) cleanupLoop() {
	ticker := time.NewTicker(c.config.CleanupPeriod)
	defer ticker.Stop()

	for range ticker.C {
		c.cleanup()
	}
}

// cleanup removes expired entries
func (c *Cache) cleanup() {
	now := time.Now()

	c.mu.Lock()
	defer c.mu.Unlock()

	for key, entry := range c.entries {
		if now.After(entry.ExpiresAt) {
			delete(c.entries, key)
		}
	}
}

// evictOldest removes the oldest entry (LRU eviction)
func (c *Cache) evictOldest() {
	// Find the oldest entry by CachedAt timestamp
	var oldestKey string
	var oldestTime time.Time
	first := true

	for key, entry := range c.entries {
		if first || entry.CachedAt.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.CachedAt
			first = false
		}
	}

	if oldestKey != "" {
		delete(c.entries, oldestKey)
		c.stats.Evictions++
	}
}

// updateStats updates cache statistics
func (c *Cache) updateStats(hit bool, tokensSaved, costSavedUSD int64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if hit {
		c.stats.Hits++
		c.stats.TokensSaved += tokensSaved
	} else {
		c.stats.Misses++
	}
}

// Helper functions

func getStringFromMap(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	if val, ok := m[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

func getInt64FromMap(m map[string]interface{}, key string) int64 {
	if m == nil {
		return 0
	}
	if val, ok := m[key]; ok {
		switch v := val.(type) {
		case int64:
			return v
		case int:
			return int64(v)
		case float64:
			return int64(v)
		}
	}
	return 0
}
