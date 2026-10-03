-- +goose Up
-- Repetitions: comparisons of one series share series_id and run one after another.
ALTER TABLE comparisons
    ADD COLUMN series_id      text    NOT NULL DEFAULT '',
    ADD COLUMN attempt        integer NOT NULL DEFAULT 0,
    ADD COLUMN series_size    integer NOT NULL DEFAULT 0,
    ADD COLUMN series_stopped boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE comparisons DROP COLUMN series_stopped, DROP COLUMN series_size, DROP COLUMN attempt, DROP COLUMN series_id;
