-- V3: workflow requests, document tasks, approved document versions, knowledge graph index.

CREATE FUNCTION append_only() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION '% is append-only', TG_TABLE_NAME;
END;
$$ LANGUAGE plpgsql;

-- A plain-language request driven through a workflow (spec §7).
CREATE TABLE requests (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    project_id    bigint NOT NULL REFERENCES projects(id),
    repository_id bigint,                 -- only for workflows that start with code (quick-fix)
    workflow      text NOT NULL,
    title         text NOT NULL CHECK (title <> ''),
    description   text NOT NULL DEFAULT '',
    status        text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'completed', 'cancelled')),
    current_step  text NOT NULL DEFAULT '',
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (repository_id, project_id) REFERENCES repositories (id, project_id)
);

-- Tasks now come in kinds. Only code tasks target a repository.
ALTER TABLE tasks
    ALTER COLUMN repository_id DROP NOT NULL,
    ADD COLUMN kind          text NOT NULL DEFAULT 'code' CHECK (kind IN ('code', 'document', 'plan', 'revise')),
    ADD COLUMN role          text NOT NULL DEFAULT 'developer',
    ADD COLUMN request_id    bigint REFERENCES requests(id),
    ADD COLUMN step          text NOT NULL DEFAULT '',
    ADD COLUMN doc_type      text NOT NULL DEFAULT '',
    ADD COLUMN revises       text NOT NULL DEFAULT '',         -- kind=revise: document key being revised
    ADD COLUMN output_docs   text[] NOT NULL DEFAULT '{}',     -- document keys this task produced
    ADD COLUMN review        boolean NOT NULL DEFAULT false,   -- AI review before the merge gate
    ADD COLUMN review_rounds int NOT NULL DEFAULT 0,
    ADD COLUMN merge_sha     text NOT NULL DEFAULT '',         -- commit on the default branch (traceability)
    ADD CONSTRAINT tasks_code_has_repository CHECK ((kind = 'code') = (repository_id IS NOT NULL));
CREATE INDEX tasks_request ON tasks (request_id) WHERE request_id IS NOT NULL;

ALTER TABLE agent_runs ADD COLUMN role text NOT NULL DEFAULT 'developer';

-- Document approvals: may be global (no project), may belong to a task, and
-- cover several documents approved together ("<key>@v<version>@<hash>").
ALTER TABLE approval_requests
    ALTER COLUMN project_id DROP NOT NULL,
    ADD COLUMN task_id bigint REFERENCES tasks(id),
    ADD COLUMN covers  text[] NOT NULL DEFAULT '{}';

-- Authoritative record of every approved document version (spec §12).
CREATE TABLE document_approvals (
    scope               text NOT NULL,
    doc_id              text NOT NULL,
    version             int NOT NULL CHECK (version >= 1),
    hash                text NOT NULL,
    approval_request_id bigint NOT NULL REFERENCES approval_requests(id),
    approved_by         text NOT NULL,
    approved_at         timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (scope, doc_id, version)
);
CREATE TRIGGER document_approvals_append_only
    BEFORE UPDATE OR DELETE ON document_approvals
    FOR EACH ROW EXECUTE FUNCTION append_only();

-- Knowledge graph index (spec §14). A cache rebuilt from the Markdown files.
CREATE TABLE documents (
    scope      text NOT NULL,
    doc_id     text NOT NULL,
    type       text NOT NULL,
    title      text NOT NULL,
    path       text NOT NULL,
    status     text NOT NULL,
    version    int NOT NULL,
    hash       text NOT NULL,
    indexed_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (scope, doc_id)
);

CREATE TABLE doc_edges (
    from_scope text NOT NULL,
    from_id    text NOT NULL,
    rel        text NOT NULL,
    to_scope   text NOT NULL,
    to_id      text NOT NULL,
    PRIMARY KEY (from_scope, from_id, rel, to_scope, to_id)
);
CREATE INDEX doc_edges_to ON doc_edges (to_scope, to_id);
