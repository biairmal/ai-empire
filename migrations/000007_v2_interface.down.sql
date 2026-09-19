DROP TABLE notifications;
ALTER TABLE agent_runs DROP COLUMN log_tail;
ALTER TABLE workers DROP COLUMN project_ids;
ALTER TABLE workers DROP COLUMN token_hash;
