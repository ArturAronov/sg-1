CREATE TABLE project_files (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL REFERENCES projects(id),
    actor_id uuid NOT NULL REFERENCES users(id),
    name text NOT NULL,
    size bigint NOT NULL CHECK(size BETWEEN 0 AND 10485760),
    object_key text NOT NULL UNIQUE,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX project_files_project_idx ON project_files(project_id,created_at DESC);
