package config

import "testing"

func TestConfigurationFailsClosed(t *testing.T) {
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
