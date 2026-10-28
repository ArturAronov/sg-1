package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type ProviderClient struct{ HTTP *http.Client }
type Generated struct {
	Summary string            `json:"summary"`
	Files   map[string]string `json:"files"`
}

func providerBase(provider string) string {
	switch provider {
	case "openai":
		return "https://api.openai.com/v1"
	case "anthropic":
		return "https://api.anthropic.com/v1"
	case "glm":
		return "https://api.z.ai/api/paas/v4"
	case "kimi":
		return "https://api.moonshot.ai/v1"
	}
	return ""
}

func (p *ProviderClient) request(ctx context.Context, provider, key, method, endpoint string, body any) ([]byte, error) {
	base := providerBase(provider)
	if base == "" {
		return nil, errors.New("unsupported model provider")
	}
	var input io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		input = bytes.NewReader(data)
	}
	r, err := http.NewRequestWithContext(ctx, method, base+endpoint, input)
	if err != nil {
		return nil, err
	}
	r.Header.Set("Content-Type", "application/json")
	if provider == "anthropic" {
		r.Header.Set("x-api-key", key)
		r.Header.Set("anthropic-version", "2023-06-01")
	} else {
		r.Header.Set("Authorization", "Bearer "+key)
	}
	client := p.HTTP
	if client == nil {
		client = &http.Client{Timeout: 3 * time.Minute, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	response, err := client.Do(r)
	if err != nil {
		return nil, errors.New("model provider could not be reached")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("model provider returned HTTP %d; check the API key, model ID, quota, and provider endpoint", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return nil, errors.New("could not read model response")
	}
	return data, nil
}

func (p *ProviderClient) Models(ctx context.Context, provider, key string) ([]string, error) {
	data, err := p.request(ctx, provider, key, "GET", "/models", nil)
	if err != nil {
		return nil, err
	}
	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if json.Unmarshal(data, &result) != nil {
		return nil, errors.New("provider returned an invalid model list")
	}
	models := []string{}
	for _, model := range result.Data {
		if len(model.ID) > 0 && len(model.ID) <= 200 {
			models = append(models, model.ID)
		}
	}
	return models, nil
}

const buildInstructions = `You build complete internal applications. Respond ONLY with a JSON object {"summary":"short user-facing description","files":{"Dockerfile":"...","relative/path":"full file contents"}}. Return the complete source tree, not patches. Do not include markdown fences.
You may use any stack. The image must listen on 0.0.0.0:8080, run as non-root, work with a read-only root filesystem and writable /tmp, and define a Docker HEALTHCHECK that verifies HTTP /healthz. Use DATABASE_URL at runtime for persistent PostgreSQL data; never store business records only in memory or the container filesystem. Keep schema creation backward-compatible and never drop existing data. All runtime dependencies must be installed during Docker build. There is no internet access at runtime. Never include credentials, .env files, Docker socket access, host mounts, or privileged operations. Do not embed SG-1 or platform credentials. Keep files under 2 MiB total. Avoid custom infrastructure and external runtime services. Applications will be embedded on their own origin behind an authentication gateway. Do not implement or bypass the platform's gateway authentication. Include basic functional checks in the Dockerfile where possible.`

func (p *ProviderClient) Generate(ctx context.Context, provider, model, key, prompt string, files map[string]string, history []Message, repair string) (Generated, error) {
	contextJSON, _ := json.Marshal(map[string]any{"current_files": files, "recent_conversation": history, "request": prompt, "previous_build_failure": repair})
	var body any
	endpoint := "/chat/completions"
	if provider == "anthropic" {
		endpoint = "/messages"
		body = map[string]any{"model": model, "max_tokens": 16000, "system": buildInstructions, "messages": []any{map[string]string{"role": "user", "content": string(contextJSON)}}}
	} else if provider == "openai" {
		endpoint = "/responses"
		body = map[string]any{"model": model, "instructions": buildInstructions, "input": string(contextJSON), "max_output_tokens": 16000, "store": false}
	} else {
		body = map[string]any{"model": model, "messages": []any{map[string]string{"role": "system", "content": buildInstructions}, map[string]string{"role": "user", "content": string(contextJSON)}}}
	}
	data, err := p.request(ctx, provider, key, "POST", endpoint, body)
	if err != nil {
		return Generated{}, err
	}
	var text string
	if provider == "anthropic" {
		var response struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		}
		if json.Unmarshal(data, &response) != nil {
			return Generated{}, errors.New("invalid provider response")
		}
		for _, part := range response.Content {
			if part.Type == "text" {
				text += part.Text
			}
		}
	} else if provider == "openai" {
		var response struct {
			Output []struct {
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			} `json:"output"`
		}
		if json.Unmarshal(data, &response) != nil {
			return Generated{}, errors.New("invalid provider response")
		}
		for _, message := range response.Output {
			for _, part := range message.Content {
				if part.Type == "output_text" {
					text += part.Text
				}
			}
		}
	} else {
		var response struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		if json.Unmarshal(data, &response) != nil || len(response.Choices) == 0 {
			return Generated{}, errors.New("provider returned no application source")
		}
		text = response.Choices[0].Message.Content
	}
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "```") {
		if i := strings.Index(text, "\n"); i >= 0 {
			text = strings.TrimSpace(strings.TrimSuffix(text[i+1:], "```"))
		}
	}
	var generated Generated
	if json.Unmarshal([]byte(text), &generated) != nil {
		return generated, errors.New("model did not return a complete JSON source tree")
	}
	if len(generated.Summary) > 2000 {
		generated.Summary = generated.Summary[:2000]
	}
	return generated, ValidateFiles(generated.Files)
}
