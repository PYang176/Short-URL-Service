package repository

import (
	"context"
	"database/sql"
	"shortURL/internal/model"
)

type URLRepo struct {
	db *sql.DB
}

func NewURLRepo(db *sql.DB) *URLRepo {
	return &URLRepo{db: db}
}

// Exists 检查短码是否已存在
func (r *URLRepo) Exists(ctx context.Context, code string) (bool, error) {
	query := `SELECT 1 FROM short_urls WHERE short_code = ? LIMIT 1`
	var temp int
	err := r.db.QueryRowContext(ctx, query, code).Scan(&temp)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

// Insert 插入新数据
func (r *URLRepo) Insert(ctx context.Context, url *model.URL) (int64, error) {
	query := `INSERT INTO short_urls (original_url, short_code, is_custom, clicks, created_at, expired_at) 
              VALUES (?, ?, ?, ?, ?, ?)`
	result, err := r.db.ExecContext(ctx, query, url.OriginalURL, url.ShortCode, url.IsCustom, url.Clicks, url.CreatedAt, url.ExpiredAt)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// FindByCode 查询短链
func (r *URLRepo) FindByCode(ctx context.Context, code string) (*model.URL, error) {
	//query := `SELECT id, original_url, short_code, is_custom, clicks FROM short_urls WHERE short_code = ? AND (expired_at = 0 OR expired_at > ?)`
	//row := r.db.QueryRowContext(ctx, query, code, time.Now().Unix())
	query := `SELECT id, original_url, short_code, is_custom, clicks, expired_at FROM short_urls WHERE short_code = ?`
	row := r.db.QueryRowContext(ctx, query, code)

	var u model.URL
	err := row.Scan(&u.ID, &u.OriginalURL, &u.ShortCode, &u.IsCustom, &u.Clicks, &u.ExpiredAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &u, err
}

// IncrementClicks 原子增加点击量
func (r *URLRepo) IncrementClicks(ctx context.Context, code string) error {
	query := `UPDATE short_urls SET clicks = clicks + 1 WHERE short_code = ?`
	_, err := r.db.ExecContext(ctx, query, code)
	return err
}

// FindByOriginalURL 查询原始URL
func (r *URLRepo) FindByOriginalURL(ctx context.Context, originalURL string) (*model.URL, error) {
	query := `SELECT id, original_url, short_code, is_custom, clicks, expired_at FROM short_urls WHERE original_url = ?`
	row := r.db.QueryRowContext(ctx, query, originalURL)

	var u model.URL
	err := row.Scan(&u.ID, &u.OriginalURL, &u.ShortCode, &u.IsCustom, &u.Clicks, &u.ExpiredAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &u, err
}
