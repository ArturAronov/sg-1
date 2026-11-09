package platform

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ArturAronov/sg-1/sg-1/backend/internal/domain"
	"github.com/ArturAronov/sg-1/sg-1/backend/internal/store"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func cancellationFixture(t *testing.T) (*Service, domain.User, domain.Project, string) {
	t.Helper()
	connection := os.Getenv("SG1_TEST_DATABASE_URL")
	if connection == "" {
		t.Skip("set SG1_TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, connection)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	schema := "sg1_cancel_test_" + randomHex()[:16]
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(ctx, "DROP SCHEMA "+quoted+" CASCADE"); err != nil {
			t.Error(err)
		}
	})
	config, err := pgxpool.ParseConfig(connection)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err = store.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	repo := &store.Store{Pool: pool}
	if err = repo.SeedDevelopment(ctx); err != nil {
		t.Fatal(err)
	}
	u, err := repo.User(ctx, store.DevUserID)
	if err != nil {
		t.Fatal(err)
	}
	p, _, err := repo.CreateProject(ctx, u, domain.CreateProject{DepartmentID: "00000000-0000-4000-8000-000000000002", Name: "Cancellation test", Slug: "cancel-test"}, "cancel-create-request")
	if err != nil {
		t.Fatal(err)
	}
	var deployment string
	if err = pool.QueryRow(ctx, `INSERT INTO deployments(project_id,image,container_name,summary,files) VALUES($1,'fixture:image','fixture-container','Working application','{}') RETURNING id::text`, p.ID).Scan(&deployment); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO project_runtimes(project_id,database_url_ciphertext,current_deployment_id) VALUES($1,'fixture',$2)`, p.ID, deployment); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE projects SET status='ready' WHERE id=$1`, p.ID); err != nil {
		t.Fatal(err)
	}
	return &Service{Store: repo}, u, p, deployment
}

