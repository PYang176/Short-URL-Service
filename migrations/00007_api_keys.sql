-- +goose Up
CREATE TABLE api_keys (
    id CHAR(26) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    workspace_id CHAR(26) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    created_by_user_id CHAR(26) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    name VARCHAR(64) NOT NULL,
    key_prefix VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    secret_hash BINARY(32) NOT NULL,
    scopes VARCHAR(255) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    last_used_at DATETIME(3) NULL,
    expires_at DATETIME(3) NULL,
    revoked_at DATETIME(3) NULL,
    created_at DATETIME(3) NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uk_api_keys_prefix (key_prefix),
    KEY idx_api_keys_workspace_created (workspace_id, created_at DESC),
    KEY idx_api_keys_active (workspace_id, revoked_at, expires_at),
    CONSTRAINT fk_api_keys_workspace
        FOREIGN KEY (workspace_id) REFERENCES workspaces (id) ON DELETE CASCADE,
    CONSTRAINT fk_api_keys_creator
        FOREIGN KEY (created_by_user_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- +goose Down
DROP TABLE IF EXISTS api_keys;
