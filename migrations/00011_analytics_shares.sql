-- +goose Up
CREATE TABLE analytics_shares (
    id CHAR(26) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    workspace_id CHAR(26) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    link_id CHAR(26) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    created_by_user_id CHAR(26) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    token_hash BINARY(32) NOT NULL,
    share_days TINYINT UNSIGNED NOT NULL,
    expires_at DATETIME(3) NOT NULL,
    revoked_at DATETIME(3) NULL,
    created_at DATETIME(3) NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uk_analytics_shares_token (token_hash),
    KEY idx_analytics_shares_link (workspace_id, link_id, created_at DESC),
    CONSTRAINT fk_analytics_shares_workspace
        FOREIGN KEY (workspace_id) REFERENCES workspaces (id) ON DELETE CASCADE,
    CONSTRAINT fk_analytics_shares_link
        FOREIGN KEY (link_id) REFERENCES links (id) ON DELETE CASCADE,
    CONSTRAINT fk_analytics_shares_creator
        FOREIGN KEY (created_by_user_id) REFERENCES users (id) ON DELETE CASCADE,
    CONSTRAINT chk_analytics_shares_days CHECK (share_days IN (1, 7, 30))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- +goose Down
DROP TABLE IF EXISTS analytics_shares;
