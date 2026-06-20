// Package config loads NexusRouter configuration from defaults, an optional YAML
// file, and environment variables (in increasing order of precedence).
//
// Env vars are prefixed NEXUSROUTER_ and use double underscores for nesting, e.g.
// NEXUSROUTER_SERVER__PORT=8080 sets server.port.
package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/structs"
	"github.com/knadh/koanf/v2"
)

// Config is the fully-resolved application configuration.
type Config struct {
	Server     ServerConfig     `koanf:"server"`
	Database   DatabaseConfig   `koanf:"database"`
	Security   SecurityConfig   `koanf:"security"`
	Cache      CacheConfig      `koanf:"cache"`
	Meter      MeterConfig      `koanf:"meter"`
	Limits     LimitsConfig     `koanf:"limits"`
	Health     HealthConfig     `koanf:"health"`
	Log        LogConfig        `koanf:"log"`
	Data       DataConfig       `koanf:"data"`
	Guardrails GuardrailsConfig `koanf:"guardrails"`
}

// ServerConfig controls the HTTP listener.
type ServerConfig struct {
	Host string `koanf:"host"`
	Port int    `koanf:"port"`
	// StreamStallTimeout aborts a stream that produces no bytes for this long.
	StreamStallTimeout time.Duration `koanf:"stream_stall_timeout"`
	// RequestTimeout bounds non-streaming upstream calls.
	RequestTimeout time.Duration `koanf:"request_timeout"`
	// CORSOrigins lists allowed dashboard origins ("*" permitted for local).
	CORSOrigins []string `koanf:"cors_origins"`
	// MaxConcurrentRequests caps in-flight API requests to prevent resource
	// exhaustion. Zero or negative means unlimited (not recommended for
	// high-traffic deployments). Default: 100.
	MaxConcurrentRequests int `koanf:"max_concurrent_requests"`
}

// DatabaseConfig selects and configures the persistence backend.
type DatabaseConfig struct {
	// Driver is "sqlite" (default, zero-config) or "postgres".
	Driver string `koanf:"driver"`
	// DSN is the connection string. Empty with sqlite => <data_dir>/nexusrouter.db.
	DSN string `koanf:"dsn"`
	// MaxOpenConns/MaxIdleConns tune the pool (postgres-relevant).
	MaxOpenConns int `koanf:"max_open_conns"`
	MaxIdleConns int `koanf:"max_idle_conns"`
}

// SecurityConfig holds auth and encryption settings.
type SecurityConfig struct {
	// MasterKey is the base64 root key for envelope encryption of credentials.
	// If empty, NexusRouter generates one on first run and persists it to
	// <data_dir>/master.key (0600). Provide via env/KMS in production.
	MasterKey string `koanf:"master_key"`
	// JWTSecret signs dashboard session tokens. Auto-generated if empty.
	JWTSecret string `koanf:"jwt_secret"`
	// DashboardPasswordHash is the argon2id hash of the dashboard password.
	DashboardPasswordHash string `koanf:"dashboard_password_hash"`
	// SessionTTL is how long a dashboard session stays valid.
	SessionTTL time.Duration `koanf:"session_ttl"`
	// BindLoopbackOnly rejects non-loopback dashboard/API access when true.
	BindLoopbackOnly bool `koanf:"bind_loopback_only"`
	// AllowPrivateBaseURL relaxes the SSRF guard on provider account base URLs
	// (and other ValidateOutboundURL callers) to permit loopback and RFC1918
	// addresses. Cloud metadata, link-local, unspecified, multicast, and
	// non-http(s) schemes remain blocked. Intended for self-hosted/LAN setups
	// pointing at on-network LLM endpoints. Default false.
	AllowPrivateBaseURL bool `koanf:"allow_private_base_url"`
}

