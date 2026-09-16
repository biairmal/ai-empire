-- M1.9: what the agent says it did, shown to the human at the merge gate.
ALTER TABLE agent_runs ADD COLUMN summary text NOT NULL DEFAULT '';
