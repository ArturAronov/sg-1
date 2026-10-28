package config

import (
	"errors"
	"os"

	"github.com/ArturAronov/sg-1/sg-1/backend/internal/secrets"
)

type Config struct {
	Environment    string
	DatabaseURL    string
	ListenAddr     string
	DevToken       string
	CredentialsKey string
}

func Load() (Config, error) {
	c := Config{Environment: os.Getenv("SG1_ENV"), DatabaseURL: os.Getenv("SG1_DATABASE_URL"), ListenAddr: os.Getenv("SG1_LISTEN_ADDR"), DevToken: os.Getenv("SG1_DEV_TOKEN"), CredentialsKey: os.Getenv("SG1_CREDENTIALS_KEY")}
	if c.Environment == "" {
		c.Environment = "production"
	}
	if c.ListenAddr == "" {
		c.ListenAddr = "127.0.0.1:8080"
	}
	if c.Environment != "development" && c.Environment != "production" {
		return c, errors.New("SG1_ENV must be development or production")
	}
	if c.DatabaseURL == "" {
		return c, errors.New("SG1_DATABASE_URL is required")
	}
	if c.Environment != "development" && c.DevToken != "" {
		return c, errors.New("development authentication is forbidden outside development")
	}
	if c.Environment == "development" && len(c.DevToken) < 32 {
		return c, errors.New("SG1_DEV_TOKEN must contain at least 32 characters in development")
	}
	if c.CredentialsKey != "" {
		if _, err := secrets.NewVault(c.CredentialsKey); err != nil {
			return c, err
		}
	}
	return c, nil
}
