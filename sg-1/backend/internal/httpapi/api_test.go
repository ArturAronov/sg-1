package httpapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ArturAronov/sg-1/sg-1/backend/internal/domain"
	"github.com/ArturAronov/sg-1/sg-1/backend/internal/secrets"
)

const testToken = "test-token-at-least-thirty-two-characters"

type fakeRepository struct {
	createCalls          int
	failPing             bool
	createErr            error
	credentialUser       string
	credentialCiphertext []byte
	credentialCalls      int
}

func (s *fakeRepository) Ping(context.Context) error {
	if s.failPing {
		return errors.New("offline")
	}
	return nil
}
func (s *fakeRepository) User(context.Context, string) (domain.User, error) {
	return domain.User{ID: "test-user", Name: "Test"}, nil
}
func (s *fakeRepository) Departments(context.Context, domain.User) ([]domain.Department, error) {
	return []domain.Department{}, nil
}
func (s *fakeRepository) Projects(context.Context, domain.User) ([]domain.Project, error) {
	return []domain.Project{}, nil
}
func (s *fakeRepository) Project(context.Context, domain.User, string) (domain.Project, error) {
	return domain.Project{}, domain.ErrNotFound
}
func (s *fakeRepository) CreateProject(_ context.Context, _ domain.User, p domain.CreateProject, _ string) (domain.Project, bool, error) {
	s.createCalls++
	return domain.Project{Name: p.Name, Slug: p.Slug, Status: "provisioning", AccessMode: "password"}, true, s.createErr
}

func (s *fakeRepository) LLMCredentials(_ context.Context, userID string) ([]domain.LLMCredentialStatus, error) {
	s.credentialUser = userID
	return []domain.LLMCredentialStatus{}, nil
}
func (s *fakeRepository) SaveLLMCredential(_ context.Context, userID, provider string, ciphertext []byte) (domain.LLMCredentialStatus, error) {
	s.credentialUser = userID
	s.credentialCiphertext = ciphertext
	s.credentialCalls++
	return domain.LLMCredentialStatus{Provider: provider, Configured: true}, nil
}
func (s *fakeRepository) DeleteLLMCredential(_ context.Context, userID, provider string) error {
	s.credentialUser = userID
	s.credentialCalls++
	return nil
}

func testAPI(s *fakeRepository) *API {
	return &API{Store: s, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Development: true, DevToken: testToken, DevUserID: "test-user"}
}

func TestAuthentication(t *testing.T) {
	s := &fakeRepository{}
	a := testAPI(s)
	for _, tc := range []struct {
		path, token string
		status      int
	}{
		{"/healthz", "", 200}, {"/readyz", "", 200}, {"/v1/projects", "", 401},
		{"/v1/projects", "wrong", 401}, {"/v1/projects", testToken, 200},
		{"/v1/projects/private/runtime", "", 401}, {"/v1/projects/private/source", "", 401}, {"/v1/projects/private/files", "", 401},
	} {
		r := httptest.NewRequest(http.MethodGet, tc.path, nil)
		if tc.token != "" {
			r.Header.Set("Authorization", "Bearer "+tc.token)
		}
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Errorf("%s: got %d want %d", tc.path, w.Code, tc.status)
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("missing private response cache policy")
		}
	}
	a.Development = false
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	r.Header.Set("Authorization", "Bearer "+testToken)
	a.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatal("production development-auth bypass")
	}
	s.failPing = true
	w = httptest.NewRecorder()
	a.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if w.Code != 503 {
		t.Fatal("database failure must fail readiness")
	}
}

func TestCreateProjectRequestValidation(t *testing.T) {
	valid := `{"department_id":"00000000-0000-4000-8000-000000000002","name":"Tracker","slug":"tracker"}`
	for _, tc := range []struct {
		name, body, key, contentType string
		status                       int
	}{
		{"valid", valid, "unique-request-key-123", "application/json", 201},
		{"no key", valid, "", "application/json", 400},
		{"bad type", valid, "unique-request-key-123", "text/plain", 415},
		{"unknown field", strings.TrimSuffix(valid, "}") + `,"platform_admin":true}`, "unique-request-key-123", "application/json", 400},
		{"multiple objects", valid + valid, "unique-request-key-123", "application/json", 400},
		{"reserved slug", strings.Replace(valid, `"tracker"`, `"build"`, 1), "unique-request-key-123", "application/json", 400},
		{"oversized", strings.Repeat("x", 17000), "unique-request-key-123", "application/json", 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &fakeRepository{}
			r := httptest.NewRequest(http.MethodPost, "/v1/projects", strings.NewReader(tc.body))
			r.Header.Set("Authorization", "Bearer "+testToken)
			r.Header.Set("Content-Type", tc.contentType)
			r.Header.Set("Idempotency-Key", tc.key)
			w := httptest.NewRecorder()
			testAPI(s).Handler().ServeHTTP(w, r)
			if w.Code != tc.status {
				t.Fatalf("got %d want %d: %s", w.Code, tc.status, w.Body.String())
			}
			if tc.status != 201 && s.createCalls != 0 {
				t.Fatal("invalid request reached store")
			}
		})
	}
}

