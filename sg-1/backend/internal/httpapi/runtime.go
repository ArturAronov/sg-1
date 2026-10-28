package httpapi

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/ArturAronov/sg-1/sg-1/backend/internal/domain"
	"github.com/ArturAronov/sg-1/sg-1/backend/internal/platform"
	"github.com/jackc/pgx/v5"
)

func decodeRuntimeBody(w http.ResponseWriter, r *http.Request, target any) bool {
	if strings.Split(r.Header.Get("Content-Type"), ";")[0] != "application/json" {
		writeError(w, 415, "invalid_content_type", "Use application/json")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 20<<10)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(target) != nil || d.Decode(&struct{}{}) != io.EOF {
		writeError(w, 400, "invalid_json", "Provide one valid JSON object")
		return false
	}
	return true
}

func (a *API) runtimeRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/projects/{slug}/source", func(w http.ResponseWriter, r *http.Request) {
		if a.Platform == nil {
			writeError(w, 503, "runtime_unavailable", "Runtime is not configured")
			return
		}
		p, err := a.Store.Project(r.Context(), currentUser(r), r.PathValue("slug"))
		if err != nil {
			a.fail(w, r, err)
			return
		}
		var raw []byte
		err = a.Platform.Store.Pool.QueryRow(r.Context(), `SELECT d.files FROM project_runtimes r JOIN deployments d ON d.id=r.current_deployment_id WHERE r.project_id=$1`, p.ID).Scan(&raw)
		if errors.Is(err, pgx.ErrNoRows) {
			a.fail(w, r, domain.ErrNotFound)
			return
		}
		if err != nil {
			a.fail(w, r, err)
			return
		}
		files := map[string]string{}
		if json.Unmarshal(raw, &files) != nil || platform.ValidateFiles(files) != nil {
			writeError(w, 500, "invalid_source", "Source archive is unavailable")
			return
		}
		var archive bytes.Buffer
		z := zip.NewWriter(&archive)
		for name, content := range files {
			file, err := z.Create(name)
			if err != nil {
				a.fail(w, r, err)
				return
			}
			if _, err = file.Write([]byte(content)); err != nil {
				a.fail(w, r, err)
				return
			}
		}
		if err = z.Close(); err != nil {
			a.fail(w, r, err)
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", `attachment; filename="`+p.Slug+`.zip"`)
		w.WriteHeader(200)
		_, _ = w.Write(archive.Bytes())
	})
	wrap := func(handler http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if a.Platform == nil {
				writeError(w, 503, "runtime_unavailable", "Application runtime is not configured")
				return
			}
			handler(w, r)
		}
	}
	mux.HandleFunc("GET /v1/projects/{slug}/runtime", wrap(func(w http.ResponseWriter, r *http.Request) {
		state, err := a.Platform.State(r.Context(), currentUser(r), r.PathValue("slug"))
		if err != nil {
			a.fail(w, r, err)
			return
		}
		writeJSON(w, 200, state)
	}))
	mux.HandleFunc("POST /v1/projects/{slug}/builds", wrap(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Provider string `json:"provider"`
			Model    string `json:"model"`
			Prompt   string `json:"prompt"`
		}
		if !decodeRuntimeBody(w, r, &in) {
			return
		}
		key := r.Header.Get("Idempotency-Key")
		if !keyPattern.MatchString(key) || !domain.ValidLLMProvider(in.Provider) || len(in.Model) < 1 || len(in.Model) > 200 || len(strings.TrimSpace(in.Prompt)) < 1 || len(in.Prompt) > 12000 {
			writeError(w, 400, "invalid_build", "Choose a provider, model, request, and a valid request key")
			return
		}
		id, err := a.Platform.QueueBuild(r.Context(), currentUser(r), r.PathValue("slug"), in.Provider, in.Model, in.Prompt, key, "")
		if err != nil {
			a.runtimeError(w, r, err)
			return
		}
		writeJSON(w, 202, map[string]string{"build_id": id})
	}))
	mux.HandleFunc("POST /v1/projects/{slug}/preview", wrap(func(w http.ResponseWriter, r *http.Request) {
		url, err := a.Platform.PreviewURL(r.Context(), currentUser(r), r.PathValue("slug"))
		if err != nil {
			a.runtimeError(w, r, err)
			return
		}
		writeJSON(w, 200, map[string]string{"url": url})
	}))
	mux.HandleFunc("PUT /v1/projects/{slug}/access", wrap(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Mode     string `json:"mode"`
			Password string `json:"password"`
		}
		if !decodeRuntimeBody(w, r, &in) {
			return
		}
		if in.Mode != "public" && in.Mode != "password" || in.Password != "" && (len(in.Password) < 12 || len(in.Password) > 72) {
			writeError(w, 400, "invalid_access", "Choose public/password access and a password of 12–72 bytes")
			return
		}
		if err := a.Platform.SetAccess(r.Context(), currentUser(r), r.PathValue("slug"), in.Mode, in.Password); err != nil {
			a.runtimeError(w, r, err)
			return
		}
		writeJSON(w, 200, map[string]string{"status": "saved"})
	}))
	mux.HandleFunc("POST /v1/projects/{slug}/rollback", wrap(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			DeploymentID string `json:"deployment_id"`
		}
		if !decodeRuntimeBody(w, r, &in) {
			return
		}
		key := r.Header.Get("Idempotency-Key")
		if !keyPattern.MatchString(key) || len(in.DeploymentID) != 36 {
			writeError(w, 400, "invalid_rollback", "Choose a deployment and valid request key")
			return
		}
		id, err := a.Platform.QueueBuild(r.Context(), currentUser(r), r.PathValue("slug"), "", "", "Restore deployment "+in.DeploymentID, key, in.DeploymentID)
		if err != nil {
			a.runtimeError(w, r, err)
			return
		}
		writeJSON(w, 202, map[string]string{"build_id": id})
	}))
	mux.HandleFunc("GET /v1/settings/llm-providers/{provider}/models", wrap(func(w http.ResponseWriter, r *http.Request) {
		provider := r.PathValue("provider")
		if !domain.ValidLLMProvider(provider) {
			writeError(w, 400, "invalid_provider", "Unknown provider")
			return
		}
		models, err := a.Platform.Models(r.Context(), currentUser(r), provider)
		if err != nil {
			writeError(w, 400, "provider_unavailable", err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"models": models})
	}))
	mux.HandleFunc("POST /v1/projects/{slug}/retry", wrap(func(w http.ResponseWriter, r *http.Request) {
		p, err := a.Store.Project(r.Context(), currentUser(r), r.PathValue("slug"))
		if err != nil {
			a.fail(w, r, err)
			return
		}
		if p.Role != "admin" {
			a.fail(w, r, domain.ErrForbidden)
			return
		}
		if p.Status != "failed" {
			writeError(w, 409, "not_failed", "Only failed provisioning can be retried")
			return
		}
		tx, err := a.Platform.Store.Pool.Begin(r.Context())
		if err != nil {
			a.fail(w, r, err)
			return
		}
		defer tx.Rollback(r.Context())
		if _, err = tx.Exec(r.Context(), `UPDATE provisioning_jobs SET status='queued',attempts=0,last_error_code=NULL WHERE project_id=$1`, p.ID); err != nil {
			a.fail(w, r, err)
			return
		}
		if _, err = tx.Exec(r.Context(), `UPDATE projects SET status='provisioning' WHERE id=$1`, p.ID); err != nil {
			a.fail(w, r, err)
			return
		}
		if err = tx.Commit(r.Context()); err != nil {
			a.fail(w, r, err)
			return
		}
		writeJSON(w, 202, map[string]string{"status": "queued"})
	}))
}

func (a *API) runtimeError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, platform.ErrBusy) || errors.Is(err, platform.ErrNotReady) {
		writeError(w, 409, "project_busy", err.Error())
		return
	}
	if strings.HasPrefix(err.Error(), "save an API key") || strings.HasPrefix(err.Error(), "provider key cannot") {
		writeError(w, 400, "provider_key_required", err.Error())
		return
	}
	a.fail(w, r, err)
}
