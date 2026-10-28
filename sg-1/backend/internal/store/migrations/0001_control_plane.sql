CREATE TABLE users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email text NOT NULL,
    name text NOT NULL,
    platform_admin boolean NOT NULL DEFAULT false,
    active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now()
);

-- Email is contact information, not the identity-linking key.
CREATE TABLE user_identities (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users(id),
    issuer text NOT NULL,
    subject text NOT NULL,
    UNIQUE (issuer, subject)
);

CREATE TABLE departments (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL,
    slug text NOT NULL UNIQUE,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE department_memberships (
    department_id uuid NOT NULL REFERENCES departments(id),
    user_id uuid NOT NULL REFERENCES users(id),
    role text NOT NULL CHECK (role IN ('read', 'write', 'admin')),
    PRIMARY KEY (department_id, user_id)
);
CREATE INDEX department_memberships_user_idx ON department_memberships(user_id);

CREATE TABLE projects (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    department_id uuid NOT NULL REFERENCES departments(id),
    created_by uuid NOT NULL REFERENCES users(id),
    name text NOT NULL CHECK (char_length(name) BETWEEN 2 AND 100),
    slug text NOT NULL CONSTRAINT projects_slug_key UNIQUE,
    description text NOT NULL DEFAULT '' CHECK (char_length(description) <= 1000),
    status text NOT NULL DEFAULT 'provisioning' CHECK (status IN ('provisioning', 'ready', 'failed', 'archiving', 'archived')),
    create_request_key text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT projects_slug_format CHECK (char_length(slug) BETWEEN 2 AND 48 AND slug ~ '^[a-z][a-z0-9]*(-[a-z0-9]+)*$'),
    CONSTRAINT projects_create_request_key UNIQUE (created_by, create_request_key)
);
CREATE INDEX projects_department_idx ON projects(department_id);

CREATE TABLE project_memberships (
    project_id uuid NOT NULL REFERENCES projects(id),
    user_id uuid NOT NULL REFERENCES users(id),
    role text NOT NULL CHECK (role IN ('read', 'write', 'admin')),
    PRIMARY KEY (project_id, user_id)
);
CREATE INDEX project_memberships_user_idx ON project_memberships(user_id);

CREATE TABLE project_access_settings (
    project_id uuid PRIMARY KEY REFERENCES projects(id),
    mode text NOT NULL DEFAULT 'password' CHECK (mode IN ('password', 'public')),
    password_hash text,
    credential_version integer NOT NULL DEFAULT 1,
    updated_at timestamptz NOT NULL DEFAULT now()
);
-- A null password hash means locked pending password setup, never public access.

CREATE TABLE provisioning_jobs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL UNIQUE REFERENCES projects(id),
    status text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'running', 'succeeded', 'failed')),
    attempts integer NOT NULL DEFAULT 0,
    lease_until timestamptz,
    last_error_code text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE outbox_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    kind text NOT NULL,
    aggregate_id uuid NOT NULL,
    payload jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    published_at timestamptz
);
CREATE INDEX outbox_pending_idx ON outbox_events(created_at) WHERE published_at IS NULL;

CREATE TABLE audit_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    actor_id uuid REFERENCES users(id),
    project_id uuid REFERENCES projects(id),
    action text NOT NULL,
    metadata jsonb NOT NULL DEFAULT '{}',
    created_at timestamptz NOT NULL DEFAULT now()
);
