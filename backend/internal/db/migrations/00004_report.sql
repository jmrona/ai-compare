-- +goose Up
-- The report: acceptance criteria set before the comparison starts, and the user's verdict on
-- the report, kept when the report is generated again.
ALTER TABLE comparisons
    ADD COLUMN criteria     jsonb NOT NULL DEFAULT '[]',
    ADD COLUMN user_verdict jsonb;

-- +goose Down
ALTER TABLE comparisons DROP COLUMN user_verdict, DROP COLUMN criteria;
