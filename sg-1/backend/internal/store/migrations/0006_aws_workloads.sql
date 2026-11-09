CREATE TABLE aws_workloads (
    name text PRIMARY KEY,
    project_id uuid NOT NULL REFERENCES projects(id),
    service_arn text NOT NULL DEFAULT '',
    discovery_id text NOT NULL DEFAULT '',
    task_definition text NOT NULL DEFAULT '',
    secret_arn text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX aws_workloads_project_idx ON aws_workloads(project_id);
