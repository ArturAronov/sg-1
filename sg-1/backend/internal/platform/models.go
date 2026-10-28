package platform

import (
	"time"

	"github.com/ArturAronov/sg-1/sg-1/backend/internal/domain"
	"github.com/ArturAronov/sg-1/sg-1/backend/internal/secrets"
	"github.com/ArturAronov/sg-1/sg-1/backend/internal/store"
)

type Service struct {
	Store        *store.Store
	Vault        *secrets.Vault
	Docker       *Docker
	AppOrigin    string
	DatabaseHost string
	SigningKey   []byte
	Provider     *ProviderClient
	GatewayURL   string
}

type Message struct {
	ID        string    `json:"id"`
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}
type Build struct {
	ID        string    `json:"id"`
	Provider  string    `json:"provider"`
	Model     string    `json:"model"`
	Prompt    string    `json:"prompt"`
	Status    string    `json:"status"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}
type Deployment struct {
	ID        string    `json:"id"`
	Summary   string    `json:"summary"`
	CreatedAt time.Time `json:"created_at"`
}
type State struct {
	Project             domain.Project `json:"project"`
	Messages            []Message      `json:"messages"`
	Builds              []Build        `json:"builds"`
	Deployments         []Deployment   `json:"deployments"`
	CurrentDeploymentID *string        `json:"current_deployment_id"`
	LastError           *string        `json:"last_error"`
	PasswordSet         bool           `json:"password_set"`
}
