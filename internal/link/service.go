package link

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/url"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
)

const (
	// cacheKeyPrefix 是链接跳转快照的 Redis Key 前缀
	cacheKeyPrefix = "link:v1:"
	// nullCacheValue 表示链接不存在或已删除的短缓存
	nullCacheValue = "__null__"
	// cacheBaseTTL 是有效链接快照的基础缓存时长
	cacheBaseTTL = 30 * time.Minute
	// nullCacheTTL 是不存在链接的缓存时长
	nullCacheTTL = time.Minute
)

// Service 提供短链接创建、管理、缓存和跳转解析
type Service struct {
	// repo 是链接和域名的权威数据仓库
	repo Repository
	// redis 提供跳转缓存
	redis *redis.Client
	// logger 记录缓存降级等非致命错误
	logger *slog.Logger
	// baseURL 是默认短链公开地址的根
	baseURL *url.URL
	// defaultDomain 是当前平台默认域名
	defaultDomain *Domain
	// cacheGroup 合并同一短码的并发回源
	cacheGroup singleflight.Group
	// now 提供可替换的 UTC 时间
	now func() time.Time
}

// NewService 创建短链接服务
func NewService(repo Repository, redisClient *redis.Client, logger *slog.Logger, publicBaseURL string) (*Service, error) {
	baseURL, err := url.Parse(publicBaseURL)
	if err != nil {
		return nil, fmt.Errorf("解析公开短链根地址失败: %w", err)
	}
	return &Service{
		repo:    repo,
		redis:   redisClient,
		logger:  logger,
		baseURL: baseURL,
		now:     func() time.Time { return time.Now().UTC() },
	}, nil
}

// Initialize 创建或加载平台默认域名
func (s *Service) Initialize(ctx context.Context) error {
	hostname := strings.ToLower(s.baseURL.Hostname())
	domain, err := s.repo.EnsureDefaultDomain(ctx, hostname, s.now())
	if err != nil {
		return err
	}
	s.defaultDomain = domain
	return nil
}

// Create 创建短链接并在随机短码冲突时有限重试
func (s *Service) Create(ctx context.Context, workspaceID string, input CreateInput) (*Link, error) {
	if s.defaultDomain == nil {
		return nil, fmt.Errorf("默认域名尚未初始化")
	}

	destination, destinationHash, err := normalizeDestination(input.DestinationURL)
	if err != nil {
		return nil, err
	}
	title := strings.TrimSpace(input.Title)
	notes := strings.TrimSpace(input.Notes)
	if len([]rune(title)) > 255 || len([]rune(notes)) > 1000 {
		return nil, fmt.Errorf("%w: 标题或备注过长", ErrInvalidInput)
	}
	redirectCode := input.RedirectCode
	if redirectCode == 0 {
		redirectCode = 302
	}
	if redirectCode != 301 && redirectCode != 302 {
		return nil, fmt.Errorf("%w: 跳转状态码只能是 301 或 302", ErrInvalidInput)
	}

	alias := strings.TrimSpace(input.Alias)
	if alias != "" {
		if err := validateAlias(alias); err != nil {
			return nil, err
		}
	}
	startsAt := normalizeUTCTime(input.StartsAt)
	expiresAt := normalizeUTCTime(input.ExpiresAt)
	if err := validateWindow(startsAt, expiresAt); err != nil {
		return nil, err
	}

	const maxAttempts = 5
	for attempt := 0; attempt < maxAttempts; attempt++ {
		slug := alias
		if slug == "" {
			slug, err = generateShortCode(randomShortCodeLength)
			if err != nil {
				return nil, err
			}
		}

		now := s.now()
		candidate := &Link{
			ID:              newID(),
			WorkspaceID:     workspaceID,
			DomainID:        s.defaultDomain.ID,
			Slug:            slug,
			DestinationURL:  destination,
			DestinationHash: destinationHash[:],
			Title:           title,
			Notes:           notes,
			Status:          StatusActive,
			RedirectCode:    redirectCode,
			StartsAt:        startsAt,
			ExpiresAt:       expiresAt,
			Version:         1,
			CreatedAt:       now,
			UpdatedAt:       now,
		}
		if err := s.repo.Create(ctx, candidate); err == nil {
			return candidate, nil
		} else if !errors.Is(err, ErrSlugTaken) {
			return nil, err
		}

		if alias != "" {
			return nil, ErrSlugTaken
		}
	}
	return nil, fmt.Errorf("生成唯一短码失败")
}

