package platform

import (
	"time"

	"github.com/ArturAronov/sg-1/sg-1/backend/internal/domain"
	"github.com/ArturAronov/sg-1/sg-1/backend/internal/secrets"
	"github.com/ArturAronov/sg-1/sg-1/backend/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	Store        *store.Store
	Vault        *secrets.Vault
	Docker       *Docker
	Cloud        Runtime
	ProjectPool  *pgxpool.Pool
	DatabaseTLS  bool
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
	Runtime             string         `json:"runtime"`
	Project             domain.Project `json:"project"`
	Messages            []Message      `json:"messages"`
	Builds              []Build        `json:"builds"`
	Deployments         []Deployment   `json:"deployments"`
	CurrentDeploymentID *string        `json:"current_deployment_id"`
	LastError           *string        `json:"last_error"`
	PasswordSet         bool           `json:"password_set"`
}
