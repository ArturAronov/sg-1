package httpapi

import (
	"encoding/json"
	"io"
	"mime"
	"net/http"

	"github.com/ArturAronov/sg-1/sg-1/backend/internal/domain"
)

func (a *API) listLLMProviders(w http.ResponseWriter, r *http.Request) {
	if a.Vault == nil {
		writeError(w, 503, "credential_storage_unavailable", "Secure credential storage is not configured")
		return
	}
	configured, err := a.Store.LLMCredentials(r.Context(), currentUser(r).ID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	statuses := make([]domain.LLMCredentialStatus, 0, 4)
	for _, provider := range domain.LLMProviders() {
		status := domain.LLMCredentialStatus{Provider: provider.ID}
		for _, saved := range configured {
			if saved.Provider == provider.ID {
				status = saved
				break
			}
		}
		statuses = append(statuses, status)
	}
	writeJSON(w, 200, map[string]any{"providers": domain.LLMProviders(), "credentials": statuses})
}

func (a *API) saveLLMCredential(w http.ResponseWriter, r *http.Request) {
	if a.Vault == nil {
		writeError(w, 503, "credential_storage_unavailable", "Secure credential storage is not configured")
		return
	}
	provider := r.PathValue("provider")
	if !domain.ValidLLMProvider(provider) {
		writeError(w, 400, "invalid_provider", "Choose OpenAI, Claude, GLM, or Kimi")
		return
	}
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeError(w, 415, "invalid_content_type", "Use application/json")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	var in struct {
		APIKey string `json:"api_key"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&in); err != nil {
		writeError(w, 400, "invalid_json", "Provide a valid API key request")
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeError(w, 400, "invalid_json", "Only one JSON object is allowed")
		return
	}
	if err := domain.ValidateLLMKey(in.APIKey); err != nil {
		writeError(w, 400, "invalid_api_key", err.Error())
		return
	}
	ciphertext, err := a.Vault.Seal(currentUser(r).ID, provider, in.APIKey)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	status, err := a.Store.SaveLLMCredential(r.Context(), currentUser(r).ID, provider, ciphertext)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, 200, map[string]any{"credential": status})
}

func (a *API) deleteLLMCredential(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	if !domain.ValidLLMProvider(provider) {
		writeError(w, 400, "invalid_provider", "Choose OpenAI, Claude, GLM, or Kimi")
		return
	}
	// Removal is deliberately possible even when the encryption key is unavailable.
	if err := a.Store.DeleteLLMCredential(r.Context(), currentUser(r).ID, provider); err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