// List 返回工作空间链接列表
func (s *Service) List(
	ctx context.Context,
	workspaceID string,
	limit int,
	beforeCreatedAt *time.Time,
	beforeID string,
) ([]Link, error) {
	if limit < 1 || limit > 100 {
		return nil, fmt.Errorf("%w: limit 必须为 1 至 100", ErrInvalidInput)
	}
	return s.repo.List(ctx, workspaceID, limit, beforeCreatedAt, beforeID)
}

// Get 返回工作空间内的单个链接
func (s *Service) Get(ctx context.Context, workspaceID, linkID string) (*Link, error) {
	return s.repo.FindByID(ctx, workspaceID, linkID)
}

// Update 修改链接可编辑字段并使缓存失效
func (s *Service) Update(
	ctx context.Context,
	workspaceID, linkID string,
	input UpdateInput,
) (*Link, error) {
	current, err := s.repo.FindByID(ctx, workspaceID, linkID)
	if err != nil {
		return nil, err
	}

	if input.DestinationURL != nil {
		destination, destinationHash, err := normalizeDestination(*input.DestinationURL)
		if err != nil {
			return nil, err
		}
		current.DestinationURL = destination
		current.DestinationHash = destinationHash[:]
	}
	if input.Title != nil {
		title := strings.TrimSpace(*input.Title)
		if len([]rune(title)) > 255 {
			return nil, fmt.Errorf("%w: 标题不能超过 255 个字符", ErrInvalidInput)
		}
		current.Title = title
	}
	if input.Notes != nil {
		notes := strings.TrimSpace(*input.Notes)
		if len([]rune(notes)) > 1000 {
			return nil, fmt.Errorf("%w: 备注不能超过 1000 个字符", ErrInvalidInput)
		}
		current.Notes = notes
	}
	if input.Status != nil {
		if *input.Status != StatusActive && *input.Status != StatusDisabled {
			return nil, fmt.Errorf("%w: 链接状态无效", ErrInvalidInput)
		}
		current.Status = *input.Status
	}
	if input.RedirectCode != nil {
		if *input.RedirectCode != 301 && *input.RedirectCode != 302 {
			return nil, fmt.Errorf("%w: 跳转状态码只能是 301 或 302", ErrInvalidInput)
		}
		current.RedirectCode = *input.RedirectCode
	}
	if input.ClearExpiresAt {
		current.ExpiresAt = nil
	} else if input.ExpiresAt != nil {
		current.ExpiresAt = normalizeUTCTime(input.ExpiresAt)
	}
	if err := validateWindow(current.StartsAt, current.ExpiresAt); err != nil {
		return nil, err
	}

	current.Version++
	current.UpdatedAt = s.now()
	if err := s.repo.Update(ctx, current); err != nil {
		return nil, err
	}
	s.deleteCache(ctx, current.DomainID, current.Slug)
	return current, nil
}

// Delete 软删除链接并使缓存失效
func (s *Service) Delete(ctx context.Context, workspaceID, linkID string) error {
	current, err := s.repo.FindByID(ctx, workspaceID, linkID)
	if err != nil {
		return err
	}
	if err := s.repo.SoftDelete(ctx, workspaceID, linkID, s.now()); err != nil {
		return err
	}
	s.deleteCache(ctx, current.DomainID, current.Slug)
	return nil
}

// Resolve 解析默认域名短码并返回可跳转目标
func (s *Service) Resolve(ctx context.Context, slug string) (string, int, error) {
	if s.defaultDomain == nil {
		return "", 0, fmt.Errorf("默认域名尚未初始化")
	}
	cacheKey := linkCacheKey(s.defaultDomain.ID, slug)

	if cached, found, err := s.readCache(ctx, cacheKey); err != nil {
		if errors.Is(err, ErrNotFound) {
			return "", 0, ErrNotFound
		}
		s.logger.Warn("读取链接缓存失败，回源 MySQL", "error", err)
	} else if found {
		return s.evaluateCached(cached, s.now())
	}

	value, err, _ := s.cacheGroup.Do(cacheKey, func() (any, error) {
		if cached, found, cacheErr := s.readCache(ctx, cacheKey); cacheErr == nil && found {
			return cached, nil
		} else if errors.Is(cacheErr, ErrNotFound) {
			return nil, ErrNotFound
		}

		link, findErr := s.repo.FindByDomainSlug(ctx, s.defaultDomain.ID, slug)
		if errors.Is(findErr, ErrNotFound) {
			s.setNullCache(ctx, cacheKey)
			return nil, ErrNotFound
		}
		if findErr != nil {
			return nil, findErr
		}

		cached := toCachedLink(link)
		s.setCache(ctx, cacheKey, cached, link.ExpiresAt)
		return cached, nil
	})
	if err != nil {
		return "", 0, err
	}
	cached, ok := value.(*CachedLink)
	if !ok {
		return "", 0, fmt.Errorf("链接缓存类型无效")
	}
	return s.evaluateCached(cached, s.now())
}

