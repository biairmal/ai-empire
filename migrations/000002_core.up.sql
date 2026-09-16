-- M1.1 core data model (spec §30, §31)

CREATE TYPE task_status AS ENUM (
    'PENDING', 'ASSIGNED', 'RUNNING', 'TESTING', 'REVIEWING',
    'WAITING_FOR_HUMAN', 'COMPLETED', 'FAILED', 'CANCELLED'
);

CREATE TYPE approval_status AS ENUM (
    'PENDING_APPROVAL', 'APPROVED', 'CHANGES_REQUESTED', 'REJECTED'
);

CREATE TYPE autonomy_level AS ENUM ('high', 'medium', 'conservative');

CREATE TABLE projects (
    id             bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    slug           text NOT NULL UNIQUE CHECK (slug ~ '^[a-z0-9][a-z0-9-]*$'),
    name           text NOT NULL,
    repo_url       text NOT NULL,
    default_branch text NOT NULL DEFAULT 'main',
    stack          text NOT NULL CHECK (stack ~ '^[a-z0-9][a-z0-9-]*$'),
    autonomy_level autonomy_level NOT NULL DEFAULT 'conservative',
    test_command   text NOT NULL DEFAULT '',
    created_at     timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE workers (
    id                bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name              text NOT NULL UNIQUE,
    capabilities      text[] NOT NULL DEFAULT '{}',
    status            text NOT NULL DEFAULT 'online' CHECK (status IN ('online', 'offline')),
    last_heartbeat_at timestamptz NOT NULL DEFAULT now(),
    created_at        timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE tasks (
    id                    bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    project_id            bigint NOT NULL REFERENCES projects(id),
    title                 text NOT NULL CHECK (title <> ''),
    description           text NOT NULL DEFAULT '',
    status                task_status NOT NULL DEFAULT 'PENDING',
    -- Where the worker resumes after a gate: 'implement' or 'merge'.
    stage                 text NOT NULL DEFAULT 'implement' CHECK (stage IN ('implement', 'merge')),
    required_capabilities text[] NOT NULL DEFAULT '{}',
    -- Project-relative knowledge docs to load into context (M1.5).
    context_docs          text[] NOT NULL DEFAULT '{}',
    feedback              text NOT NULL DEFAULT '',
    worker_id             bigint REFERENCES workers(id),
    attempts              int NOT NULL DEFAULT 0,
    last_error            text NOT NULL DEFAULT '',
    created_at            timestamptz NOT NULL DEFAULT now(),
    updated_at            timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX tasks_claimable ON tasks (id) WHERE status = 'PENDING';
CREATE INDEX tasks_worker ON tasks (worker_id) WHERE worker_id IS NOT NULL;

CREATE TABLE task_dependencies (
    task_id            bigint NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    depends_on_task_id bigint NOT NULL REFERENCES tasks(id),
    PRIMARY KEY (task_id, depends_on_task_id),
    CHECK (task_id <> depends_on_task_id)
);

CREATE TABLE agent_runs (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    task_id       bigint NOT NULL REFERENCES tasks(id),
    worker_id     bigint NOT NULL REFERENCES workers(id),
    model         text NOT NULL,
    context_files text[] NOT NULL DEFAULT '{}',
    started_at    timestamptz NOT NULL DEFAULT now(),
    finished_at   timestamptz,
    exit_status   int,
    log_path      text NOT NULL DEFAULT '',
    tokens        bigint NOT NULL DEFAULT 0,
    cost_usd      numeric(12, 6) NOT NULL DEFAULT 0
);

CREATE TABLE approval_requests (
    id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    project_id      bigint NOT NULL REFERENCES projects(id),
    subject_type    text NOT NULL,           -- 'task' in V1; documents in V3
    subject_ref     text NOT NULL,           -- task id / doc id
    subject_version text NOT NULL DEFAULT '', -- branch sha / doc version
    gate            text NOT NULL,           -- policy action, e.g. 'merge_protected'
    status          approval_status NOT NULL DEFAULT 'PENDING_APPROVAL',
    summary         text NOT NULL DEFAULT '',
    requested_by    text NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now()
);
-- At most one open request per subject+gate.
CREATE UNIQUE INDEX approval_requests_one_open
    ON approval_requests (subject_type, subject_ref, gate)
    WHERE status = 'PENDING_APPROVAL';

CREATE TABLE approval_decisions (
    id                  bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    approval_request_id bigint NOT NULL REFERENCES approval_requests(id),
    decision            approval_status NOT NULL CHECK (decision <> 'PENDING_APPROVAL'),
    comment             text NOT NULL DEFAULT '',
    decided_by          text NOT NULL,
    decided_at          timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE audit_log (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    actor_type text NOT NULL,   -- 'owner' | 'worker' | 'system'
    actor_id   text NOT NULL,
    action     text NOT NULL,
    target     text NOT NULL,
    payload    jsonb NOT NULL DEFAULT '{}',
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE FUNCTION audit_log_append_only() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'audit_log is append-only';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER audit_log_no_update_delete
    BEFORE UPDATE OR DELETE ON audit_log
    FOR EACH ROW EXECUTE FUNCTION audit_log_append_only();

CREATE TRIGGER audit_log_no_truncate
    BEFORE TRUNCATE ON audit_log
    FOR EACH STATEMENT EXECUTE FUNCTION audit_log_append_only();
