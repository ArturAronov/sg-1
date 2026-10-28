package domain

import (
	"strings"
	"testing"
)

func TestProvidersAndKeyValidation(t *testing.T) {
	for _, provider := range []string{"openai", "anthropic", "glm", "kimi"} {
		if !ValidLLMProvider(provider) {
			t.Fatal("missing provider", provider)
		}
	}
	for _, provider := range []string{"claude", "unknown", "../openai", "OpenAI", ""} {
		if ValidLLMProvider(provider) {
			t.Fatal("accepted unknown provider", provider)
		}
	}
	for _, key := range []string{"short", strings.Repeat("a", 4097), "spaces are not accepted", "test-key-with\nnewline", "test-key-with-unicode-é"} {
		if ValidateLLMKey(key) == nil {
			t.Fatal("invalid key accepted")
		}
	}
	if err := ValidateLLMKey("synthetic.provider-key_123456"); err != nil {
		t.Fatal(err)
	}
}
