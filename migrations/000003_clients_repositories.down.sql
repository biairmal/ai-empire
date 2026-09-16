-- Lossy for multi-repo projects: each project keeps only its first repository's settings.
ALTER TABLE projects
    ADD COLUMN repo_url       text NOT NULL DEFAULT '',
    ADD COLUMN default_branch text NOT NULL DEFAULT 'main',
    ADD COLUMN stack          text NOT NULL DEFAULT 'go' CHECK (stack ~ '^[a-z0-9][a-z0-9-]*$'),
    ADD COLUMN test_command   text NOT NULL DEFAULT '';

UPDATE projects p SET repo_url = r.repo_url, default_branch = r.default_branch, stack = r.stack, test_command = r.test_command
FROM (SELECT DISTINCT ON (project_id) * FROM repositories ORDER BY project_id, id) r
WHERE r.project_id = p.id;

ALTER TABLE projects ALTER COLUMN repo_url DROP DEFAULT, ALTER COLUMN stack DROP DEFAULT;

ALTER TABLE tasks DROP CONSTRAINT tasks_repository_in_project;
ALTER TABLE tasks DROP COLUMN repository_id;
DROP TABLE repositories;
ALTER TABLE projects DROP COLUMN client_id;
DROP TABLE clients;
ALTER TABLE projects DROP CONSTRAINT projects_slug_has_letter;