// CacheConfig configures the semantic response cache.
type CacheConfig struct {
	Enabled bool `koanf:"enabled"`
	// Backend is "memory" (in-process) or "redis".
	Backend string `koanf:"backend"`
	// RedisURL is used when Backend == "redis". Format: redis://host:port[/db]
	RedisURL string `koanf:"redis_url"`
	// SimilarityThreshold in [0,1]; a candidate hit must score >= this.
	SimilarityThreshold float64 `koanf:"similarity_threshold"`
	// TTL bounds cached entry lifetime.
	TTL time.Duration `koanf:"ttl"`

	// EmbeddingProvider selects the embedding backend: "hash" (exact-match,
	// zero-dependency default) or "api" (OpenAI-compatible embeddings API for
	// true semantic near-match caching).
	EmbeddingProvider string `koanf:"embedding_provider"`
	// EmbeddingAPIURL is the base URL for the embedding API (e.g.
	// "https://api.openai.com/v1"). Only used when EmbeddingProvider == "api".
	EmbeddingAPIURL string `koanf:"embedding_api_url"`
	// EmbeddingAPIKey is the bearer token for the embedding API.
	EmbeddingAPIKey string `koanf:"embedding_api_key"`
	// EmbeddingModel is the model name for the embedding API (default:
	// "text-embedding-3-small").
	EmbeddingModel string `koanf:"embedding_model"`
	// EmbeddingDims is the output vector dimensions (default: 1536).
	EmbeddingDims int `koanf:"embedding_dims"`
}

// MeterConfig controls usage metering writes.
type MeterConfig struct {
	// Async enables buffered/batched DB writes for usage rows.
	Async bool `koanf:"async"`
	// BatchSize is the maximum records per flush.
	BatchSize int `koanf:"batch_size"`
	// FlushInterval bounds how long records wait in memory.
	FlushInterval time.Duration `koanf:"flush_interval"`
	// QueueSize is the buffered event capacity.
	QueueSize int `koanf:"queue_size"`
	// FullQueuePolicy is "sync" (fallback direct write) or "drop".
	FullQueuePolicy string `koanf:"full_queue_policy"`
	// ShutdownFlushTimeout bounds final drain on shutdown.
	ShutdownFlushTimeout time.Duration `koanf:"shutdown_flush_timeout"`
}

// LimitsConfig controls per-key request rate limiting.
type LimitsConfig struct {
	// Enabled turns request limiting on. When false, limiter is bypassed.
	Enabled bool `koanf:"enabled"`
	// Backend is "memory" for MVP. "redis" is reserved for distributed limits.
	Backend string `koanf:"backend"`
	// DefaultRPM/TPM/Concurrency apply to keys with no assigned plan.
	// Zero means unlimited.
	DefaultRPM         int64 `koanf:"default_rpm"`
	DefaultTPM         int64 `koanf:"default_tpm"`
	DefaultConcurrency int64 `koanf:"default_concurrency"`
	// Window is the fixed counting window for RPM/TPM.
	Window time.Duration `koanf:"window"`
	// CleanupInterval controls stale bucket cleanup for memory backend.
	CleanupInterval time.Duration `koanf:"cleanup_interval"`
	// RedisURL is reserved for the future redis backend.
	RedisURL string `koanf:"redis_url"`
}

// HealthConfig controls background account/model health probes.
type HealthConfig struct {
	Enabled              bool          `koanf:"enabled"`
	Interval             time.Duration `koanf:"interval"`
	Timeout              time.Duration `koanf:"timeout"`
	MaxParallel          int           `koanf:"max_parallel"`
	FailureThreshold     int           `koanf:"failure_threshold"`
	SuccessThreshold     int           `koanf:"success_threshold"`
	RecentModelWindow    time.Duration `koanf:"recent_model_window"`
	MaxModelsPerProvider int           `koanf:"max_models_per_provider"`
}

// LogConfig controls structured logging.
type LogConfig struct {
	// Level: debug, info, warn, error.
	Level string `koanf:"level"`
	// Format: json or text.
	Format string `koanf:"format"`
}

