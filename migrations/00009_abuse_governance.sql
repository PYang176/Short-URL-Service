-- +goose Up
CREATE TABLE blocked_domains (
    id CHAR(26) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    hostname VARCHAR(253) CHARACTER SET ascii COLLATE ascii_general_ci NOT NULL,
    reason VARCHAR(500) NOT NULL,
    created_by_user_id CHAR(26) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    deleted_at DATETIME(3) NULL,
    created_at DATETIME(3) NOT NULL,
    updated_at DATETIME(3) NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uk_blocked_domains_hostname (hostname),
    KEY idx_blocked_domains_active (deleted_at, hostname),
    CONSTRAINT fk_blocked_domains_creator
        FOREIGN KEY (created_by_user_id) REFERENCES users (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE abuse_reports (
    id CHAR(26) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    link_id CHAR(26) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    workspace_id CHAR(26) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    reporter_user_id CHAR(26) CHARACTER SET ascii COLLATE ascii_bin NULL,
    reason VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    details VARCHAR(1000) NOT NULL DEFAULT '',
    status TINYINT UNSIGNED NOT NULL DEFAULT 1,
    resolution_note VARCHAR(1000) NOT NULL DEFAULT '',
    resolved_by_user_id CHAR(26) CHARACTER SET ascii COLLATE ascii_bin NULL,
    resolved_at DATETIME(3) NULL,
    created_at DATETIME(3) NOT NULL,
    updated_at DATETIME(3) NOT NULL,
    PRIMARY KEY (id),
    KEY idx_abuse_reports_status_created (status, created_at, id),
    KEY idx_abuse_reports_link (link_id, created_at),
    KEY idx_abuse_reports_workspace (workspace_id, status, created_at),
    CONSTRAINT fk_abuse_reports_link
        FOREIGN KEY (link_id) REFERENCES links (id),
    CONSTRAINT fk_abuse_reports_workspace
        FOREIGN KEY (workspace_id) REFERENCES workspaces (id),
    CONSTRAINT fk_abuse_reports_reporter
        FOREIGN KEY (reporter_user_id) REFERENCES users (id) ON DELETE SET NULL,
    CONSTRAINT fk_abuse_reports_resolver
        FOREIGN KEY (resolved_by_user_id) REFERENCES users (id) ON DELETE SET NULL,
    CONSTRAINT chk_abuse_reports_status CHECK (status IN (1, 2, 3)),
    CONSTRAINT chk_abuse_reports_reason
        CHECK (reason IN ('phishing', 'malware', 'spam', 'impersonation', 'other'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE audit_logs (
    id CHAR(26) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    actor_user_id CHAR(26) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    workspace_id CHAR(26) CHARACTER SET ascii COLLATE ascii_bin NULL,
    action VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    target_type VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    target_id VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    metadata JSON NULL,
    created_at DATETIME(3) NOT NULL,
    PRIMARY KEY (id),
    KEY idx_audit_logs_actor_created (actor_user_id, created_at, id),
    KEY idx_audit_logs_action_created (action, created_at, id),
    KEY idx_audit_logs_target (target_type, target_id, created_at),
    CONSTRAINT fk_audit_logs_actor
        FOREIGN KEY (actor_user_id) REFERENCES users (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- +goose Down
DROP TABLE IF EXISTS audit_logs;
DROP TABLE IF EXISTS abuse_reports;
DROP TABLE IF EXISTS blocked_domains;
