CREATE TABLE project_runtimes (
    project_id uuid PRIMARY KEY REFERENCES projects(id),
    database_url_ciphertext bytea NOT NULL,
    current_deployment_id uuid,
    last_error text,
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE deployments (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL REFERENCES projects(id),
    image text NOT NULL,
    container_name text NOT NULL,
    summary text NOT NULL,
    files jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
ALTER TABLE project_runtimes ADD FOREIGN KEY (current_deployment_id) REFERENCES deployments(id);

CREATE TABLE build_jobs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL REFERENCES projects(id),
    actor_id uuid NOT NULL REFERENCES users(id),
    kind text NOT NULL DEFAULT 'build' CHECK(kind IN ('build','rollback')),
    provider text CHECK(provider IN ('openai','anthropic','glm','kimi')),
    model text,
    target_deployment_id uuid REFERENCES deployments(id),
    prompt text NOT NULL,
    request_key text NOT NULL,
    status text NOT NULL DEFAULT 'queued' CHECK(status IN ('queued','generating','building','validating','succeeded','failed')),
    message text NOT NULL DEFAULT 'Waiting for a worker',
    deployment_id uuid REFERENCES deployments(id),
    lease_until timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(project_id,actor_id,request_key)
);
CREATE UNIQUE INDEX one_active_build_per_project ON build_jobs(project_id) WHERE status NOT IN ('succeeded','failed');
CREATE INDEX build_jobs_project_idx ON build_jobs(project_id,created_at DESC);

CREATE TABLE project_messages (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL REFERENCES projects(id),
    actor_id uuid REFERENCES users(id),
    role text NOT NULL CHECK(role IN ('user','assistant','system')),
    content text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX project_messages_project_idx ON project_messages(project_id,created_at);

CREATE TABLE app_password_attempts (
    project_id uuid PRIMARY KEY REFERENCES projects(id),
    attempts integer NOT NULL DEFAULT 0,
    window_start timestamptz NOT NULL DEFAULT now()
);
