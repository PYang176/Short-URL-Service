-- +goose Up
ALTER TABLE workspaces
    DROP FOREIGN KEY fk_workspaces_owner;

ALTER TABLE workspaces
    ADD COLUMN kind TINYINT UNSIGNED NOT NULL DEFAULT 1 AFTER id,
    MODIFY owner_user_id CHAR(26) CHARACTER SET ascii COLLATE ascii_bin NULL;

ALTER TABLE workspaces
    ADD CONSTRAINT fk_workspaces_owner
        FOREIGN KEY (owner_user_id) REFERENCES users (id),
    ADD CONSTRAINT chk_workspaces_kind
        CHECK (
            (kind = 1 AND owner_user_id IS NOT NULL)
            OR (kind = 2 AND owner_user_id IS NULL)
        );

CREATE TABLE trial_sessions (
    id CHAR(26) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    workspace_id CHAR(26) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    token_hash BINARY(32) NOT NULL,
    link_count SMALLINT UNSIGNED NOT NULL DEFAULT 0,
    expires_at DATETIME(3) NOT NULL,
    created_at DATETIME(3) NOT NULL,
    updated_at DATETIME(3) NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uk_trial_sessions_workspace (workspace_id),
    UNIQUE KEY uk_trial_sessions_token (token_hash),
    KEY idx_trial_sessions_expiry (expires_at),
    CONSTRAINT fk_trial_sessions_workspace
        FOREIGN KEY (workspace_id) REFERENCES workspaces (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- +goose Down
DROP TABLE IF EXISTS trial_sessions;

DELETE ce
FROM click_events ce
INNER JOIN workspaces w ON w.id = ce.workspace_id
WHERE w.kind = 2;

DELETE FROM links
WHERE workspace_id IN (SELECT id FROM workspaces WHERE kind = 2);

DELETE FROM idempotency_keys
WHERE workspace_id IN (SELECT id FROM workspaces WHERE kind = 2);

DELETE FROM workspace_members
WHERE workspace_id IN (SELECT id FROM workspaces WHERE kind = 2);

DELETE FROM workspaces WHERE kind = 2;

ALTER TABLE workspaces
    DROP CHECK chk_workspaces_kind,
    DROP FOREIGN KEY fk_workspaces_owner;

ALTER TABLE workspaces
    MODIFY owner_user_id CHAR(26) CHARACTER SET ascii COLLATE ascii_bin NOT NULL;

ALTER TABLE workspaces
    ADD CONSTRAINT fk_workspaces_owner
        FOREIGN KEY (owner_user_id) REFERENCES users (id);

ALTER TABLE workspaces
    DROP COLUMN kind;
