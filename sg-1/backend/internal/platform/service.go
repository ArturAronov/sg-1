package platform

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/ArturAronov/sg-1/sg-1/backend/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/bcrypt"
)

var ErrBusy = errors.New("a build is already running")
var ErrNotReady = errors.New("project is not ready")

func (s *Service) State(ctx context.Context, u domain.User, slug string) (State, error) {
	p, err := s.Store.Project(ctx, u, slug)
	if err != nil {
		return State{}, err
	}
	state := State{Project: p, Messages: []Message{}, Builds: []Build{}, Deployments: []Deployment{}}
	rows, err := s.Store.Pool.Query(ctx, `SELECT id::text,role,content,created_at FROM (SELECT * FROM project_messages WHERE project_id=$1 ORDER BY created_at DESC LIMIT 60) m ORDER BY created_at`, p.ID)
	if err != nil {
		return state, err
	}
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.Role, &m.Content, &m.CreatedAt); err != nil {
			rows.Close()
			return state, err
		}
		state.Messages = append(state.Messages, m)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return state, err
	}
	rows, err = s.Store.Pool.Query(ctx, `SELECT id::text,coalesce(provider,'system'),coalesce(model,'rollback'),prompt,status,message,created_at FROM build_jobs WHERE project_id=$1 ORDER BY created_at DESC LIMIT 20`, p.ID)
	if err != nil {
		return state, err
	}
	for rows.Next() {
		var b Build
		if err := rows.Scan(&b.ID, &b.Provider, &b.Model, &b.Prompt, &b.Status, &b.Message, &b.CreatedAt); err != nil {
			rows.Close()
			return state, err
		}
		state.Builds = append(state.Builds, b)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return state, err
	}
	rows, err = s.Store.Pool.Query(ctx, `SELECT id::text,summary,created_at FROM deployments WHERE project_id=$1 ORDER BY created_at DESC LIMIT 20`, p.ID)
	if err != nil {
		return state, err
	}
	for rows.Next() {
		var d Deployment
		if err := rows.Scan(&d.ID, &d.Summary, &d.CreatedAt); err != nil {
			rows.Close()
			return state, err
		}
		state.Deployments = append(state.Deployments, d)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return state, err
	}
	_ = s.Store.Pool.QueryRow(ctx, `SELECT current_deployment_id::text,last_error FROM project_runtimes WHERE project_id=$1`, p.ID).Scan(&state.CurrentDeploymentID, &state.LastError)
	err = s.Store.Pool.QueryRow(ctx, `SELECT password_hash IS NOT NULL FROM project_access_settings WHERE project_id=$1`, p.ID).Scan(&state.PasswordSet)
	return state, err
}

func (s *Service) QueueBuild(ctx context.Context, u domain.User, slug, provider, model, prompt, key, target string) (string, error) {
	p, err := s.Store.Project(ctx, u, slug)
	if err != nil {
		return "", err
	}
	if p.Role == "read" {
		return "", domain.ErrForbidden
	}
	if p.Status != "ready" {
		return "", ErrNotReady
	}
	if len(key) < 16 || len(key) > 128 {
		return "", errors.New("a request key is required")
	}
	if target == "" && (!domain.ValidLLMProvider(provider) || len(model) < 1 || len(model) > 200 || len(prompt) < 1 || len(prompt) > 12000) {
		return "", errors.New("choose a provider, model, and a request of 1–12000 characters")
	}
	if target == "" {
		if _, err := s.providerKey(ctx, u.ID, provider); err != nil {
			return "", err
		}
	}
	tx, err := s.Store.Pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	var id, existingPrompt, existingModel, existingProvider string
	err = tx.QueryRow(ctx, `SELECT id::text,prompt,coalesce(model,''),coalesce(provider,'') FROM build_jobs WHERE project_id=$1 AND actor_id=$2 AND request_key=$3`, p.ID, u.ID, key).Scan(&id, &existingPrompt, &existingModel, &existingProvider)
	if err == nil {
		if existingPrompt != prompt || existingModel != model || existingProvider != provider {
			return "", domain.ErrIdempotencyConflict
		}
		return id, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	if target != "" {
		var exists bool
		err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM deployments WHERE id::text=$1 AND project_id=$2)`, target, p.ID).Scan(&exists)
		if err != nil {
			return "", err
		}
		if !exists {
			return "", domain.ErrNotFound
		}
		err = tx.QueryRow(ctx, `INSERT INTO build_jobs(project_id,actor_id,kind,target_deployment_id,prompt,request_key) VALUES($1,$2,'rollback',$3,$4,$5) RETURNING id::text`, p.ID, u.ID, target, prompt, key).Scan(&id)
	} else {
		err = tx.QueryRow(ctx, `INSERT INTO build_jobs(project_id,actor_id,provider,model,prompt,request_key) VALUES($1,$2,$3,$4,$5,$6) RETURNING id::text`, p.ID, u.ID, provider, model, prompt, key).Scan(&id)
	}
	if err != nil {
		var pgerr *pgconn.PgError
		if errors.As(err, &pgerr) && pgerr.Code == "23505" {
			return "", ErrBusy
		}
		return "", err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO project_messages(project_id,actor_id,role,content) VALUES($1,$2,'user',$3)`, p.ID, u.ID, prompt); err != nil {
		return "", err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO audit_events(actor_id,project_id,action) VALUES($1,$2,'build.queued')`, u.ID, p.ID); err != nil {
		return "", err
	}
	return id, tx.Commit(ctx)
}

func (s *Service) providerKey(ctx context.Context, userID, provider string) (string, error) {
	if s.Vault == nil {
		return "", errors.New("secure credential storage is not configured")
	}
	var ciphertext []byte
	if err := s.Store.Pool.QueryRow(ctx, `SELECT key_ciphertext FROM llm_credentials WHERE user_id=$1 AND provider=$2`, userID, provider).Scan(&ciphertext); err != nil {
		return "", errors.New("save an API key for the selected provider first")
	}
	plain, err := s.Vault.Open(userID, provider, ciphertext)
	if err != nil {
		return "", errors.New("provider key cannot be decrypted; replace it in AI providers")
	}
	return string(plain), nil
}

func (s *Service) Models(ctx context.Context, u domain.User, provider string) ([]string, error) {
	key, err := s.providerKey(ctx, u.ID, provider)
	if err != nil {
		return nil, err
	}
	return s.Provider.Models(ctx, provider, key)
}

func (s *Service) SetAccess(ctx context.Context, u domain.User, slug, mode, password string) error {
	p, err := s.Store.Project(ctx, u, slug)
	if err != nil {
		return err
	}
	if p.Role != "admin" {
		return domain.ErrForbidden
	}
	if mode != "public" && mode != "password" {
		return errors.New("choose public or password access")
	}
	var hash *string
	if password != "" {
		if len(password) < 12 || len(password) > 72 {
			return errors.New("password must be 12–72 bytes")
		}
		data, err := bcrypt.GenerateFromPassword([]byte(password), 12)
		if err != nil {
			return err
		}
		value := string(data)
		hash = &value
	}
	tx, err := s.Store.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `UPDATE project_access_settings SET mode=$2,password_hash=coalesce($3,password_hash),credential_version=credential_version+1,updated_at=now() WHERE project_id=$1`, p.ID, mode, hash); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO audit_events(actor_id,project_id,action,metadata) VALUES($1,$2,'project.access_changed',jsonb_build_object('mode',$3::text))`, u.ID, p.ID, mode); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type databases struct {
	Production string `json:"production"`
	Preview    string `json:"preview"`
}

func randomHex() string {
	var b [24]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("secure random unavailable")
	}
	return hex.EncodeToString(b[:])
}