func TestStoreErrorsDoNotExposeSecrets(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
	}{
		{domain.ErrForbidden, 403}, {domain.ErrSlugTaken, 409}, {domain.ErrIdempotencyConflict, 409}, {errors.New("password=supersecret"), 500},
	} {
		s := &fakeRepository{createErr: tc.err}
		r := httptest.NewRequest(http.MethodPost, "/v1/projects", strings.NewReader(`{"department_id":"00000000-0000-4000-8000-000000000002","name":"Tracker","slug":"tracker"}`))
		r.Header.Set("Authorization", "Bearer "+testToken)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Idempotency-Key", "unique-request-key-123")
		w := httptest.NewRecorder()
		testAPI(s).Handler().ServeHTTP(w, r)
		if w.Code != tc.status || strings.Contains(w.Body.String(), "supersecret") {
			t.Fatalf("unsafe or incorrect error response: %s", w.Body.String())
		}
	}
}

func TestLLMCredentialEndpoints(t *testing.T) {
	vault, err := secrets.NewVault(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{9}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	key := "synthetic-test-key-never-expose-in-response"
	for _, provider := range []string{"openai", "anthropic", "glm", "kimi"} {
		s := &fakeRepository{}
		a := testAPI(s)
		a.Vault = vault
		r := httptest.NewRequest(http.MethodPut, "/v1/settings/llm-providers/"+provider, strings.NewReader(`{"api_key":"`+key+`"}`))
		r.Header.Set("Authorization", "Bearer "+testToken)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		if strings.Contains(w.Body.String(), key) || bytes.Contains(s.credentialCiphertext, []byte(key)) {
			t.Fatal("key exposed as plaintext")
		}
		if s.credentialUser != "test-user" {
			t.Fatal("credential not scoped to authenticated user")
		}
		plain, err := vault.Open("test-user", provider, s.credentialCiphertext)
		if err != nil || string(plain) != key {
			t.Fatal("key not recoverable in trusted backend", err)
		}
	}
}

func TestLLMCredentialGuards(t *testing.T) {
	vault, _ := secrets.NewVault(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{9}, 32)))
	for _, tc := range []struct {
		method, path, body, token string
		vault                     bool
		want                      int
	}{
		{"GET", "/v1/settings/llm-providers", "", "", true, 401},
		{"GET", "/v1/settings/llm-providers", "", testToken, false, 503},
		{"GET", "/v1/settings/llm-providers", "", testToken, true, 200},
		{"PUT", "/v1/settings/llm-providers/openai", `{"api_key":"synthetic-valid-test-key"}`, testToken, false, 503},
		{"PUT", "/v1/settings/llm-providers/unknown", `{"api_key":"synthetic-valid-test-key"}`, testToken, true, 400},
		{"PUT", "/v1/settings/llm-providers/openai", `{"api_key":"short"}`, testToken, true, 400},
		{"PUT", "/v1/settings/llm-providers/openai", `{"api_key":"synthetic-valid-test-key","user_id":"other"}`, testToken, true, 400},
		{"PUT", "/v1/settings/llm-providers/openai", `{"api_key":"synthetic-valid-test-key"}{}`, testToken, true, 400},
		{"DELETE", "/v1/settings/llm-providers/openai", "", "", true, 401},
		{"DELETE", "/v1/settings/llm-providers/openai", "", testToken, false, 204},
	} {
		s := &fakeRepository{}
		a := testAPI(s)
		if tc.vault {
			a.Vault = vault
		}
		r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		r.Header.Set("Content-Type", "application/json")
		if tc.token != "" {
			r.Header.Set("Authorization", "Bearer "+tc.token)
		}
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Fatalf("%s %s: got %d want %d", tc.method, tc.path, w.Code, tc.want)
		}
		if tc.want >= 400 && s.credentialCalls > 0 {
			t.Fatal("rejected request reached credential mutation")
		}
	}
}
