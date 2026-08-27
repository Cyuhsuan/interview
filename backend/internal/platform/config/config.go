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

	DatabaseURL              string
	DBMaxOpenConns           int
	DBMaxIdleConns           int
	DBConnMaxLifetimeMinutes int
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