// DataConfig controls on-disk state location.
type DataConfig struct {
	// Dir is the root data directory. Empty => OS-specific default.
	Dir string `koanf:"dir"`
}

// GuardrailsConfig wires optional external engines for the guardrails layer.
// Detectors run with their built-in native engines by default; populate the
// sub-configs to enable an external engine on a per-detector basis.
type GuardrailsConfig struct {
	Toxicity ToxicityEngineConfig `koanf:"toxicity"`
	PII      PIIEngineConfig      `koanf:"pii"`
	// AuditRetentionDays drops guardrail_logs older than N days via a
	// background sweeper. Zero / negative disables retention sweeping (the
	// table grows unbounded). 90 is a sensible operational default.
	AuditRetentionDays int `koanf:"audit_retention_days"`
}

// PIIEngineConfig configures the optional Microsoft Presidio HTTP analyzer
// engine. When AnalyzerURL is empty the engine stays disabled and policies
// that select engine="presidio" fall back to native.
type PIIEngineConfig struct {
	PresidioAnalyzerURL string        `koanf:"presidio_analyzer_url"`
	PresidioTimeout     time.Duration `koanf:"presidio_timeout"`
	PresidioLanguage    string        `koanf:"presidio_language"`
}

// ToxicityEngineConfig configures the optional OpenAI Moderation engine. When
// OpenAIAPIKey is empty the OpenAI engine stays disabled and policies that
// select engine="openai" fall back to native.
type ToxicityEngineConfig struct {
	OpenAIAPIKey  string        `koanf:"openai_api_key"`
	OpenAIBaseURL string        `koanf:"openai_base_url"`
	OpenAIModel   string        `koanf:"openai_model"`
	OpenAITimeout time.Duration `koanf:"openai_timeout"`
}

// Default returns the baseline configuration applied before file/env overrides.
func Default() Config {
	return Config{
		Server: ServerConfig{
			Host: "127.0.0.1",
			Port: 20180,
			// 60s prevents premature stall timeouts for codex/Responses
			// streams which may have longer thinking periods between chunks.
			StreamStallTimeout: 60 * time.Second,
			RequestTimeout:     5 * time.Minute,
			CORSOrigins:        []string{"*"},
		},
		Database: DatabaseConfig{
			Driver:       "sqlite",
			MaxOpenConns: 1, // sqlite: serialize writers to avoid SQLITE_BUSY
			MaxIdleConns: 1,
		},
		Security: SecurityConfig{
			SessionTTL:       24 * time.Hour,
			BindLoopbackOnly: true,
		},
		Cache: CacheConfig{
			Enabled:             false,
			Backend:             "memory",
			SimilarityThreshold: 0.95,
			TTL:                 time.Hour,
			EmbeddingProvider:   "hash",
			EmbeddingModel:      "text-embedding-3-small",
			EmbeddingDims:       1536,
		},
		Meter: MeterConfig{
			Async:                true,
			BatchSize:            100,
			FlushInterval:        time.Second,
			QueueSize:            10000,
			FullQueuePolicy:      "sync",
			ShutdownFlushTimeout: 5 * time.Second,
		},
		Limits: LimitsConfig{
			Enabled:         false,
			Backend:         "memory",
			Window:          time.Minute,
			CleanupInterval: time.Minute,
		},
		Health: HealthConfig{
			Enabled:              true,
			Interval:             30 * time.Second,
			Timeout:              5 * time.Second,
			MaxParallel:          8,
			FailureThreshold:     2,
			SuccessThreshold:     1,
			RecentModelWindow:    24 * time.Hour,
			MaxModelsPerProvider: 8,
		},
		Log: LogConfig{Level: "info", Format: "text"},
	}
}

