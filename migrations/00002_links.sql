-- +goose Up
CREATE TABLE domains (
    id CHAR(26) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    workspace_id CHAR(26) CHARACTER SET ascii COLLATE ascii_bin NULL,
    hostname VARCHAR(253) CHARACTER SET ascii COLLATE ascii_general_ci NOT NULL,
    status TINYINT UNSIGNED NOT NULL DEFAULT 1,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    UNIQUE KEY uk_domains_hostname (hostname),
    KEY idx_domains_workspace (workspace_id, created_at),
    CONSTRAINT fk_domains_workspace FOREIGN KEY (workspace_id) REFERENCES workspaces (id),
    CONSTRAINT chk_domains_status CHECK (status IN (1, 2, 3, 4, 5))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE links (
    id CHAR(26) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    workspace_id CHAR(26) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    domain_id CHAR(26) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    slug VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    destination_url VARCHAR(2048) NOT NULL,
    destination_hash BINARY(32) NOT NULL,
    title VARCHAR(255) NOT NULL DEFAULT '',
    notes VARCHAR(1000) NOT NULL DEFAULT '',
    status TINYINT UNSIGNED NOT NULL DEFAULT 1,
    redirect_code SMALLINT UNSIGNED NOT NULL DEFAULT 302,
    starts_at DATETIME(3) NULL,
    expires_at DATETIME(3) NULL,
    human_clicks BIGINT UNSIGNED NOT NULL DEFAULT 0,
    version INT UNSIGNED NOT NULL DEFAULT 1,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    deleted_at DATETIME(3) NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uk_links_domain_slug (domain_id, slug),
    KEY idx_links_workspace_active_created (workspace_id, deleted_at, created_at DESC, id DESC),
    KEY idx_links_workspace_status_created (workspace_id, status, deleted_at, created_at DESC, id DESC),
    KEY idx_links_workspace_destination (workspace_id, destination_hash),
    CONSTRAINT fk_links_workspace FOREIGN KEY (workspace_id) REFERENCES workspaces (id),
    CONSTRAINT fk_links_domain FOREIGN KEY (domain_id) REFERENCES domains (id),
    CONSTRAINT chk_links_status CHECK (status IN (1, 2)),
    CONSTRAINT chk_links_redirect_code CHECK (redirect_code IN (301, 302))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- +goose Down
DROP TABLE IF EXISTS links;
DROP TABLE IF EXISTS domains;