func (s *Service) databases(ctx context.Context, projectID string) (databases, error) {
	var encrypted []byte
	err := s.Store.Pool.QueryRow(ctx, `SELECT database_url_ciphertext FROM project_runtimes WHERE project_id=$1`, projectID).Scan(&encrypted)
	if errors.Is(err, pgx.ErrNoRows) {
		base := strings.ReplaceAll(projectID, "-", "")
		makeURL := func(prefix string) string {
			role := "sg1_" + prefix + base
			return (&url.URL{Scheme: "postgres", User: url.UserPassword(role, randomHex()), Host: s.DatabaseHost + ":5432", Path: "/" + role, RawQuery: "sslmode=disable"}).String()
		}
		db := databases{Production: makeURL("p"), Preview: makeURL("q")}
		raw, _ := json.Marshal(db)
		encrypted, err = s.Vault.Seal(projectID, "databases", string(raw))
		if err != nil {
			return db, err
		}
		if _, err = s.Store.Pool.Exec(ctx, `INSERT INTO project_runtimes(project_id,database_url_ciphertext) VALUES($1,$2) ON CONFLICT DO NOTHING`, projectID, encrypted); err != nil {
			return db, err
		}
		return s.databases(ctx, projectID)
	}
	if err != nil {
		return databases{}, err
	}
	raw, err := s.Vault.Open(projectID, "databases", encrypted)
	if err != nil {
		return databases{}, err
	}
	var db databases
	if err = json.Unmarshal(raw, &db); err != nil {
		return db, err
	}
	for _, rawURL := range []string{db.Production, db.Preview} {
		u, err := url.Parse(rawURL)
		if err != nil {
			return db, err
		}
		role := u.User.Username()
		password, _ := u.User.Password()
		identifier := pgx.Identifier{role}.Sanitize()
		var exists bool
		if err = s.Store.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_roles WHERE rolname=$1)`, role).Scan(&exists); err != nil {
			return db, err
		}
		// Role/password are generated locally, not model- or user-supplied SQL.
		if !exists {
			if _, err = s.Store.Pool.Exec(ctx, "CREATE ROLE "+identifier+" LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION PASSWORD '"+password+"'"); err != nil {
				return db, err
			}
		}
		if err = s.Store.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname=$1)`, role).Scan(&exists); err != nil {
			return db, err
		}
		if !exists {
			if _, err = s.Store.Pool.Exec(ctx, "CREATE DATABASE "+identifier+" OWNER "+identifier); err != nil {
				return db, err
			}
		}
		if _, err = s.Store.Pool.Exec(ctx, "REVOKE ALL ON DATABASE "+identifier+" FROM PUBLIC"); err != nil {
			return db, err
		}
	}
	if _, err = s.Store.Pool.Exec(ctx, `REVOKE CONNECT ON DATABASE sg1 FROM PUBLIC`); err != nil {
		return db, fmt.Errorf("could not isolate control-plane database")
	}
	return db, nil
}
