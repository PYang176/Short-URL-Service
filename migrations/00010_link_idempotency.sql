-- +goose Up
ALTER TABLE links
    ADD COLUMN idempotency_key VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NULL AFTER notes,
    ADD UNIQUE KEY uk_links_workspace_idempotency (workspace_id, idempotency_key);

-- +goose Down
ALTER TABLE links
    DROP INDEX uk_links_workspace_idempotency,
    DROP COLUMN idempotency_key;
