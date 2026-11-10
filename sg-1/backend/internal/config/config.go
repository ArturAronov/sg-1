package config

import (
	"errors"
	"net"
	"net/url"
	"os"
	"regexp"
	"strings"

	"github.com/ArturAronov/sg-1/sg-1/backend/internal/secrets"
	"golang.org/x/net/publicsuffix"
)

var dnsHostname = regexp.MustCompile(`^(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z][a-z0-9-]*$`)

type Config struct {
	Environment                                  string
	DatabaseURL                                  string
	ListenAddr                                   string
	DevToken                                     string
	CredentialsKey                               string
	Runtime                                      string
	AppOrigin                                    string
	PublicURL                                    string
	ProjectDatabaseURL                           string
	Region, ALBARN, CognitoIssuer, CognitoClient string
	BootstrapSubjects                            []string
}

func Load() (Config, error) {
	c := Config{Environment: os.Getenv("SG1_ENV"), DatabaseURL: os.Getenv("SG1_DATABASE_URL"), ListenAddr: os.Getenv("SG1_LISTEN_ADDR"), DevToken: os.Getenv("SG1_DEV_TOKEN"), CredentialsKey: os.Getenv("SG1_CREDENTIALS_KEY")}
	c.Runtime = os.Getenv("SG1_RUNTIME")
	c.AppOrigin = os.Getenv("SG1_APP_ORIGIN")
	c.PublicURL = os.Getenv("SG1_PUBLIC_URL")
	c.ProjectDatabaseURL = os.Getenv("SG1_PROJECT_DATABASE_URL")
	c.Region = os.Getenv("AWS_REGION")
	c.ALBARN = os.Getenv("SG1_ALB_ARN")
	c.CognitoIssuer = os.Getenv("SG1_COGNITO_ISSUER")
	c.CognitoClient = os.Getenv("SG1_COGNITO_CLIENT_ID")
	for _, subject := range strings.Split(os.Getenv("SG1_BOOTSTRAP_SUBJECTS"), ",") {
		if subject = strings.TrimSpace(subject); subject != "" {
			c.BootstrapSubjects = append(c.BootstrapSubjects, subject)
		}
	}
	if c.AppOrigin == "" {
		c.AppOrigin = "http://{slug}.localhost:8081"
	}
	if c.PublicURL == "" {
		c.PublicURL = "http://localhost:42069"
	}
	if c.Runtime != "" && c.Runtime != "docker" && c.Runtime != "aws" {
		return c, errors.New("unknown runtime")
	}
	if c.Runtime == "docker" && c.Environment != "development" {
		return c, errors.New("the Docker runtime is development-only")
	}
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
	if c.Environment == "production" && c.Runtime != "" {
		if c.Runtime != "aws" || c.CredentialsKey == "" {
			return c, errors.New("production requires AWS runtime and an encryption key")
		}
		origin, err := url.Parse(c.PublicURL)
		if err != nil || origin.Scheme != "https" || origin.Hostname() == "" || origin.Path != "" || origin.RawQuery != "" || origin.ForceQuery || origin.Fragment != "" || origin.User != nil || origin.Port() != "" {
			return c, errors.New("production public URL must be an HTTPS origin")
		}
		app, err := url.Parse(strings.ReplaceAll(c.AppOrigin, "{slug}", "example"))
		if err != nil || app.Scheme != "https" || app.Path != "" || app.RawQuery != "" || app.ForceQuery || app.Fragment != "" || app.User != nil || app.Port() != "" {
			return c, errors.New("production application origin must be a separate HTTPS wildcard origin")
		}
		appNamespace := strings.TrimPrefix(app.Hostname(), "example.")
		_, pErr := publicsuffix.EffectiveTLDPlusOne(origin.Hostname())
		_, aErr := publicsuffix.EffectiveTLDPlusOne(appNamespace)
		if pErr != nil || aErr != nil || !dnsHostname.MatchString(origin.Hostname()) || !dnsHostname.MatchString(appNamespace) || net.ParseIP(origin.Hostname()) != nil || c.AppOrigin != "https://{slug}."+appNamespace || origin.Hostname() == appNamespace || strings.HasSuffix(origin.Hostname(), "."+appNamespace) || strings.HasSuffix(appNamespace, "."+origin.Hostname()) {
			return c, errors.New("application wildcard namespace must be disjoint from the platform hostname")
		}
		if !regexp.MustCompile(`^[a-z]{2}-[a-z]+-[0-9]+$`).MatchString(c.Region) || !strings.HasPrefix(c.ALBARN, "arn:aws:elasticloadbalancing:"+c.Region+":") || !strings.Contains(c.ALBARN, ":loadbalancer/app/") {
			return c, errors.New("production ALB signer/region is required")
		}
		if !strings.HasPrefix(c.CognitoIssuer, "https://cognito-idp."+c.Region+".amazonaws.com/") || c.CognitoClient == "" {
			return c, errors.New("production Cognito issuer/client is required")
		}
		for _, raw := range []string{c.DatabaseURL, c.ProjectDatabaseURL} {
			u, e := url.Parse(raw)
			if e != nil || u.Hostname() == "" || u.Query().Get("sslmode") != "verify-full" || u.Query().Get("sslrootcert") == "" {
				return c, errors.New("production RDS connections require verify-full TLS and a CA certificate")
			}
		}
		if os.Getenv("SG1_S3_BUCKET") == "" || os.Getenv("SG1_S3_ENDPOINT") != "" || os.Getenv("SG1_S3_SECRET") != "" || os.Getenv("AWS_ACCESS_KEY_ID") != "" || os.Getenv("AWS_SECRET_ACCESS_KEY") != "" {
			return c, errors.New("production S3 requires a private AWS bucket and workload-role credentials")
		}
	}
	return c, nil
}
