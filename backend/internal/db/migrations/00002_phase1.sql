-- +goose Up
ALTER TABLE comparison_sides
    -- The side's proxy token while it runs, so a restarted api can reattach to its container.
    -- Emptied when the side ends.
    ADD COLUMN token   text  NOT NULL DEFAULT '',
    -- For status "error": "agent" or "infrastructure".
    ADD COLUMN failure text  NOT NULL DEFAULT '',
    -- What was collected after the agent ended: changed files, tests, session usage (comparison.Result).
    ADD COLUMN result  jsonb NOT NULL DEFAULT '{}',
    -- When the user typed into the terminal, for the human wait time.
    ADD COLUMN inputs  jsonb NOT NULL DEFAULT '[]';

ALTER TABLE comparisons
    -- "none", "generating", "ready" or "error".
    ADD COLUMN report_status text NOT NULL DEFAULT 'none',
    ADD COLUMN report        jsonb,
    -- When retention removed its containers, images and staging copy.
    ADD COLUMN cleaned_at    timestamptz;

CREATE TABLE settings (
    id   integer PRIMARY KEY CHECK (id = 1),
    data jsonb   NOT NULL
);

-- +goose Down
DROP TABLE settings;
ALTER TABLE comparisons DROP COLUMN cleaned_at, DROP COLUMN report, DROP COLUMN report_status;
ALTER TABLE comparison_sides DROP COLUMN inputs, DROP COLUMN result, DROP COLUMN failure, DROP COLUMN token;
