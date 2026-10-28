package config

import (
	"strings"
	"testing"
)

func TestConfigurationFailsClosed(t *testing.T) {
	t.Setenv("SG1_CREDENTIALS_KEY", "")
	t.Setenv("SG1_DATABASE_URL", "postgres://localhost/test")
	t.Setenv("SG1_ENV", "")
	t.Setenv("SG1_DEV_TOKEN", "")
	c, err := Load()
	if err != nil || c.Environment != "production" {
		t.Fatal("default must be production", err)
	}
	t.Setenv("SG1_DEV_TOKEN", "a-token-that-is-at-least-32-characters")
	if _, err := Load(); err == nil {
		t.Fatal("production accepted development credentials")
	}
	t.Setenv("SG1_ENV", "development")
	if _, err := Load(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SG1_DEV_TOKEN", "short")
	if _, err := Load(); err == nil {
		t.Fatal("accepted short token")
	}
	t.Setenv("SG1_ENV", "unknown")
	if _, err := Load(); err == nil {
		t.Fatal("accepted unknown environment")
	}
}

func TestInvalidCredentialEncryptionKey(t *testing.T) {
	t.Setenv("SG1_ENV", "production")
	t.Setenv("SG1_DEV_TOKEN", "")
	t.Setenv("SG1_DATABASE_URL", "postgres://localhost/test")
	t.Setenv("SG1_CREDENTIALS_KEY", "invalid-key-should-not-be-echoed")
	if _, err := Load(); err == nil || strings.Contains(err.Error(), "invalid-key-should-not-be-echoed") {
		t.Fatal("invalid encryption configuration was accepted or leaked")
	}
}
