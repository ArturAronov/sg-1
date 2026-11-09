ALTER TABLE build_jobs DROP CONSTRAINT build_jobs_status_check;
ALTER TABLE build_jobs ADD CONSTRAINT build_jobs_status_check
    CHECK(status IN ('queued','generating','building','validating','cancelling','cancelled','succeeded','failed'));

DROP INDEX one_active_build_per_project;
CREATE UNIQUE INDEX one_active_build_per_project ON build_jobs(project_id)
    WHERE status NOT IN ('succeeded','failed','cancelled');
