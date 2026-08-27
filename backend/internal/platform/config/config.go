package config

import (
	"fmt"
	"os"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Env            string
	Port           string
	ClinicTimezone string
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

	return cfg, nil
}
