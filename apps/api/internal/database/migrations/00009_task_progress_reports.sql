-- +goose Up
-- Structured progress reports: members can explain what they did, what is
-- finished, what is still open and attach delivery references. The reported
-- percentage still feeds the existing task progress/history pipeline, but a
-- report never counts as task completion or acceptance on its own.

CREATE TABLE task_progress_reports (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id uuid NOT NULL,
    project_id uuid NOT NULL,
    task_id uuid NOT NULL,
    actor_id uuid NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    reported_progress smallint NOT NULL CHECK (reported_progress BETWEEN 0 AND 99),
    work_summary text NOT NULL CHECK (length(btrim(work_summary)) BETWEEN 1 AND 4000),
    completed_work text NOT NULL DEFAULT '' CHECK (length(completed_work) <= 4000),
    pending_work text NOT NULL DEFAULT '' CHECK (length(pending_work) <= 4000),
    evidence_links jsonb NOT NULL DEFAULT '[]'
        CHECK (jsonb_typeof(evidence_links) = 'array' AND jsonb_array_length(evidence_links) <= 20),
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    FOREIGN KEY (team_id, project_id, task_id)
        REFERENCES tasks (team_id, project_id, id) ON DELETE RESTRICT
);

CREATE INDEX task_progress_reports_task_idx
    ON task_progress_reports (team_id, project_id, task_id, created_at DESC, id);

COMMENT ON TABLE task_progress_reports IS
    'Append-only member progress reports; a report is execution evidence, never formal task acceptance.';

-- +goose Down
-- Progress reports are business evidence. Refuse to remove them silently.
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM task_progress_reports) THEN
        RAISE EXCEPTION 'refusing to drop task_progress_reports while progress evidence exists';
    END IF;
END $$;
-- +goose StatementEnd
DROP TABLE IF EXISTS task_progress_reports;