// ShortURL 返回默认域名下的完整短链地址
func (s *Service) ShortURL(link *Link) string {
	target := *s.baseURL
	target.Path = strings.TrimRight(target.Path, "/") + "/r/" + link.Slug
	target.RawQuery = ""
	target.Fragment = ""
	return target.String()
}

// evaluateCached 根据时间窗口和状态判断缓存链接是否可跳转
func (s *Service) evaluateCached(cached *CachedLink, now time.Time) (string, int, error) {
	if cached.Status != StatusActive {
		return "", 0, ErrGone
	}
	if cached.StartsAt != nil && now.Before(*cached.StartsAt) {
		return "", 0, ErrNotStarted
	}
	if cached.ExpiresAt != nil && !now.Before(*cached.ExpiresAt) {
		return "", 0, ErrGone
	}
	return cached.DestinationURL, cached.RedirectCode, nil
}

// readCache 读取并反序列化链接缓存
func (s *Service) readCache(ctx context.Context, key string) (*CachedLink, bool, error) {
	value, err := s.redis.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if string(value) == nullCacheValue {
		return nil, false, ErrNotFound
	}

	var cached CachedLink
	if err := json.Unmarshal(value, &cached); err != nil {
		return nil, false, err
	}
	return &cached, true, nil
}

// setCache 写入带随机抖动和过期上限的链接缓存
func (s *Service) setCache(ctx context.Context, key string, cached *CachedLink, expiresAt *time.Time) {
	payload, err := json.Marshal(cached)
	if err != nil {
		s.logger.Warn("序列化链接缓存失败", "error", err)
		return
	}
	ttl := cacheBaseTTL + time.Duration(rand.IntN(300))*time.Second
	if expiresAt != nil {
		remaining := expiresAt.UTC().Sub(s.now())
		if remaining < ttl {
			ttl = remaining
		}
	}
	if ttl < time.Second {
		ttl = time.Second
	}
	if err := s.redis.Set(ctx, key, payload, ttl).Err(); err != nil {
		s.logger.Warn("写入链接缓存失败", "error", err)
	}
}

// setNullCache 写入短时不存在缓存以防止缓存穿透
func (s *Service) setNullCache(ctx context.Context, key string) {
	if err := s.redis.Set(ctx, key, nullCacheValue, nullCacheTTL).Err(); err != nil {
		s.logger.Warn("写入空值缓存失败", "error", err)
	}
}

// deleteCache 删除更新或删除链接对应的缓存
func (s *Service) deleteCache(ctx context.Context, domainID, slug string) {
	if err := s.redis.Del(ctx, linkCacheKey(domainID, slug)).Err(); err != nil {
		s.logger.Warn("删除链接缓存失败", "error", err)
	}
}

// linkCacheKey 生成域名和短码的跳转缓存 Key
func linkCacheKey(domainID, slug string) string {
	return cacheKeyPrefix + domainID + ":" + slug
}

// toCachedLink 将权威链接转换为跳转缓存快照
func toCachedLink(link *Link) *CachedLink {
	return &CachedLink{
		ID:             link.ID,
		WorkspaceID:    link.WorkspaceID,
		DestinationURL: link.DestinationURL,
		Status:         link.Status,
		RedirectCode:   link.RedirectCode,
		StartsAt:       link.StartsAt,
		ExpiresAt:      link.ExpiresAt,
		Version:        link.Version,
	}
}

// newID 生成短链接业务使用的 ULID
func newID() string {
	return ulid.Make().String()
}
