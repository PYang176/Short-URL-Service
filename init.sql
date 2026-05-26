CREATE TABLE IF NOT EXISTS short_urls (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    original_url TEXT NOT NULL,
    short_code VARCHAR(20) NOT NULL UNIQUE, -- 唯一索引，防止重复
    is_custom TINYINT(1) DEFAULT 0, -- 0: false, 1: true
    clicks BIGINT DEFAULT 0,
    created_at BIGINT NOT NULL,
    expired_at BIGINT DEFAULT 0
);

-- 索引优化
CREATE INDEX idx_short_code ON short_urls(short_code);