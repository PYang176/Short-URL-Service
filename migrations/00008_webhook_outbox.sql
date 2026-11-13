-- +goose Up
CREATE TABLE webhook_endpoints (
    id CHAR(26) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    workspace_id CHAR(26) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    name VARCHAR(64) NOT NULL,
    url VARCHAR(2048) NOT NULL,
    secret_ciphertext VARBINARY(512) NOT NULL,
    status TINYINT UNSIGNED NOT NULL DEFAULT 1,
    failure_count INT UNSIGNED NOT NULL DEFAULT 0,
    disabled_at DATETIME(3) NULL,
    deleted_at DATETIME(3) NULL,
    created_at DATETIME(3) NOT NULL,
    updated_at DATETIME(3) NOT NULL,
    PRIMARY KEY (id),
    KEY idx_webhook_endpoints_workspace (workspace_id, deleted_at, created_at),
    CONSTRAINT fk_webhook_endpoints_workspace
        FOREIGN KEY (workspace_id) REFERENCES workspaces (id) ON DELETE CASCADE,
    CONSTRAINT chk_webhook_endpoint_status CHECK (status IN (1, 2))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE webhook_deliveries (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    workspace_id CHAR(26) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    webhook_id CHAR(26) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    event_id CHAR(26) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    event_type VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    payload JSON NOT NULL,
    status TINYINT UNSIGNED NOT NULL DEFAULT 1,
    attempt_count SMALLINT UNSIGNED NOT NULL DEFAULT 0,
    next_attempt_at DATETIME(3) NOT NULL,
    locked_until DATETIME(3) NULL,
    response_status INT NULL,
    last_error VARCHAR(1000) NOT NULL DEFAULT '',
    delivered_at DATETIME(3) NULL,
    created_at DATETIME(3) NOT NULL,
    updated_at DATETIME(3) NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uk_webhook_delivery_event (webhook_id, event_id),
    KEY idx_webhook_deliveries_due (status, next_attempt_at, locked_until),
    KEY idx_webhook_deliveries_endpoint (webhook_id, created_at),
    KEY idx_webhook_deliveries_workspace (workspace_id, created_at),
    CONSTRAINT fk_webhook_deliveries_endpoint
        FOREIGN KEY (webhook_id) REFERENCES webhook_endpoints (id) ON DELETE CASCADE,
    CONSTRAINT chk_webhook_delivery_status CHECK (status IN (1, 2, 3, 4))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- +goose Down
DROP TABLE IF EXISTS webhook_deliveries;
DROP TABLE IF EXISTS webhook_endpoints;
