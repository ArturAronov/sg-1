package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestProviderProtocols(t *testing.T) {
	generated, _ := json.Marshal(Generated{Summary: "A working application", Files: StarterFiles()})
	for _, provider := range []string{"openai", "anthropic", "glm", "kimi"} {
		t.Run(provider, func(t *testing.T) {
			client := &ProviderClient{HTTP: &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
				if !strings.HasPrefix(r.URL.String(), providerBase(provider)) {
					t.Fatal("unexpected destination")
				}
				if provider == "anthropic" {
					if r.Header.Get("x-api-key") != "synthetic-key" || r.Header.Get("anthropic-version") == "" {
						t.Fatal("missing Anthropic authentication")
					}
				} else if r.Header.Get("Authorization") != "Bearer synthetic-key" {
					t.Fatal("missing bearer authentication")
				}
				if r.URL.Path == "/v1/models" || strings.HasSuffix(r.URL.Path, "/models") {
					return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"data":[{"id":"test-model"}]}`))}, nil
				}
				body, _ := io.ReadAll(r.Body)
				if bytes.Contains(body, []byte("synthetic-key")) {
					t.Fatal("credential was put in model context")
				}
				var request map[string]any
				if json.Unmarshal(body, &request) != nil || request["model"] != "test-model" {
					t.Fatal("invalid model request")
				}
				var response any
				switch provider {
				case "openai":
					if r.URL.Path != "/v1/responses" || request["store"] != false {
						t.Fatal("invalid OpenAI Responses request")
					}
					response = map[string]any{"output": []any{map[string]any{"content": []any{map[string]string{"type": "output_text", "text": string(generated)}}}}}
				case "anthropic":
					if r.URL.Path != "/v1/messages" {
						t.Fatal("invalid Anthropic request")
					}
					response = map[string]any{"content": []any{map[string]string{"type": "text", "text": string(generated)}}}
				default:
					if !strings.HasSuffix(r.URL.Path, "/chat/completions") {
						t.Fatal("invalid compatible provider request")
					}
					response = map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": string(generated)}}}}
				}
				data, _ := json.Marshal(response)
				return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(data))}, nil
			})}}
			models, err := client.Models(context.Background(), provider, "synthetic-key")
			if err != nil || len(models) != 1 {
				t.Fatal("model listing", err)
			}
			app, err := client.Generate(context.Background(), provider, "test-model", "synthetic-key", "Build a tracker", StarterFiles(), nil, "")
			if err != nil || app.Summary != "A working application" {
				t.Fatal("source generation", err)
			}
		})
	}
}

func TestProviderErrorsNeverExposeResponseBody(t *testing.T) {
	c := &ProviderClient{HTTP: &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 401, Body: io.NopCloser(strings.NewReader("private-key-was-echoed-by-provider"))}, nil
	})}}
	_, err := c.Models(context.Background(), "openai", "synthetic-key")
	if err == nil || strings.Contains(err.Error(), "private-key") {
		t.Fatal("unsafe provider error", err)
	}
}

func TestSourceValidation(t *testing.T) {
	for _, name := range []string{"../outside", "/etc/passwd", "a/../../escape", ".git/config", "nested/.env", "a\\b", "a\x00b", "a/../b"} {
		if ValidateFiles(map[string]string{"Dockerfile": "FROM scratch", name: "bad"}) == nil {
			t.Fatal("unsafe source path", name)
		}
	}
	if err := ValidateFiles(StarterFiles()); err != nil {
		t.Fatal(err)
	}
	if ValidateFiles(map[string]string{"server.js": "hello"}) == nil {
		t.Fatal("accepted source without a Dockerfile")
	}
}

func TestGatewayTickets(t *testing.T) {
	s := &Service{SigningKey: bytes.Repeat([]byte{9}, 32)}
	raw := s.sign(ticket{Project: "project-a", Kind: "preview", User: "user-a", Version: 4, Expires: time.Now().Add(time.Minute).Unix()})
	parsed, err := s.verify(raw)
	if err != nil || parsed.Project != "project-a" {
		t.Fatal(err)
	}
	for _, bad := range []string{raw + "x", "invalid", s.sign(ticket{Expires: time.Now().Add(-time.Minute).Unix()})} {
		if _, err := s.verify(bad); err == nil {
			t.Fatal("accepted invalid ticket")
		}
	}
	if s.allowed(context.Background(), ticket{Project: "other", Version: 4}, application{ID: "project-a", Version: 4}) {
		t.Fatal("cross-project ticket accepted")
	}
	if s.allowed(context.Background(), ticket{Project: "project-a", Version: 3, Kind: "password"}, application{ID: "project-a", Version: 4}) {
		t.Fatal("revoked ticket accepted")
	}
}

func TestPrivateProbeCannotBecomeAnOpenProxy(t *testing.T) {
	s := &Service{SigningKey: bytes.Repeat([]byte{4}, 32)}
	for _, value := range []string{"", s.sign(ticket{Kind: "preview", Project: "a", User: "user", Expires: time.Now().Add(time.Minute).Unix()}), s.sign(ticket{Kind: "probe", Project: "a", Target: "127.0.0.1:8080/admin", Expires: time.Now().Add(time.Minute).Unix()})} {
		r := httptest.NewRequest("POST", "http://gateway:8081/__sg1_probe", nil)
		if value != "" {
			r.Header.Set("Authorization", "Bearer "+value)
		}
		w := httptest.NewRecorder()
		s.Gateway("http://localhost:3000").ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatal("unauthorized probe allowed", w.Code)
		}
	}
}
