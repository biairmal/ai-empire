-- M3.8: files changed by a merged task, for impact analysis and traceability.
ALTER TABLE tasks ADD COLUMN changed_files text[] NOT NULL DEFAULT '{}';
