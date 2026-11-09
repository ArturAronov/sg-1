package platform

import (
	"context"
	"errors"
	"time"

	"github.com/ArturAronov/sg-1/sg-1/backend/internal/domain"
	"github.com/jackc/pgx/v5"
)

var ErrBuildFinished = errors.New("this build has already finished; refresh to see its result")
var errBuildCancelled = errors.New("build cancellation requested")

const cancelledMessage = "Build cancelled. The previous deployment is unchanged. Provider usage already incurred may still be charged."

func lockBuildForDeployment(ctx context.Context, tx pgx.Tx, id string) error {
	var status string
	if err := tx.QueryRow(ctx, `SELECT status FROM build_jobs WHERE id=$1 FOR UPDATE`, id).Scan(&status); err != nil {
		return err
	}
	if status == "cancelling" || status == "cancelled" {
		return errBuildCancelled
	}
	if status != "validating" {
		return errors.New("build is no longer eligible for deployment")
	}
	return nil
}

// CancelBuild locks the same job row as publication. Once cancellation is
// accepted, that job cannot publish. Running jobs retain the project build slot
// until the worker has stopped execution and cleaned up its candidates.
func (s *Service) CancelBuild(ctx context.Context, u domain.User, slug, id string) (string, error) {
	p, err := s.Store.Project(ctx, u, slug)
	if err != nil {
		return "", err
	}
	if p.Role == "read" {
		return "", domain.ErrForbidden
	}
	tx, err := s.Store.Pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	var status string
	err = tx.QueryRow(ctx, `SELECT status FROM build_jobs WHERE id::text=$1 AND project_id=$2 FOR UPDATE`, id, p.ID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", domain.ErrNotFound
	}
	if err != nil {
		return "", err
	}
	switch status {
	case "cancelled", "cancelling":
		return status, nil
	case "succeeded", "failed":
		return "", ErrBuildFinished
	}
	message := "Stopping build and cleaning up candidates…"
	if status == "queued" {
		status, message = "cancelled", cancelledMessage
		if _, err = tx.Exec(ctx, `INSERT INTO project_messages(project_id,role,content) VALUES($1,'system',$2)`, p.ID, message); err != nil {
			return "", err
		}
	} else {
		status = "cancelling"
	}
	if _, err = tx.Exec(ctx, `UPDATE build_jobs SET status=$2,message=$3,updated_at=now() WHERE id::text=$1`, id, status, message); err != nil {
		return "", err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO audit_events(actor_id,project_id,action,metadata) VALUES($1,$2,'build.cancel_requested',jsonb_build_object('build_id',$3::text))`, u.ID, p.ID, id); err != nil {
		return "", err
	}
	return status, tx.Commit(ctx)
}

// Observe durable requests from any API process, not just this worker's process.
// A failed status check stops execution rather than risking an unobserved cancel.
func (s *Service) monitorBuild(ctx context.Context, id string, cancel context.CancelFunc) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			var status string
			checkCtx, stopCheck := context.WithTimeout(ctx, 2*time.Second)
			err := s.Store.Pool.QueryRow(checkCtx, `SELECT status FROM build_jobs WHERE id=$1`, id).Scan(&status)
			stopCheck()
			if err != nil || status == "cancelling" || status == "cancelled" || status == "failed" {
				cancel()
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return done
}

func (s *Service) finishBuild(ctx context.Context, id, projectID string, buildErr error) error {
	if s.Cloud != nil {
		var pending bool
		if err := s.Store.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM aws_workloads WHERE project_id=$1 AND cleanup_requested) OR EXISTS(SELECT 1 FROM aws_builds WHERE project_id=$1 AND cancel_requested)`, projectID).Scan(&pending); err != nil {
			return err
		}
		if pending {
			_, err := s.Store.Pool.Exec(ctx, `UPDATE build_jobs SET message='Cloud cleanup pending; the previous deployment is unchanged.',lease_until=now()+interval '5 minutes',updated_at=now() WHERE id=$1 AND status NOT IN ('succeeded','failed','cancelled')`, id)
			return err
		}
	}
	tx, err := s.Store.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var status string
	if err = tx.QueryRow(ctx, `SELECT status FROM build_jobs WHERE id=$1 FOR UPDATE`, id).Scan(&status); err != nil {
		return err
	}
	if status == "succeeded" || status == "cancelled" || status == "failed" {
		return nil
	}
	message := cancelledMessage
	if status == "cancelling" {
		status = "cancelled"
		buildErr = nil
	} else {
		status = "failed"
		if buildErr == nil {
			buildErr = errors.New("build stopped before deployment")
		}
		message = buildErr.Error()
		if len(message) > 1000 {
			message = message[:1000]
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE build_jobs SET status=$2,message=$3,updated_at=now(),lease_until=NULL WHERE id=$1`, id, status, message); err != nil {
		return err
	}
	if status == "failed" {
		message = "Build failed; the previous deployment is unchanged. " + message
	}
	if _, err = tx.Exec(ctx, `INSERT INTO project_messages(project_id,role,content) VALUES($1,'system',$2)`, projectID, message); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	return buildErr
}
