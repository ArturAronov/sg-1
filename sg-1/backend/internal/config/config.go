package config

import (
	"errors"
	"os"
)

type Config struct {
	Environment string
	DatabaseURL string
	ListenAddr  string
	DevToken    string
}

func Load() (Config, error) {
	c := Config{Environment: os.Getenv("SG1_ENV"), DatabaseURL: os.Getenv("SG1_DATABASE_URL"), ListenAddr: os.Getenv("SG1_LISTEN_ADDR"), DevToken: os.Getenv("SG1_DEV_TOKEN")}
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
	return c, nil
}
