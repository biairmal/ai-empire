DROP TABLE IF EXISTS doc_edges;
DROP TABLE IF EXISTS documents;
DROP TABLE IF EXISTS document_approvals;
ALTER TABLE approval_requests DROP COLUMN covers, DROP COLUMN task_id;
DELETE FROM approval_requests WHERE project_id IS NULL;
ALTER TABLE approval_requests ALTER COLUMN project_id SET NOT NULL;
ALTER TABLE agent_runs DROP COLUMN role;
-- Non-code tasks cannot exist without the V3 columns.
DELETE FROM task_dependencies WHERE task_id IN (SELECT id FROM tasks WHERE kind <> 'code')
                                 OR depends_on_task_id IN (SELECT id FROM tasks WHERE kind <> 'code');
DELETE FROM agent_runs WHERE task_id IN (SELECT id FROM tasks WHERE kind <> 'code');
DELETE FROM tasks WHERE kind <> 'code';
ALTER TABLE tasks
    DROP CONSTRAINT tasks_code_has_repository,
    DROP COLUMN merge_sha,
    DROP COLUMN review_rounds,
    DROP COLUMN review,
    DROP COLUMN output_docs,
    DROP COLUMN revises,
    DROP COLUMN doc_type,
    DROP COLUMN step,
    DROP COLUMN request_id,
    DROP COLUMN role,
    DROP COLUMN kind,
    ALTER COLUMN repository_id SET NOT NULL;
DROP TABLE IF EXISTS requests;
DROP FUNCTION IF EXISTS append_only();
