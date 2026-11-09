package config

import (
	"strings"
	"testing"
)

func TestConfigurationFailsClosed(t *testing.T) {
	t.Setenv("SG1_RUNTIME", "")
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
	t.Setenv("SG1_RUNTIME", "")
	t.Setenv("SG1_ENV", "production")
	t.Setenv("SG1_DEV_TOKEN", "")
	t.Setenv("SG1_DATABASE_URL", "postgres://localhost/test")
	t.Setenv("SG1_CREDENTIALS_KEY", "invalid-key-should-not-be-echoed")
	if _, err := Load(); err == nil || strings.Contains(err.Error(), "invalid-key-should-not-be-echoed") {
		t.Fatal("invalid encryption configuration was accepted or leaked")
	}
}

func TestPublicURLDefaultAndOverride(t *testing.T) {
	t.Setenv("SG1_ENV", "production")
	t.Setenv("SG1_RUNTIME", "")
	t.Setenv("SG1_DEV_TOKEN", "")
	t.Setenv("SG1_CREDENTIALS_KEY", "")
	t.Setenv("SG1_DATABASE_URL", "postgres://localhost/test")
	for _, tt := range []struct {
		name, value, want string
	}{
		{"default", "", "http://localhost:42069"},
		{"override", "http://localhost:54321", "http://localhost:54321"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("SG1_PUBLIC_URL", tt.value)
			c, err := Load()
			if err != nil {
				t.Fatal(err)
			}
			if c.PublicURL != tt.want {
				t.Fatalf("PublicURL = %q, want %q", c.PublicURL, tt.want)
			}
		})
	}
}

func TestAWSProductionConfigurationFailsClosed(t *testing.T) {
	values := map[string]string{
		"SG1_ENV": "production", "SG1_RUNTIME": "aws", "SG1_DATABASE_URL": "postgres://runtime:password@control.example/sg1?sslmode=verify-full&sslrootcert=/ca.pem",
		"SG1_PROJECT_DATABASE_URL": "postgres://admin:password@apps.example/sg1_apps?sslmode=verify-full&sslrootcert=/ca.pem",
		"SG1_PUBLIC_URL":           "https://sg1.example.com", "SG1_APP_ORIGIN": "https://{slug}.apps.example-apps.com", "AWS_REGION": "us-east-1",
		"SG1_ALB_ARN":        "arn:aws:elasticloadbalancing:us-east-1:123456789012:loadbalancer/app/sg1/abc",
		"SG1_COGNITO_ISSUER": "https://cognito-idp.us-east-1.amazonaws.com/us-east-1_test", "SG1_COGNITO_CLIENT_ID": "test-client",
		"SG1_CREDENTIALS_KEY": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=", "SG1_S3_BUCKET": "private-files",
		"SG1_DEV_TOKEN": "", "SG1_S3_ENDPOINT": "", "SG1_S3_SECRET": "", "AWS_ACCESS_KEY_ID": "", "AWS_SECRET_ACCESS_KEY": "",
	}
	for key, value := range values {
		t.Setenv(key, value)
	}
	if _, err := Load(); err != nil {
		t.Fatal("valid AWS configuration rejected", err)
	}
	for _, test := range []struct{ key, value string }{
		{"SG1_DEV_TOKEN", "shared-development-token"}, {"SG1_PUBLIC_URL", "http://sg1.example.com"},
		{"SG1_DATABASE_URL", "postgres://runtime:password@control.example/sg1?sslmode=disable"},
		{"SG1_PROJECT_DATABASE_URL", ""}, {"SG1_ALB_ARN", ""}, {"SG1_COGNITO_CLIENT_ID", ""},
		{"SG1_APP_ORIGIN", "https://{slug}.apps.example.com"},
		{"SG1_S3_BUCKET", ""}, {"SG1_S3_ENDPOINT", "http://local-s3"}, {"AWS_ACCESS_KEY_ID", "static-key"},
	} {
		t.Run(test.key, func(t *testing.T) {
			t.Setenv(test.key, test.value)
			if _, err := Load(); err == nil {
				t.Fatal("unsafe production configuration accepted")
			}
		})
	}
}
