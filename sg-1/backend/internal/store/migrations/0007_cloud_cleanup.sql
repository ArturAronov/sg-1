ALTER TABLE aws_workloads ADD COLUMN cleanup_requested boolean NOT NULL DEFAULT false;
CREATE TABLE aws_builds (
    id text PRIMARY KEY,
    project_id uuid NOT NULL REFERENCES projects(id),
    cancel_requested boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX aws_builds_project_idx ON aws_builds(project_id);