// Load resolves configuration. filePath may be empty to skip file loading.
func Load(filePath string) (Config, error) {
	k := koanf.New(".")

	// 1. Defaults.
	if err := k.Load(structs.Provider(Default(), "koanf"), nil); err != nil {
		return Config{}, fmt.Errorf("load defaults: %w", err)
	}

	// 2. Optional YAML file.
	if filePath != "" {
		if err := k.Load(file.Provider(filePath), yaml.Parser()); err != nil {
			return Config{}, fmt.Errorf("load config file %q: %w", filePath, err)
		}
	}

	// 3. Environment overrides: NEXUSROUTER_SERVER__PORT -> server.port.
	envProvider := env.Provider("NEXUSROUTER_", ".", func(s string) string {
		s = strings.TrimPrefix(s, "NEXUSROUTER_")
		s = strings.ReplaceAll(s, "__", ".")
		s = strings.ToLower(s)
		if s == "security.allow_private_baseurl" {
			return "security.allow_private_base_url"
		}
		return s
	})
	if err := k.Load(envProvider, nil); err != nil {
		return Config{}, fmt.Errorf("load env: %w", err)
	}

	var cfg Config
	if err := k.Unmarshal("", &cfg); err != nil {
		return Config{}, fmt.Errorf("unmarshal config: %w", err)
	}
	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) validate() error {
	switch c.Database.Driver {
	case "sqlite", "postgres":
	default:
		return fmt.Errorf("database.driver must be sqlite or postgres, got %q", c.Database.Driver)
	}
	if c.Database.Driver == "postgres" && c.Database.DSN == "" {
		return fmt.Errorf("database.dsn is required when driver=postgres")
	}
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("server.port out of range: %d", c.Server.Port)
	}
	if c.Meter.BatchSize <= 0 {
		c.Meter.BatchSize = 100
	}
	if c.Meter.FlushInterval <= 0 {
		c.Meter.FlushInterval = time.Second
	}
	if c.Meter.QueueSize <= 0 {
		c.Meter.QueueSize = 10000
	}
	switch c.Meter.FullQueuePolicy {
	case "", "sync", "drop":
	default:
		return fmt.Errorf("meter.full_queue_policy must be sync or drop, got %q", c.Meter.FullQueuePolicy)
	}
	if c.Meter.ShutdownFlushTimeout <= 0 {
		c.Meter.ShutdownFlushTimeout = 5 * time.Second
	}
	switch c.Limits.Backend {
	case "", "memory":
	default:
		return fmt.Errorf("limits.backend must be memory, got %q", c.Limits.Backend)
	}
	if c.Limits.Backend == "" {
		c.Limits.Backend = "memory"
	}
	if c.Limits.DefaultRPM < 0 {
		return fmt.Errorf("limits.default_rpm must not be negative")
	}
	if c.Limits.DefaultTPM < 0 {
		return fmt.Errorf("limits.default_tpm must not be negative")
	}
	if c.Limits.DefaultConcurrency < 0 {
		return fmt.Errorf("limits.default_concurrency must not be negative")
	}
	if c.Limits.Window <= 0 {
		c.Limits.Window = time.Minute
	}
	if c.Limits.CleanupInterval <= 0 {
		c.Limits.CleanupInterval = time.Minute
	}
	if c.Health.Interval <= 0 {
		c.Health.Interval = 30 * time.Second
	}
	if c.Health.Timeout <= 0 {
		c.Health.Timeout = 5 * time.Second
	}
	if c.Health.MaxParallel <= 0 {
		c.Health.MaxParallel = 8
	}
	if c.Health.FailureThreshold <= 0 {
		c.Health.FailureThreshold = 2
	}
	if c.Health.SuccessThreshold <= 0 {
		c.Health.SuccessThreshold = 1
	}
	if c.Health.RecentModelWindow <= 0 {
		c.Health.RecentModelWindow = 24 * time.Hour
	}
	if c.Health.MaxModelsPerProvider <= 0 {
		c.Health.MaxModelsPerProvider = 8
	}
	return nil
}

// Addr returns the host:port the server binds to.
func (c Config) Addr() string {
	return fmt.Sprintf("%s:%d", c.Server.Host, c.Server.Port)
}
