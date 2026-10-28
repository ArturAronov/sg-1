package httpapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/ArturAronov/sg-1/sg-1/backend/internal/domain"
	"github.com/ArturAronov/sg-1/sg-1/backend/internal/platform"
	"github.com/ArturAronov/sg-1/sg-1/backend/internal/secrets"
	"github.com/ArturAronov/sg-1/sg-1/backend/internal/storage"
)

type Repository interface {
	Ping(context.Context) error
	User(context.Context, string) (domain.User, error)
	Departments(context.Context, domain.User) ([]domain.Department, error)
	Projects(context.Context, domain.User) ([]domain.Project, error)
	Project(context.Context, domain.User, string) (domain.Project, error)
	CreateProject(context.Context, domain.User, domain.CreateProject, string) (domain.Project, bool, error)
	LLMCredentials(context.Context, string) ([]domain.LLMCredentialStatus, error)
	SaveLLMCredential(context.Context, string, string, []byte) (domain.LLMCredentialStatus, error)
	DeleteLLMCredential(context.Context, string, string) error
}

type API struct {
	Store  Repository
	Logger *slog.Logger
	// Development auth is opt-in; production authentication is not implemented yet.
	Development bool
	DevToken    string
	DevUserID   string
	Vault       *secrets.Vault
	Platform    *platform.Service
	Files       *storage.S3
}

type userContextKey struct{}

var keyPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{16,128}$`)

func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := a.Store.Ping(ctx); err != nil {
			writeError(w, http.StatusServiceUnavailable, "not_ready", "Database or migrations are unavailable")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})
	protected := http.NewServeMux()
	mux.HandleFunc("GET /public/projects/{slug}", func(w http.ResponseWriter, r *http.Request) {
		if a.Platform == nil {
			writeError(w, 503, "runtime_unavailable", "Application serving is not configured")
			return
		}
		slug := r.PathValue("slug")
		if domain.ValidateSlug(slug) != nil {
			writeError(w, 404, "not_found", "Application not found")
			return
		}
		var ready bool
		err := a.Platform.Store.Pool.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM projects p JOIN project_runtimes r ON r.project_id=p.id WHERE p.slug=$1 AND p.status='ready' AND r.current_deployment_id IS NOT NULL)`, slug).Scan(&ready)
		if err != nil || !ready {
			writeError(w, 404, "not_found", "Application not found")
			return
		}
		writeJSON(w, 200, map[string]string{"url": strings.ReplaceAll(a.Platform.AppOrigin, "{slug}", slug)})
	})
	protected.HandleFunc("GET /v1/me", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"user": currentUser(r), "authentication": "development", "capabilities": map[string]bool{"provisioning": a.Platform != nil, "ai_builds": a.Platform != nil, "sso": false}})
	})
	protected.HandleFunc("GET /v1/departments", func(w http.ResponseWriter, r *http.Request) {
		items, err := a.Store.Departments(r.Context(), currentUser(r))
		if err != nil {
			a.fail(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"departments": items})
	})
	protected.HandleFunc("GET /v1/projects", func(w http.ResponseWriter, r *http.Request) {
		items, err := a.Store.Projects(r.Context(), currentUser(r))
		if err != nil {
			a.fail(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"projects": items})
	})
	protected.HandleFunc("GET /v1/projects/{slug}", func(w http.ResponseWriter, r *http.Request) {
		p, err := a.Store.Project(r.Context(), currentUser(r), r.PathValue("slug"))
		if err != nil {
			a.fail(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"project": p})
	})
	protected.HandleFunc("POST /v1/projects", a.createProject)
	protected.HandleFunc("GET /v1/settings/llm-providers", a.listLLMProviders)
	protected.HandleFunc("PUT /v1/settings/llm-providers/{provider}", a.saveLLMCredential)
	protected.HandleFunc("DELETE /v1/settings/llm-providers/{provider}", a.deleteLLMCredential)
	a.runtimeRoutes(protected)
	a.fileRoutes(protected)
	mux.Handle("/v1/", a.authenticate(protected))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		mux.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (a *API) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.Development || len(a.DevToken) < 32 {
			writeError(w, http.StatusServiceUnavailable, "authentication_unconfigured", "SSO is not configured")
			return
		}
		token, found := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !found || subtle.ConstantTimeCompare([]byte(token), []byte(a.DevToken)) != 1 {
			writeError(w, http.StatusUnauthorized, "unauthenticated", "Authentication is required")
			return
		}
		u, err := a.Store.User(r.Context(), a.DevUserID)
		if errors.Is(err, domain.ErrNotFound) {
			writeError(w, http.StatusUnauthorized, "unauthenticated", "Development user is unavailable; run seed-dev")
			return
		}
		if err != nil {
			a.fail(w, r, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userContextKey{}, u)))
	})
}

func currentUser(r *http.Request) domain.User {
	return r.Context().Value(userContextKey{}).(domain.User)
}

func (a *API) createProject(w http.ResponseWriter, r *http.Request) {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeError(w, http.StatusUnsupportedMediaType, "invalid_content_type", "Use application/json")
		return
	}
	key := r.Header.Get("Idempotency-Key")
	if !keyPattern.MatchString(key) {
		writeError(w, http.StatusBadRequest, "invalid_idempotency_key", "Provide an Idempotency-Key of 16–128 letters, numbers, hyphens, or underscores")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	var in domain.CreateProject
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Provide a valid project request")
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeError(w, http.StatusBadRequest, "invalid_json", "Only one JSON object is allowed")
		return
	}
	if err := in.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_project", err.Error())
		return
	}
	p, created, err := a.Store.CreateProject(r.Context(), currentUser(r), in, key)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	w.Header().Set("Location", "/v1/projects/"+p.Slug)
	writeJSON(w, status, map[string]any{"project": p, "provisioning": "queued", "message": "Project reserved. The provisioning worker will start its container."})
}

func (a *API) fail(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "Project not found")
	case errors.Is(err, domain.ErrForbidden):
		writeError(w, http.StatusForbidden, "forbidden", "You do not have permission for this action")
	case errors.Is(err, domain.ErrSlugTaken):
		writeError(w, http.StatusConflict, "slug_taken", "That project URL is already in use")
	case errors.Is(err, domain.ErrIdempotencyConflict):
		writeError(w, http.StatusConflict, "idempotency_conflict", "This request key was already used for different project details")
	default:
		// Do not log raw SQL errors: they can include submitted values or credentials.
		a.Logger.ErrorContext(r.Context(), "request failed", "method", r.Method, "path", r.URL.Path, "error_type", "internal")
		writeError(w, http.StatusInternalServerError, "internal_error", "The request could not be completed")
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}