func queueCancellationTest(t *testing.T, s *Service, u domain.User, p domain.Project, deployment string) string {
	t.Helper()
	id, err := s.QueueBuild(context.Background(), u, p.Slug, "", "", "Restore deployment "+deployment, randomHex(), deployment)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestQueuedBuildCancellationAuthorizationAndIdempotency(t *testing.T) {
	s, u, p, deployment := cancellationFixture(t)
	ctx := context.Background()
	id := queueCancellationTest(t, s, u, p, deployment)
	// A project reader must not be able to cancel another person's paid work.
	var readerID string
	if err := s.Store.Pool.QueryRow(ctx, `INSERT INTO users(email,name) VALUES('reader@test.local','Reader') RETURNING id::text`).Scan(&readerID); err != nil {
		t.Fatal(err)
	}
	reader, err := s.Store.User(ctx, readerID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Store.Pool.Exec(ctx, `INSERT INTO project_memberships(project_id,user_id,role) VALUES($1,$2,'read')`, p.ID, readerID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.CancelBuild(ctx, reader, p.Slug, id); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal("reader could cancel a build", err)
	}
	other, _, err := s.Store.CreateProject(ctx, u, domain.CreateProject{DepartmentID: p.DepartmentID, Name: "Other project", Slug: "other-project"}, "other-project-request")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.CancelBuild(ctx, u, other.Slug, id); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("cross-project build cancellation was allowed", err)
	}
	if _, err = s.Store.Pool.Exec(ctx, `UPDATE project_memberships SET role='write' WHERE project_id=$1 AND user_id=$2`, p.ID, readerID); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if status, err := s.CancelBuild(ctx, reader, p.Slug, id); err != nil || status != "cancelled" {
			t.Fatal("queued cancellation was not idempotent", status, err)
		}
	}
	if err := s.progress(ctx, id, "building", "Must not resurrect cancelled job"); !errors.Is(err, errBuildCancelled) {
		t.Fatal("cancelled build could advance to a new stage", err)
	}
	state, err := s.State(ctx, u, p.Slug)
	if err != nil || state.Builds[0].Status != "cancelled" || *state.CurrentDeploymentID != deployment || len(state.Messages) != 2 {
		t.Fatal("queued cancellation changed deployment or duplicated messages", state, err)
	}
	var audits int
	if err = s.Store.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE action='build.cancel_requested' AND actor_id=$1`, readerID).Scan(&audits); err != nil || audits != 1 {
		t.Fatal("cancellation audit was not exactly once", audits, err)
	}
	if worked, err := s.buildOne(ctx); err != nil || worked {
		t.Fatal("cancelled queued build was executed", worked, err)
	}
	queueCancellationTest(t, s, u, p, deployment) // Terminal cancellation releases the build slot.
}

func TestRunningBuildCancellationStopsProviderAndKeepsSlotUntilCleanup(t *testing.T) {
	for _, stage := range []string{"generating", "building", "validating"} {
		t.Run(stage, func(t *testing.T) {
			s, u, p, deployment := cancellationFixture(t)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			id := queueCancellationTest(t, s, u, p, deployment)
			if _, err := s.Store.Pool.Exec(ctx, `UPDATE build_jobs SET status=$2,lease_until=now()+interval '15 minutes' WHERE id=$1`, id, stage); err != nil {
				t.Fatal(err)
			}
			buildCtx, stopBuild := context.WithCancel(ctx)
			defer stopBuild()
			monitorCtx, stopMonitor := context.WithCancel(buildCtx)
			defer stopMonitor()
			done := s.monitorBuild(monitorCtx, id, stopBuild)
			started := make(chan struct{})
			providerDone := make(chan error, 1)
			provider := &ProviderClient{HTTP: &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
				close(started)
				<-r.Context().Done()
				return nil, r.Context().Err()
			})}}
			go func() {
				_, err := provider.Generate(buildCtx, "openai", "fixture-model", "synthetic-key", "Build", StarterFiles(), nil, "")
				providerDone <- err
			}()
			select {
			case <-started:
			case <-ctx.Done():
				t.Fatal("provider did not start")
			}
			for range 2 {
				if status, err := s.CancelBuild(ctx, u, p.Slug, id); err != nil || status != "cancelling" {
					t.Fatal("running cancellation was not idempotent", status, err)
				}
			}
			if err := s.progress(ctx, id, "validating", "Must not overwrite cancelling"); !errors.Is(err, errBuildCancelled) {
				t.Fatal("cancelling build could advance to a new stage", err)
			}
			if _, err := s.QueueBuild(ctx, u, p.Slug, "", "", "Another restore", randomHex(), deployment); !errors.Is(err, ErrBusy) {
				t.Fatal("cancelling job released build slot before cleanup", err)
			}
			select {
			case err := <-providerDone:
				if err == nil || buildCtx.Err() == nil {
					t.Fatal("provider was not cancelled")
				}
			case <-ctx.Done():
				t.Fatal("provider was not interrupted")
			}
			<-done
			if err := s.finishBuild(ctx, id, p.ID, buildCtx.Err()); err != nil {
				t.Fatal("cancellation reported as build failure", err)
			}
			if err := s.finishBuild(ctx, id, p.ID, errors.New("late worker error")); err != nil {
				t.Fatal("terminal cancellation was overwritten", err)
			}
			state, err := s.State(ctx, u, p.Slug)
			if err != nil || state.Builds[0].Status != "cancelled" || *state.CurrentDeploymentID != deployment || len(state.Messages) != 2 {
				t.Fatal("cancellation lost state", state, err)
			}
			queueCancellationTest(t, s, u, p, deployment)
		})
	}
}

func TestCancellationAndPublicationShareLock(t *testing.T) {
	t.Run("cancel wins", func(t *testing.T) {
		s, u, p, deployment := cancellationFixture(t)
		ctx := context.Background()
		id := queueCancellationTest(t, s, u, p, deployment)
		s.progress(ctx, id, "validating", "Checking health")
		if _, err := s.CancelBuild(ctx, u, p.Slug, id); err != nil {
			t.Fatal(err)
		}
		tx, err := s.Store.Pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if err := lockBuildForDeployment(ctx, tx, id); !errors.Is(err, errBuildCancelled) {
			t.Fatal("accepted cancellation did not block publication", err)
		}
	})
	t.Run("publication wins", func(t *testing.T) {
		s, u, p, deployment := cancellationFixture(t)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		id := queueCancellationTest(t, s, u, p, deployment)
		s.progress(ctx, id, "validating", "Checking health")
		tx, err := s.Store.Pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if err = lockBuildForDeployment(ctx, tx, id); err != nil {
			t.Fatal(err)
		}
		result := make(chan error, 1)
		go func() {
			_, err := s.CancelBuild(ctx, u, p.Slug, id)
			result <- err
		}()
		// A short deadline proves cancellation cannot pass the publication lock.
		select {
		case err := <-result:
			t.Fatal("cancellation did not wait for publication", err)
		case <-time.After(100 * time.Millisecond):
		}
		if _, err = tx.Exec(ctx, `UPDATE build_jobs SET status='succeeded' WHERE id=$1`, id); err != nil {
			t.Fatal(err)
		}
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		if err = <-result; !errors.Is(err, ErrBuildFinished) {
			t.Fatal("completed deployment reported as cancelled", err)
		}
	})
}

func TestAbandonedCancellationRecoversWithoutNewWork(t *testing.T) {
	s, u, p, deployment := cancellationFixture(t)
	ctx := context.Background()
	id := queueCancellationTest(t, s, u, p, deployment)
	if _, err := s.Store.Pool.Exec(ctx, `UPDATE build_jobs SET status='cancelling',lease_until=now()-interval '1 minute' WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if worked, err := s.buildOne(ctx); err != nil || worked {
			t.Fatal("abandoned cancellation was executed", worked, err)
		}
	}
	state, err := s.State(ctx, u, p.Slug)
	if err != nil || state.Builds[0].Status != "cancelled" || !strings.Contains(state.Builds[0].Message, "worker stopped") || len(state.Messages) != 2 || state.Messages[1].Content != state.Builds[0].Message {
		t.Fatal("abandoned cancellation did not recover", state, err)
	}
}
