-- +goose Up
CREATE TABLE comparisons (
    id           text PRIMARY KEY,
    created_at   timestamptz NOT NULL,
    project_path text        NOT NULL,
    prompt       text        NOT NULL,
    profile      jsonb       NOT NULL
);

CREATE TABLE comparison_sides (
    comparison_id  text        NOT NULL REFERENCES comparisons (id) ON DELETE CASCADE,
    side           text        NOT NULL,
    config         jsonb       NOT NULL,
    cli_version    text        NOT NULL,
    status         text        NOT NULL,
    end_reason     text        NOT NULL DEFAULT '',
    run_started_at timestamptz,
    ended_at       timestamptz,
    updated_at     timestamptz NOT NULL,
    -- How long each preparation step took (comparison.Phases).
    phases         jsonb       NOT NULL DEFAULT '{}',
    -- The models.dev price taken when the side started (comparison.PriceSnapshot).
    price_snapshot jsonb       NOT NULL DEFAULT '{}',
    container_id   text        NOT NULL DEFAULT '',
    -- The proxy session at its last save: usage, cost and every request (proxy.Snapshot).
    proxy          jsonb,
    logs           jsonb       NOT NULL DEFAULT '[]',
    -- The terminal output, replayed when the comparison is opened from the history.
    terminal       bytea,
    PRIMARY KEY (comparison_id, side)
);

-- +goose Down
DROP TABLE comparison_sides;
DROP TABLE comparisons;
