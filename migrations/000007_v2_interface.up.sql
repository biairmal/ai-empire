-- V2: Hermes, notifications, remote workers.

-- M2.4: a worker may have its own token (sha256 hex) and be limited to some projects (empty = any).
ALTER TABLE workers ADD COLUMN token_hash text UNIQUE;
ALTER TABLE workers ADD COLUMN project_ids bigint[] NOT NULL DEFAULT '{}';

-- M2.2: the tail of the agent's log, so "why did…?" works when the worker is on another machine.
ALTER TABLE agent_runs ADD COLUMN log_tail text NOT NULL DEFAULT '';

-- M2.3: notification outbox. Rows are written in the same transaction as the audit entry
-- they announce, so a restart never loses one.
CREATE TABLE notifications (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    audit_id   bigint NOT NULL REFERENCES audit_log(id),
    attempts   int NOT NULL DEFAULT 0,
    last_error text NOT NULL DEFAULT '',
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    sent_at    timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX notifications_unsent ON notifications (next_attempt_at) WHERE sent_at IS NULL;
