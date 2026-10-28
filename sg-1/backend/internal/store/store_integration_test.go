package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"sync"
	"testing"

	"github.com/ArturAronov/sg-1/sg-1/backend/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func integrationStore(t *testing.T) *Store {
	t.Helper()
	url := os.Getenv("SG1_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set SG1_TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	random := make([]byte, 8)
	if _, err := rand.Read(random); err != nil {
		t.Fatal(err)
	}
	schema := "sg1_test_" + hex.EncodeToString(random)
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(ctx, "DROP SCHEMA "+quoted+" CASCADE"); err != nil {
			t.Error(err)
		}
	})
	c, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	c.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, pool); err != nil {
		t.Fatal("migration repeat", err)
	}
	s := &Store{Pool: pool}
	if err := s.SeedDevelopment(ctx); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestIntegrationProjectCreation(t *testing.T) {
	s := integrationStore(t)
	ctx := context.Background()
	u, err := s.User(ctx, DevUserID)
	if err != nil {
		t.Fatal(err)
	}
	in := domain.CreateProject{DepartmentID: "00000000-0000-4000-8000-000000000002", Name: "Maintenance tracker", Slug: "maintenance-tracker"}
	p, created, err := s.CreateProject(ctx, u, in, "first-request-key")
	if err != nil || !created {
		t.Fatal(created, err)
	}
	if p.AccessMode != "password" || p.Status != "provisioning" || p.Role != "admin" {
		t.Fatalf("unsafe defaults: %+v", p)
	}
	for _, table := range []string{"projects", "project_memberships", "project_access_settings", "provisioning_jobs", "outbox_events", "audit_events"} {
		var count int
		if err := s.Pool.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&count); err != nil || count != 1 {
			t.Fatalf("%s: count=%d err=%v", table, count, err)
		}
	}
	retried, created, err := s.CreateProject(ctx, u, in, "first-request-key")
	if err != nil || created || retried.ID != p.ID {
		t.Fatal("retry was not idempotent", err)
	}
	if _, _, err := s.CreateProject(ctx, u, in, "different-request-key"); !errors.Is(err, domain.ErrSlugTaken) {
		t.Fatal("duplicate slug accepted", err)
	}
	in.Name = "Changed request"
	if _, _, err := s.CreateProject(ctx, u, in, "first-request-key"); !errors.Is(err, domain.ErrIdempotencyConflict) {
		t.Fatal("changed retry accepted", err)
	}
}

func TestIntegrationPermissions(t *testing.T) {
	s := integrationStore(t)
	ctx := context.Background()
	admin, _ := s.User(ctx, DevUserID)
	in := domain.CreateProject{DepartmentID: "00000000-0000-4000-8000-000000000002", Name: "Private project", Slug: "private-project"}
	p, _, err := s.CreateProject(ctx, admin, in, "private-project-key")
	if err != nil {
		t.Fatal(err)
	}
	var id string
	if err := s.Pool.QueryRow(ctx, `INSERT INTO users(email,name) VALUES ('other@localhost','Other user') RETURNING id::text`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	u, err := s.User(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	projects, err := s.Projects(ctx, u)
	if err != nil || len(projects) != 0 {
		t.Fatal("cross-project listing leak", err)
	}
	if _, err := s.Project(ctx, u, p.Slug); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("project metadata leak", err)
	}
	if _, err := s.Pool.Exec(ctx, `INSERT INTO department_memberships(department_id,user_id,role) VALUES($1,$2,'read')`, in.DepartmentID, id); err != nil {
		t.Fatal(err)
	}
	projects, err = s.Projects(ctx, u)
	if err != nil || len(projects) != 0 {
		t.Fatal("department membership granted implicit project access", err)
	}
	in.Slug = "unauthorized-project"
	if _, _, err := s.CreateProject(ctx, u, in, "unauthorized-request"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal("reader created project", err)
	}
	if _, err := s.Pool.Exec(ctx, `INSERT INTO project_memberships(project_id,user_id,role) VALUES($1,$2,'read')`, p.ID, id); err != nil {
		t.Fatal(err)
	}
	visible, err := s.Project(ctx, u, p.Slug)
	if err != nil || visible.Role != "read" {
		t.Fatal("explicit read access failed", err)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE department_memberships SET role='admin' WHERE department_id=$1 AND user_id=$2`, in.DepartmentID, id); err != nil {
		t.Fatal(err)
	}
	visible, err = s.Project(ctx, u, p.Slug)
	if err != nil || visible.Role != "admin" {
		t.Fatal("department-admin authority failed", err)
	}
	var otherDepartmentID string
	if err := s.Pool.QueryRow(ctx, `INSERT INTO departments(name,slug) VALUES('Finance','finance') RETURNING id::text`).Scan(&otherDepartmentID); err != nil {
		t.Fatal(err)
	}
	in.DepartmentID = otherDepartmentID
	in.Slug = "finance-private"
	other, _, err := s.CreateProject(ctx, admin, in, "other-department-key")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Project(ctx, u, other.Slug); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("department-admin cross-department leak", err)
	}
	in.Slug = "unauthorized-finance"
	if _, _, err := s.CreateProject(ctx, u, in, "unauthorized-finance-key"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal("department admin created a project in another department", err)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE users SET active=false WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := s.User(ctx, id); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("disabled user remained authenticatable", err)
	}
}

func TestIntegrationConcurrentCreation(t *testing.T) {
	s := integrationStore(t)
	ctx := context.Background()
	u, _ := s.User(ctx, DevUserID)
	in := domain.CreateProject{DepartmentID: "00000000-0000-4000-8000-000000000002", Name: "Concurrent project", Slug: "concurrent-project"}
	var wg sync.WaitGroup
	var mu sync.Mutex
	createdCount := 0
	for range 6 {
		wg.Go(func() {
			_, created, err := s.CreateProject(ctx, u, in, "same-concurrent-request")
			if err != nil {
				t.Error(err)
				return
			}
			if created {
				mu.Lock()
				createdCount++
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	if createdCount != 1 {
		t.Fatalf("created %d projects, want 1", createdCount)
	}
	var jobs int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM provisioning_jobs`).Scan(&jobs); err != nil || jobs != 1 {
		t.Fatal("duplicated provisioning", jobs, err)
	}
}
