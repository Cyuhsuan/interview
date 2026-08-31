package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Env            string
	Port           string
	ClinicTimezone string
	FrontendOrigin string

	DatabaseURL              string
	DBMaxOpenConns           int
	DBMaxIdleConns           int
	DBConnMaxLifetimeMinutes int

	// Scheduling/Booking computation parameters. These are operational
	// parameters (not availability-judgment inputs, which per README must
	// live in PostgreSQL — see clinic_hours/clinic_closures), so they are
	// required, fail-closed environment variables with no default, same as
	// ClinicTimezone.
	ClinicSlotIntervalMinutes int
	ClinicMinLeadMinutes      int
	BookingSessionTTLMinutes  int

	// AI Provider adapter (internal/ai). See backend/README.md's "AI
	// Provider Adapter Contract" — this is a development/test-time
	// OpenAI-compatible integration, not an approved production AI
	// provider, so it fails closed like other clinic-specific settings
	// rather than falling back to an implicit default.
	AIProviderAPIKey  string
	AIProviderBaseURL string
	AIProviderModel   string

	// AIProviderTranscriptionModel is the model used for the voice
	// transcription adapter (internal/ai.TranscriptionClient). It reuses
	// AIProviderBaseURL/AIProviderAPIKey — see backend/README.md's "Voice
	// Transcription Endpoint" — rather than introducing a separate
	// credential set for a hypothetical different provider.
	AIProviderTranscriptionModel string

	// Calendar outbox worker (internal/service/calendar, cmd/calendar-worker).
	// Retry interval, max attempts and the dead_letter alert target/SLA are
	// still pending clinic confirmation (README's 待診所確認 item 3), so
	// these fail closed with no implicit default, same as the scheduling
	// parameters above.
	CalendarOutboxMaxAttempts         int
	CalendarOutboxRetryBackoffSeconds int
	CalendarWorkerPollIntervalSeconds int
}

func Load() (Config, error) {
	env := os.Getenv("APP_ENV")
	if env == "" {
		env = "development"
	}

	if env != "production" {
		_ = godotenv.Load()
	}

	cfg := Config{
		Env:            env,
		Port:           os.Getenv("PORT"),
		ClinicTimezone: os.Getenv("CLINIC_TIMEZONE"),
		DatabaseURL:    os.Getenv("DATABASE_URL"),
	}

	if cfg.Port == "" {
		cfg.Port = "8080"
	}

	cfg.FrontendOrigin = os.Getenv("FRONTEND_ORIGIN")
	if cfg.FrontendOrigin == "" {
		cfg.FrontendOrigin = "http://localhost:5173"
	}

	if cfg.ClinicTimezone == "" {
		return Config{}, fmt.Errorf("CLINIC_TIMEZONE is required")
	}
	if _, err := time.LoadLocation(cfg.ClinicTimezone); err != nil {
		return Config{}, fmt.Errorf("CLINIC_TIMEZONE %q is not a valid IANA timezone: %w", cfg.ClinicTimezone, err)
	}

	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}

	var err error
	if cfg.DBMaxOpenConns, err = intEnv("DB_MAX_OPEN_CONNS", 25); err != nil {
		return Config{}, err
	}
	if cfg.DBMaxIdleConns, err = intEnv("DB_MAX_IDLE_CONNS", 25); err != nil {
		return Config{}, err
	}
	if cfg.DBConnMaxLifetimeMinutes, err = intEnv("DB_CONN_MAX_LIFETIME_MINUTES", 5); err != nil {
		return Config{}, err
	}

	if cfg.ClinicSlotIntervalMinutes, err = requiredPositiveIntEnv("CLINIC_SLOT_INTERVAL_MINUTES"); err != nil {
		return Config{}, err
	}
	if cfg.ClinicMinLeadMinutes, err = requiredPositiveIntEnv("CLINIC_MIN_LEAD_MINUTES"); err != nil {
		return Config{}, err
	}
	if cfg.BookingSessionTTLMinutes, err = requiredPositiveIntEnv("BOOKING_SESSION_TTL_MINUTES"); err != nil {
		return Config{}, err
	}

	if cfg.AIProviderAPIKey, err = requiredStringEnv("AI_PROVIDER_API_KEY"); err != nil {
		return Config{}, err
	}
	if cfg.AIProviderBaseURL, err = requiredStringEnv("AI_PROVIDER_BASE_URL"); err != nil {
		return Config{}, err
	}
	if cfg.AIProviderModel, err = requiredStringEnv("AI_PROVIDER_MODEL"); err != nil {
		return Config{}, err
	}
	if cfg.AIProviderTranscriptionModel, err = requiredStringEnv("AI_PROVIDER_TRANSCRIPTION_MODEL"); err != nil {
		return Config{}, err
	}

	if cfg.CalendarOutboxMaxAttempts, err = requiredPositiveIntEnv("CALENDAR_OUTBOX_MAX_ATTEMPTS"); err != nil {
		return Config{}, err
	}
	if cfg.CalendarOutboxRetryBackoffSeconds, err = requiredPositiveIntEnv("CALENDAR_OUTBOX_RETRY_BACKOFF_SECONDS"); err != nil {
		return Config{}, err
	}
	if cfg.CalendarWorkerPollIntervalSeconds, err = requiredPositiveIntEnv("CALENDAR_WORKER_POLL_INTERVAL_SECONDS"); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func intEnv(key string, fallback int) (int, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", key, err)
	}
	return value, nil
}

// requiredPositiveIntEnv fails startup instead of falling back to a
// default, per the MVP principle in root AGENTS.md against implicit
// default values for clinic-specific scheduling parameters.
func requiredPositiveIntEnv(key string) (int, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return 0, fmt.Errorf("%s is required", key)
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", key, err)
	}
	if value <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", key)
	}
	return value, nil
}

// requiredStringEnv fails startup instead of falling back to an implicit
// default, same rationale as requiredPositiveIntEnv.
func requiredStringEnv(key string) (string, error) {
	value := os.Getenv(key)
	if value == "" {
		return "", fmt.Errorf("%s is required", key)
	}
	return value, nil
}
