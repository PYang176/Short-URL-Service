-- +goose Up
CREATE TABLE click_events (
    event_id CHAR(26) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    workspace_id CHAR(26) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    link_id CHAR(26) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    domain_id CHAR(26) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    slug VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    clicked_at DATETIME(3) NOT NULL,
    visitor_hash BINARY(32) NOT NULL,
    country CHAR(2) CHARACTER SET ascii NOT NULL DEFAULT '',
    device VARCHAR(16) NOT NULL DEFAULT '',
    os VARCHAR(32) NOT NULL DEFAULT '',
    browser VARCHAR(32) NOT NULL DEFAULT '',
    language VARCHAR(16) NOT NULL DEFAULT '',
    referrer VARCHAR(2048) NOT NULL DEFAULT '',
    utm_source VARCHAR(255) NOT NULL DEFAULT '',
    utm_medium VARCHAR(255) NOT NULL DEFAULT '',
    utm_campaign VARCHAR(255) NOT NULL DEFAULT '',
    utm_term VARCHAR(255) NOT NULL DEFAULT '',
    utm_content VARCHAR(255) NOT NULL DEFAULT '',
    is_bot TINYINT(1) NOT NULL DEFAULT 0,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (event_id),
    KEY idx_click_events_link_time (workspace_id, link_id, clicked_at),
    KEY idx_click_events_workspace_time (workspace_id, clicked_at),
    KEY idx_click_events_retention (clicked_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- +goose Down
DROP TABLE IF EXISTS click_events;
