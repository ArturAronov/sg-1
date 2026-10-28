package platform

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

//go:embed starter/*
var starter embed.FS

func StarterFiles() map[string]string {
	result := map[string]string{}
	for _, name := range []string{"Dockerfile", "server.js"} {
		content, _ := starter.ReadFile("starter/" + name)
		result[name] = string(content)
	}
	return result
}

func (s *Service) RunWorker(ctx context.Context, logger *slog.Logger) error {
	if s.Vault == nil || s.Docker == nil {
		return errors.New("worker needs Docker and credential encryption")
	}
	lastReconcile := time.Time{}
	for ctx.Err() == nil {
		if time.Since(lastReconcile) > 15*time.Second {
			s.reconcileNetworks(ctx)
			lastReconcile = time.Now()
		}
		worked, err := s.provisionOne(ctx)
		if err != nil {
			logger.Error("project provisioning attempt failed")
		}
		if !worked {
			worked, err = s.buildOne(ctx)
			if err != nil {
				logger.Error("application build attempt failed")
			}
		}
		if !worked {
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(2 * time.Second):
			}
		}
	}
	return nil
}

func (s *Service) reconcileNetworks(ctx context.Context) {
	rows, err := s.Store.Pool.Query(ctx, `SELECT project_id::text FROM project_runtimes WHERE current_deployment_id IS NOT NULL`)
	if err != nil {
		return
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	for _, id := range ids {
		repair, cancel := context.WithTimeout(ctx, 15*time.Second)
		_, _ = s.Docker.PrepareNetwork(repair, id)
		cancel()
	}
	_, _ = s.Store.Pool.Exec(ctx, `WITH exhausted AS(UPDATE provisioning_jobs SET status='failed',last_error_code='worker_stopped',updated_at=now() WHERE status='running' AND lease_until<now() AND attempts>=3 RETURNING project_id) UPDATE projects SET status='failed' WHERE id IN(SELECT project_id FROM exhausted)`)
}

func (s *Service) provisionOne(parent context.Context) (bool, error) {
	ctx, cancel := context.WithTimeout(parent, 12*time.Minute)
	defer cancel()
	var projectID string
	err := s.Store.Pool.QueryRow(ctx, `UPDATE provisioning_jobs SET status='running',attempts=attempts+1,lease_until=now()+interval '15 minutes',updated_at=now()
	 WHERE id=(SELECT id FROM provisioning_jobs WHERE (status='queued' OR (status='running' AND lease_until<now())) AND attempts<3 ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT 1) RETURNING project_id::text`).Scan(&projectID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	err = s.provision(ctx, projectID)
	finish, c := context.WithTimeout(context.Background(), 10*time.Second)
	defer c()
	if err != nil {
		_, _ = s.Store.Pool.Exec(finish, `UPDATE provisioning_jobs SET status='failed',last_error_code='provisioning_failed',updated_at=now() WHERE project_id=$1`, projectID)
		_, _ = s.Store.Pool.Exec(finish, `UPDATE projects SET status='failed' WHERE id=$1`, projectID)
		_, _ = s.Store.Pool.Exec(finish, `UPDATE project_runtimes SET last_error='Provisioning failed. Check the local worker and Docker daemon.' WHERE project_id=$1`, projectID)
		return true, err
	}
	_, err = s.Store.Pool.Exec(finish, `UPDATE provisioning_jobs SET status='succeeded',lease_until=NULL,updated_at=now() WHERE project_id=$1`, projectID)
	return true, err
}

func (s *Service) provision(ctx context.Context, projectID string) error {
	var deployed bool
	if err := s.Store.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM project_runtimes WHERE project_id=$1 AND current_deployment_id IS NOT NULL)`, projectID).Scan(&deployed); err != nil {
		return err
	}
	if deployed {
		return nil
	}
	db, err := s.databases(ctx, projectID)
	if err != nil {
		return err
	}
	network, err := s.Docker.PrepareNetwork(ctx, projectID)
	if err != nil {
		return err
	}
	files := StarterFiles()
	image := "sg1-starter:local"
	if _, err = s.Docker.Build(ctx, image, files); err != nil {
		return err
	}
	return s.deploy(ctx, projectID, image, "Project container ready. Describe the application you want to build.", files, network, db, "")
}

func (s *Service) deploy(ctx context.Context, projectID, image, summary string, files map[string]string, network string, db databases, jobID string) error {
	var id string
	if err := s.Store.Pool.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&id); err != nil {
		return err
	}
	name := containerName(projectID, id)
	// Validate with a separate preview database. The active application and its
	// production database stay untouched while a candidate is checked.
	if err := s.Docker.Start(ctx, image, name, network, db.Preview); err != nil {
		return err
	}
	defer func() {
		cleanup, c := context.WithTimeout(context.Background(), 10*time.Second)
		defer c()
		s.Docker.Remove(cleanup, name)
	}()
	if err := s.Docker.Healthy(ctx, name); err != nil {
		return err
	}
	if err := s.probe(ctx, projectID, name); err != nil {
		return err
	}
	s.Docker.Remove(ctx, name)
	production := name + "-live"
	if err := s.Docker.Start(ctx, image, production, network, db.Production); err != nil {
		return err
	}
	published := false
	defer func() {
		if !published {
			cleanup, c := context.WithTimeout(context.Background(), 10*time.Second)
			defer c()
			s.Docker.Remove(cleanup, production)
		}
	}()
	if err := s.Docker.Healthy(ctx, production); err != nil {
		s.Docker.Remove(ctx, production)
		return err
	}
	if err := s.probe(ctx, projectID, production); err != nil {
		return err
	}
	raw, err := json.Marshal(files)
	if err != nil {
		return err
	}
	tx, err := s.Store.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if jobID != "" {
		var allowed bool
		err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM build_jobs b JOIN users u ON u.id=b.actor_id JOIN projects p ON p.id=b.project_id WHERE b.id=$1 AND u.active AND (u.platform_admin OR EXISTS(SELECT 1 FROM department_memberships dm WHERE dm.department_id=p.department_id AND dm.user_id=u.id AND dm.role='admin') OR EXISTS(SELECT 1 FROM project_memberships pm WHERE pm.project_id=p.id AND pm.user_id=u.id AND pm.role IN('write','admin'))))`, jobID).Scan(&allowed)
		if err != nil {
			return err
		}
		if !allowed {
			return errors.New("build permission was revoked before deployment")
		}
	}
	var previous *string
	if err = tx.QueryRow(ctx, `SELECT d.container_name FROM project_runtimes r LEFT JOIN deployments d ON d.id=r.current_deployment_id WHERE r.project_id=$1 FOR UPDATE OF r`, projectID).Scan(&previous); err != nil {
		s.Docker.Remove(ctx, production)
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO deployments(id,project_id,image,container_name,summary,files) VALUES($1,$2,$3,$4,$5,$6)`, id, projectID, image, production, summary, raw); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE project_runtimes SET current_deployment_id=$2,last_error=NULL,updated_at=now() WHERE project_id=$1`, projectID, id); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE projects SET status='ready' WHERE id=$1`, projectID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO project_messages(project_id,role,content) VALUES($1,'assistant',$2)`, projectID, summary); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE outbox_events SET published_at=now() WHERE aggregate_id=$1 AND kind='project.provision' AND published_at IS NULL`, projectID); err != nil {
		return err
	}
	if jobID != "" {
		if _, err = tx.Exec(ctx, `UPDATE build_jobs SET status='succeeded',message='Healthy application deployed',deployment_id=$2,updated_at=now(),lease_until=NULL WHERE id=$1`, jobID, id); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(ctx, `INSERT INTO audit_events(actor_id,project_id,action,metadata) SELECT coalesce((SELECT actor_id FROM build_jobs WHERE id::text=$3),created_by),id,'deployment.succeeded',jsonb_build_object('deployment_id',$2::text) FROM projects WHERE id=$1`, projectID, id, jobID); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		s.Docker.Remove(ctx, production)
		return err
	}
	published = true
	if previous != nil {
		s.Docker.Stop(ctx, *previous)
	}
	return nil
}

func (s *Service) progress(ctx context.Context, id, status, message string) {
	_, _ = s.Store.Pool.Exec(ctx, `UPDATE build_jobs SET status=$2,message=$3,updated_at=now() WHERE id=$1`, id, status, message)
}

func (s *Service) buildOne(parent context.Context) (bool, error) {
	ctx, cancel := context.WithTimeout(parent, 12*time.Minute)
	defer cancel()
	// A crashed/timed-out attempt is failed, not silently retried with new model
	// charges. The user can explicitly retry it from chat.
	_, _ = s.Store.Pool.Exec(ctx, `UPDATE build_jobs SET status='failed',message='Worker stopped before completion. The previous application is unchanged; retry explicitly.',updated_at=now() WHERE status NOT IN ('queued','succeeded','failed') AND lease_until<now()`)
	var id, projectID, userID, provider, model, prompt, kind string
	var target *string
	err := s.Store.Pool.QueryRow(ctx, `UPDATE build_jobs SET status='generating',message='Preparing application source',lease_until=now()+interval '15 minutes',updated_at=now()
	 WHERE id=(SELECT id FROM build_jobs WHERE status='queued' ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT 1)
	 RETURNING id::text,project_id::text,actor_id::text,coalesce(provider,''),coalesce(model,''),prompt,kind,target_deployment_id::text`).Scan(&id, &projectID, &userID, &provider, &model, &prompt, &kind, &target)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	err = s.executeBuild(ctx, id, projectID, userID, provider, model, prompt, kind, target)
	if err != nil {
		finish, c := context.WithTimeout(context.Background(), 10*time.Second)
		defer c()
		message := err.Error()
		if len(message) > 1000 {
			message = message[:1000]
		}
		s.progress(finish, id, "failed", message)
		_, _ = s.Store.Pool.Exec(finish, `INSERT INTO project_messages(project_id,role,content) VALUES($1,'system',$2)`, projectID, "Build failed; the previous deployment is unchanged. "+message)
	}
	return true, err
}

func (s *Service) executeBuild(ctx context.Context, id, projectID, userID, provider, model, prompt, kind string, target *string) error {
	u, err := s.Store.User(ctx, userID)
	if err != nil {
		return errors.New("requesting user is no longer active")
	}
	var slug string
	if err = s.Store.Pool.QueryRow(ctx, `SELECT slug FROM projects WHERE id=$1`, projectID).Scan(&slug); err != nil {
		return errors.New("project is unavailable")
	}
	p, err := s.Store.Project(ctx, u, slug)
	if err != nil || p.Role == "read" {
		return errors.New("build permission was revoked")
	}
	db, err := s.databases(ctx, projectID)
	if err != nil {
		return errors.New("project database is unavailable")
	}
	network, err := s.Docker.PrepareNetwork(ctx, projectID)
	if err != nil {
		return err
	}
	var raw []byte
	var image, summary string
	if kind == "rollback" {
		if target == nil {
			return errors.New("rollback deployment is missing")
		}
		if err = s.Store.Pool.QueryRow(ctx, `SELECT image,files FROM deployments WHERE id=$1 AND project_id=$2`, *target, projectID).Scan(&image, &raw); err != nil {
			return errors.New("rollback image is unavailable")
		}
		files := map[string]string{}
		if json.Unmarshal(raw, &files) != nil {
			return errors.New("invalid deployment source")
		}
		s.progress(ctx, id, "validating", "Validating rollback image")
		return s.deploy(ctx, projectID, image, "Restored a previous application version. Database records were preserved.", files, network, db, id)
	}
	key, err := s.providerKey(ctx, userID, provider)
	if err != nil {
		return err
	}
	state, err := s.State(ctx, u, slug)
	if err != nil {
		return err
	}
	history := state.Messages
	if len(history) > 8 {
		history = history[len(history)-8:]
	}
	files := map[string]string{}
	if err = s.Store.Pool.QueryRow(ctx, `SELECT d.files FROM project_runtimes r JOIN deployments d ON d.id=r.current_deployment_id WHERE r.project_id=$1`, projectID).Scan(&raw); err != nil {
		return errors.New("current application source is unavailable")
	}
	if json.Unmarshal(raw, &files) != nil {
		return errors.New("invalid application source")
	}
	repair := ""
	for attempt := 1; attempt <= 3; attempt++ {
		s.progress(ctx, id, "generating", "Generating application source (attempt "+string(rune('0'+attempt))+" of 3)")
		generated, err := s.Provider.Generate(ctx, provider, model, key, prompt, files, history, repair)
		if err != nil {
			repair = err.Error()
			continue
		}
		image = "sg1-project:" + id + "-" + string(rune('0'+attempt))
		summary = generated.Summary
		s.progress(ctx, id, "building", "Building the Docker image")
		output, err := s.Docker.Build(ctx, image, generated.Files)
		if err != nil {
			repair = strings.ReplaceAll(output, key, "[redacted]")
			if len(repair) > 12000 {
				repair = repair[len(repair)-12000:]
			}
			continue
		}
		s.progress(ctx, id, "validating", "Checking candidate startup before deployment")
		if err = s.deploy(ctx, projectID, image, summary, generated.Files, network, db, id); err != nil {
			repair = err.Error()
			continue
		}
		return nil
	}
	if len(repair) > 600 {
		repair = repair[len(repair)-600:]
	}
	return errors.New("Validation failed after 3 attempts. " + strings.ReplaceAll(repair, key, "[redacted]"))
}
