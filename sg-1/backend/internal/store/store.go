package store

import (
	"context"
	"errors"
	"strings"

	"github.com/ArturAronov/sg-1/sg-1/backend/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const DevUserID = "00000000-0000-4000-8000-000000000001"

type Store struct{ Pool *pgxpool.Pool }

func (s *Store) Ping(ctx context.Context) error {
	var applied int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM schema_migrations WHERE name IN ('0001_control_plane.sql','0002_llm_credentials.sql','0003_runtime.sql','0004_project_files.sql')`).Scan(&applied); err != nil {
		return err
	}
	if applied != 4 {
		return errors.New("control-plane migrations have not been applied")
	}
	return nil
}

func (s *Store) User(ctx context.Context, id string) (domain.User, error) {
	var u domain.User
	err := s.Pool.QueryRow(ctx, `SELECT id::text,email,name,platform_admin FROM users WHERE id=$1 AND active`, id).Scan(&u.ID, &u.Email, &u.Name, &u.PlatformAdmin)
	if errors.Is(err, pgx.ErrNoRows) {
		return u, domain.ErrNotFound
	}
	return u, err
}

func (s *Store) Departments(ctx context.Context, u domain.User) ([]domain.Department, error) {
	rows, err := s.Pool.Query(ctx, `SELECT d.id::text,d.name,d.slug,CASE WHEN $2 THEN 'admin' ELSE m.role END
		FROM departments d LEFT JOIN department_memberships m ON m.department_id=d.id AND m.user_id=$1
		WHERE $2 OR m.user_id IS NOT NULL ORDER BY d.name`, u.ID, u.PlatformAdmin)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []domain.Department{}
	for rows.Next() {
		var d domain.Department
		if err := rows.Scan(&d.ID, &d.Name, &d.Slug, &d.Role); err != nil {
			return nil, err
		}
		result = append(result, d)
	}
	return result, rows.Err()
}

const projectSelect = `SELECT p.id::text,p.department_id::text,d.name,p.name,p.slug,p.description,p.status,a.mode,
	CASE WHEN $2 OR dm.role='admin' THEN 'admin' ELSE pm.role END,p.created_at
	FROM projects p JOIN departments d ON d.id=p.department_id
	JOIN project_access_settings a ON a.project_id=p.id
	LEFT JOIN department_memberships dm ON dm.department_id=p.department_id AND dm.user_id=$1
	LEFT JOIN project_memberships pm ON pm.project_id=p.id AND pm.user_id=$1
	WHERE ($2 OR dm.role='admin' OR pm.user_id IS NOT NULL)`

func scanProject(row pgx.Row) (domain.Project, error) {
	var p domain.Project
	err := row.Scan(&p.ID, &p.DepartmentID, &p.DepartmentName, &p.Name, &p.Slug, &p.Description, &p.Status, &p.AccessMode, &p.Role, &p.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, domain.ErrNotFound
	}
	return p, err
}

func (s *Store) Projects(ctx context.Context, u domain.User) ([]domain.Project, error) {
	rows, err := s.Pool.Query(ctx, projectSelect+` ORDER BY p.created_at DESC`, u.ID, u.PlatformAdmin)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []domain.Project{}
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

func (s *Store) Project(ctx context.Context, u domain.User, slug string) (domain.Project, error) {
	return scanProject(s.Pool.QueryRow(ctx, projectSelect+` AND p.slug=$3`, u.ID, u.PlatformAdmin, slug))
}

func (s *Store) existingRequest(ctx context.Context, u domain.User, in domain.CreateProject, key string) (domain.Project, bool, error) {
	p, err := scanProject(s.Pool.QueryRow(ctx, projectSelect+` AND p.created_by=$1 AND p.create_request_key=$3`, u.ID, u.PlatformAdmin, key))
	if errors.Is(err, domain.ErrNotFound) {
		return p, false, nil
	}
	if err != nil {
		return p, false, err
	}
	if p.DepartmentID != in.DepartmentID || p.Name != in.Name || p.Slug != in.Slug || p.Description != in.Description {
		return p, false, domain.ErrIdempotencyConflict
	}
	return p, true, nil
}

func (s *Store) CreateProject(ctx context.Context, u domain.User, in domain.CreateProject, key string) (domain.Project, bool, error) {
	if err := in.Validate(); err != nil {
		return domain.Project{}, false, err
	}
	in.DepartmentID = strings.ToLower(in.DepartmentID)
	if p, exists, err := s.existingRequest(ctx, u, in, key); err != nil || exists {
		return p, false, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return domain.Project{}, false, err
	}
	defer tx.Rollback(ctx)
	var permitted bool
	err = tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM departments d WHERE d.id=$1 AND ($3 OR EXISTS (
		SELECT 1 FROM department_memberships WHERE department_id=d.id AND user_id=$2 AND role='admin')))`, in.DepartmentID, u.ID, u.PlatformAdmin).Scan(&permitted)
	if err != nil {
		return domain.Project{}, false, err
	}
	if !permitted {
		return domain.Project{}, false, domain.ErrForbidden
	}
	var id string
	err = tx.QueryRow(ctx, `INSERT INTO projects (department_id,created_by,name,slug,description,create_request_key)
		VALUES ($1,$2,$3,$4,$5,$6) RETURNING id::text`, in.DepartmentID, u.ID, in.Name, in.Slug, in.Description, key).Scan(&id)
	if err != nil {
		_ = tx.Rollback(ctx)
		var pgerr *pgconn.PgError
		if errors.As(err, &pgerr) && pgerr.Code == "23505" {
			// A simultaneous retry can conflict on either unique index. Resolve
			// idempotency first, then report an actual slug collision.
			if p, exists, retryErr := s.existingRequest(ctx, u, in, key); retryErr != nil || exists {
				return p, false, retryErr
			}
			return domain.Project{}, false, domain.ErrSlugTaken
		}
		return domain.Project{}, false, err
	}
	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO project_access_settings (project_id) VALUES ($1)`, []any{id}},
		{`INSERT INTO project_memberships (project_id,user_id,role) VALUES ($1,$2,'admin')`, []any{id, u.ID}},
		{`INSERT INTO provisioning_jobs (project_id) VALUES ($1)`, []any{id}},
		{`INSERT INTO outbox_events (kind,aggregate_id,payload) VALUES ('project.provision',$1::uuid,jsonb_build_object('project_id',($1::uuid)::text))`, []any{id}},
		{`INSERT INTO audit_events (actor_id,project_id,action) VALUES ($2,$1,'project.created')`, []any{id, u.ID}},
	}
	for _, statement := range statements {
		if _, err = tx.Exec(ctx, statement.query, statement.args...); err != nil {
			return domain.Project{}, false, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.Project{}, false, err
	}
	p, err := s.Project(ctx, u, in.Slug)
	return p, true, err
}

// SeedDevelopment must only be called after an explicit development-mode check.
func (s *Store) SeedDevelopment(ctx context.Context) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `INSERT INTO users (id,email,name,platform_admin) VALUES ($1,'developer@localhost','Local developer',true) ON CONFLICT (id) DO NOTHING`, DevUserID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO departments (id,name,slug) VALUES ('00000000-0000-4000-8000-000000000002','Property management','property-management') ON CONFLICT (id) DO NOTHING`); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO department_memberships (department_id,user_id,role) VALUES ('00000000-0000-4000-8000-000000000002',$1,'admin') ON CONFLICT DO NOTHING`, DevUserID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
