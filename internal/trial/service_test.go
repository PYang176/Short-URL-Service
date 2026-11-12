package trial

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// TestServiceCreateAndResume 验证游客会话创建、Token 哈希保存和恢复
func TestServiceCreateAndResume(t *testing.T) {
	now := time.Date(2026, 11, 13, 8, 0, 0, 0, time.UTC)
	repo := newFakeRepository()
	service := NewService(repo, 24*time.Hour, 3)
	service.now = func() time.Time { return now }

	session, token, err := service.Create(context.Background())
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if !strings.HasPrefix(token, "trial_") {
		t.Fatalf("token = %q, want trial_ prefix", token)
	}
	if string(session.TokenHash) == token {
		t.Fatal("TokenHash must not equal raw token")
	}
	if repo.createdName != "游客试用" || !strings.HasPrefix(repo.createdSlug, "trial_") {
		t.Fatalf("created workspace = %q/%q", repo.createdName, repo.createdSlug)
	}

	resumed, err := service.Resume(context.Background(), token)
	if err != nil {
		t.Fatalf("Resume() error = %v", err)
	}
	if resumed.ID != session.ID || resumed.WorkspaceID != session.WorkspaceID {
		t.Fatalf("Resume() = %+v, want %+v", resumed, session)
	}
	if _, err := service.Resume(context.Background(), "trial_missing"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("Resume() missing error = %v", err)
	}
}

// TestServiceRejectsExpiredSession 验证过期 Cookie 不能恢复游客会话
func TestServiceRejectsExpiredSession(t *testing.T) {
	now := time.Date(2026, 11, 13, 8, 0, 0, 0, time.UTC)
	repo := newFakeRepository()
	service := NewService(repo, time.Hour, 3)
	service.now = func() time.Time { return now }
	_, token, err := service.Create(context.Background())
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	service.now = func() time.Time { return now.Add(2 * time.Hour) }
	if _, err := service.Resume(context.Background(), token); !errors.Is(err, ErrSessionExpired) {
		t.Fatalf("Resume() error = %v, want ErrSessionExpired", err)
	}
}

// TestServiceEnforcesLinkQuota 验证游客链接配额能够原子占用和归还
func TestServiceEnforcesLinkQuota(t *testing.T) {
	now := time.Date(2026, 11, 13, 8, 0, 0, 0, time.UTC)
	repo := newFakeRepository()
	service := NewService(repo, time.Hour, 2)
	service.now = func() time.Time { return now }
	session, _, err := service.Create(context.Background())
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	for index := 0; index < 2; index++ {
		if err := service.ReserveLink(context.Background(), session.ID); err != nil {
			t.Fatalf("ReserveLink() attempt %d error = %v", index+1, err)
		}
	}
	if err := service.ReserveLink(context.Background(), session.ID); !errors.Is(err, ErrLinkLimitReached) {
		t.Fatalf("ReserveLink() over limit error = %v", err)
	}
	if err := service.ReleaseLink(context.Background(), session.ID); err != nil {
		t.Fatalf("ReleaseLink() error = %v", err)
	}
	if err := service.ReserveLink(context.Background(), session.ID); err != nil {
		t.Fatalf("ReserveLink() after release error = %v", err)
	}
}

type fakeRepository struct {
	sessions    map[string]*Session
	tokens      map[string]*Session
	createdName string
	createdSlug string
}

// newFakeRepository 创建游客会话内存仓库
func newFakeRepository() *fakeRepository {
	return &fakeRepository{
		sessions: make(map[string]*Session),
		tokens:   make(map[string]*Session),
	}
}

// Create 保存内存游客工作空间与会话
func (r *fakeRepository) Create(_ context.Context, session *Session, name, slug string) error {
	copy := cloneSession(session)
	r.sessions[session.ID] = copy
	r.tokens[string(session.TokenHash)] = copy
	r.createdName = name
	r.createdSlug = slug
	return nil
}

// FindByTokenHash 按摘要查询内存游客会话
func (r *fakeRepository) FindByTokenHash(_ context.Context, tokenHash []byte) (*Session, error) {
	session, ok := r.tokens[string(tokenHash)]
	if !ok {
		return nil, ErrSessionNotFound
	}
	return cloneSession(session), nil
}

// ReserveLinkSlot 模拟原子占用游客链接配额
func (r *fakeRepository) ReserveLinkSlot(
	_ context.Context,
	sessionID string,
	maxLinks int,
	now time.Time,
) error {
	session, ok := r.sessions[sessionID]
	if !ok || !session.ExpiresAt.After(now) || int(session.LinkCount) >= maxLinks {
		return ErrLinkLimitReached
	}
	session.LinkCount++
	return nil
}

// ReleaseLinkSlot 模拟归还游客链接配额
func (r *fakeRepository) ReleaseLinkSlot(_ context.Context, sessionID string, _ time.Time) error {
	if session, ok := r.sessions[sessionID]; ok && session.LinkCount > 0 {
		session.LinkCount--
	}
	return nil
}

// CleanupExpired 模拟过期游客数据清理
func (r *fakeRepository) CleanupExpired(_ context.Context, _ time.Time, _ int) (int, error) {
	return 0, nil
}

// cloneSession 复制游客会话，避免测试仓库暴露内部切片
func cloneSession(session *Session) *Session {
	copy := *session
	copy.TokenHash = append([]byte(nil), session.TokenHash...)
	return &copy
}
