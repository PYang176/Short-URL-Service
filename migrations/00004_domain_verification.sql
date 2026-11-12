-- +goose Up
ALTER TABLE domains
    ADD COLUMN verification_token VARCHAR(64) NOT NULL DEFAULT '' AFTER status,
    ADD COLUMN expected_cname VARCHAR(253) CHARACTER SET ascii COLLATE ascii_general_ci NOT NULL DEFAULT '' AFTER verification_token,
    ADD COLUMN verification_attempts INT UNSIGNED NOT NULL DEFAULT 0 AFTER expected_cname,
    ADD COLUMN last_verified_at DATETIME(3) NULL AFTER verification_attempts,
    ADD COLUMN next_verify_at DATETIME(3) NULL AFTER last_verified_at,
    ADD COLUMN deleted_at DATETIME(3) NULL AFTER updated_at,
    ADD KEY idx_domains_verification (status, next_verify_at);

-- +goose Down
ALTER TABLE domains
    DROP INDEX idx_domains_verification,
    DROP COLUMN deleted_at,
    DROP COLUMN next_verify_at,
    DROP COLUMN last_verified_at,
    DROP COLUMN verification_attempts,
    DROP COLUMN expected_cname,
    DROP COLUMN verification_token;
