-- M1.8 clients & multi-repository projects (spec §11A, §11B)

-- Slugs must contain a letter so API paths can take "id or slug" unambiguously.
ALTER TABLE projects ADD CONSTRAINT projects_slug_has_letter CHECK (slug ~ '[a-z]');

CREATE TABLE clients (
    id                     bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    slug                   text NOT NULL UNIQUE CHECK (slug ~ '^[a-z0-9][a-z0-9-]*$' AND slug ~ '[a-z]'),
    name                   text NOT NULL,
    default_autonomy_level autonomy_level NOT NULL DEFAULT 'conservative',
    created_at             timestamptz NOT NULL DEFAULT now()
);

-- NULL = personal / internal project.
ALTER TABLE projects ADD COLUMN client_id bigint REFERENCES clients(id);

CREATE TABLE repositories (
    id             bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    project_id     bigint NOT NULL REFERENCES projects(id),
    name           text NOT NULL CHECK (name ~ '^[a-z0-9][a-z0-9-]*$'),
    repo_url       text NOT NULL,
    default_branch text NOT NULL DEFAULT 'main',
    stack          text NOT NULL CHECK (stack ~ '^[a-z0-9][a-z0-9-]*$'),
    test_command   text NOT NULL DEFAULT '',
    created_at     timestamptz NOT NULL DEFAULT now(),
    UNIQUE (project_id, name),
    UNIQUE (id, project_id) -- target of the tasks composite FK
);

-- Every existing project becomes a project with one repository named after it.
INSERT INTO repositories (project_id, name, repo_url, default_branch, stack, test_command)
SELECT id, slug, repo_url, default_branch, stack, test_command FROM projects;

ALTER TABLE tasks ADD COLUMN repository_id bigint;
UPDATE tasks t SET repository_id = r.id FROM repositories r WHERE r.project_id = t.project_id;
ALTER TABLE tasks ALTER COLUMN repository_id SET NOT NULL;
-- A task's repository must belong to the task's project.
ALTER TABLE tasks ADD CONSTRAINT tasks_repository_in_project
    FOREIGN KEY (repository_id, project_id) REFERENCES repositories (id, project_id);

ALTER TABLE projects
    DROP COLUMN repo_url,
    DROP COLUMN default_branch,
    DROP COLUMN stack,
    DROP COLUMN test_command;
