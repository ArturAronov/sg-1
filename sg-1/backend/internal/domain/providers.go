package domain

import (
	"errors"
	"time"
)

type LLMProvider struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

func LLMProviders() []LLMProvider {
	return []LLMProvider{
		{ID: "openai", Name: "OpenAI", Description: "Your OpenAI API key."},
		{ID: "anthropic", Name: "Claude", Description: "Your Anthropic API key for Claude."},
		{ID: "glm", Name: "GLM", Description: "Your Z.AI / Zhipu API key for GLM."},
		{ID: "kimi", Name: "Kimi", Description: "Your Moonshot AI API key for Kimi."},
	}
}

func ValidLLMProvider(id string) bool {
	for _, p := range LLMProviders() {
		if p.ID == id {
			return true
		}
	}
	return false
}

// No key material, including suffixes or ciphertext, belongs in API responses.
type LLMCredentialStatus struct {
	Provider   string     `json:"provider"`
	Configured bool       `json:"configured"`
	UpdatedAt  *time.Time `json:"updated_at"`
}

func ValidateLLMKey(key string) error {
	if len(key) < 16 || len(key) > 4096 {
		return errors.New("API keys must contain 16–4096 characters")
	}
	for _, character := range key {
		if character < 33 || character > 126 {
			return errors.New("API keys cannot contain spaces or non-printable characters")
		}
	}
	// Providers use different key formats. Do not invent a prefix check or claim
	// that local format validation verifies the key with its provider.
	return nil
}
