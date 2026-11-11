package identity

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// TestServiceRegisterAndRefreshRotation 验证注册、空间创建和 Token 轮换
func TestServiceRegisterAndRefreshRotation(t *testing.T) {
	repo := newFakeRepository()
	hasher := testPasswordHasher()
	tokens := NewTokenManager("0123456789abcdef0123456789abcdef", 15*time.Minute)
	service := NewService(repo, hasher, tokens, 24*time.Hour)

	registered, err := service.Register(context.Background(), "Alice@Example.com", "password123", "Alice")
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if registered.User.Email != "alice@example.com" {
		t.Fatalf("email = %q, want normalized email", registered.User.Email)
	}
	if registered.Workspace.OwnerUserID != registered.User.ID {
		t.Fatalf("workspace owner = %q, want %q", registered.Workspace.OwnerUserID, registered.User.ID)
	}
	if registered.RefreshToken == "" || registered.AccessToken == "" {
		t.Fatal("Register() returned empty tokens")
	}

	rotated, err := service.Refresh(context.Background(), registered.RefreshToken)
	if err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	if rotated.RefreshToken == registered.RefreshToken {
		t.Fatal("Refresh() reused the same refresh token")
	}

	_, err = service.Refresh(context.Background(), registered.RefreshToken)
	if !errors.Is(err, ErrTokenReuse) {
		t.Fatalf("Refresh() old token error = %v, want ErrTokenReuse", err)
	}
}

// TestServiceLoginRejectsWrongPassword 验证错误密码不会签发登录凭证
func TestServiceLoginRejectsWrongPassword(t *testing.T) {
	repo := newFakeRepository()
	hasher := testPasswordHasher()
	service := NewService(
		repo,
		hasher,
		NewTokenManager("0123456789abcdef0123456789abcdef", time.Minute),
		time.Hour,
	)

	if _, err := service.Register(context.Background(), "user@example.com", "password123", "User"); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	_, err := service.Login(context.Background(), "user@example.com", "wrong-password")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("Login() error = %v, want ErrInvalidCredentials", err)
	}
}

// TestServiceRejectsDuplicateEmail 验证邮箱冲突返回稳定业务错误
func TestServiceRejectsDuplicateEmail(t *testing.T) {
	repo := newFakeRepository()
	service := NewService(
		repo,
		testPasswordHasher(),
		NewTokenManager("0123456789abcdef0123456789abcdef", time.Minute),
		time.Hour,
	)

	if _, err := service.Register(context.Background(), "user@example.com", "password123", "User"); err != nil {
		t.Fatalf("Register() first error = %v", err)
	}
	_, err := service.Register(context.Background(), "USER@example.com", "password123", "Other")
	if !errors.Is(err, ErrEmailExists) {
		t.Fatalf("Register() duplicate error = %v, want ErrEmailExists", err)
	}
}

// testPasswordHasher 返回低成本测试摘要器，避免单元测试消耗过多资源
func testPasswordHasher() *PasswordHasher {
	return NewPasswordHasher(PasswordParams{
		Memory:      8 * 1024,
		Iterations:  1,
		Parallelism: 1,
		SaltLength:  16,
		KeyLength:   32,
	})
}

type fakeRepository struct {
	mu          sync.Mutex
	usersByID   map[string]*User
	usersByMail map[string]*User
	workspaces  map[string]*Workspace
	tokens      map[string]*RefreshToken
}

// newFakeRepository 创建隔离的内存身份仓库
func newFakeRepository() *fakeRepository {
	return &fakeRepository{
		usersByID:   make(map[string]*User),
		usersByMail: make(map[string]*User),
		workspaces:  make(map[string]*Workspace),
		tokens:      make(map[string]*RefreshToken),
	}
}

// CreateIdentity 在内存中模拟身份事务写入
func (r *fakeRepository) CreateIdentity(_ context.Context, user *User, workspace *Workspace, token *RefreshToken) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.usersByMail[user.Email]; exists {
		return ErrEmailExists
	}
	r.usersByID[user.ID] = cloneUser(user)
	r.usersByMail[user.Email] = cloneUser(user)
	r.workspaces[user.ID] = cloneWorkspace(workspace)
	r.tokens[string(token.TokenHash)] = cloneToken(token)
	return nil
}

// FindUserByEmail 按邮箱返回内存用户副本
func (r *fakeRepository) FindUserByEmail(_ context.Context, email string) (*User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	user, ok := r.usersByMail[email]
	if !ok {
		return nil, ErrNotFound
	}
	return cloneUser(user), nil
}

// FindUserByID 按用户 ID 返回内存用户副本
func (r *fakeRepository) FindUserByID(_ context.Context, id string) (*User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	user, ok := r.usersByID[id]
	if !ok {
		return nil, ErrNotFound
	}
	return cloneUser(user), nil
}

// FindWorkspaceForUser 返回内存中的用户默认工作空间
func (r *fakeRepository) FindWorkspaceForUser(_ context.Context, userID string) (*Workspace, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	workspace, ok := r.workspaces[userID]
	if !ok {
		return nil, ErrNotFound
	}
	return cloneWorkspace(workspace), nil
}

// UpdateDisplayName 更新内存用户的展示名称
func (r *fakeRepository) UpdateDisplayName(_ context.Context, userID, displayName string, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	user, ok := r.usersByID[userID]
	if !ok {
		return ErrNotFound
	}
	user.DisplayName = displayName
	user.UpdatedAt = now
	r.usersByMail[user.Email] = cloneUser(user)
	return nil
}

// CreateRefreshToken 保存内存 Refresh Token
func (r *fakeRepository) CreateRefreshToken(_ context.Context, token *RefreshToken) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tokens[string(token.TokenHash)] = cloneToken(token)
	return nil
}

// FindRefreshTokenByHash 按摘要返回内存 Refresh Token 副本
func (r *fakeRepository) FindRefreshTokenByHash(_ context.Context, hash []byte) (*RefreshToken, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	token, ok := r.tokens[string(hash)]
	if !ok {
		return nil, ErrNotFound
	}
	return cloneToken(token), nil
}

// RotateRefreshToken 模拟旧 Token 撤销与新 Token 保存
func (r *fakeRepository) RotateRefreshToken(_ context.Context, oldID string, replacement *RefreshToken, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, token := range r.tokens {
		if token.ID != oldID {
			continue
		}
		if token.RevokedAt != nil {
			return ErrTokenReuse
		}
		token.RevokedAt = &now
		replacementID := replacement.ID
		token.ReplacedBy = &replacementID
		r.tokens[string(replacement.TokenHash)] = cloneToken(replacement)
		return nil
	}
	return ErrNotFound
}

// RevokeTokenFamily 撤销内存中同一族的所有有效 Token
func (r *fakeRepository) RevokeTokenFamily(_ context.Context, familyID string, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, token := range r.tokens {
		if token.FamilyID == familyID && token.RevokedAt == nil {
			token.RevokedAt = &now
		}
	}
	return nil
}

// RevokeRefreshToken 撤销内存中指定摘要的 Token
func (r *fakeRepository) RevokeRefreshToken(_ context.Context, hash []byte, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if token, ok := r.tokens[string(hash)]; ok && token.RevokedAt == nil {
		token.RevokedAt = &now
	}
	return nil
}

// cloneUser 复制用户模型，避免测试仓库暴露内部可变指针
func cloneUser(user *User) *User {
	copy := *user
	return &copy
}

// cloneWorkspace 复制工作空间模型
func cloneWorkspace(workspace *Workspace) *Workspace {
	copy := *workspace
	return &copy
}

// cloneToken 深复制 Token 摘要和可空时间字段
func cloneToken(token *RefreshToken) *RefreshToken {
	copy := *token
	copy.TokenHash = append([]byte(nil), token.TokenHash...)
	if token.RevokedAt != nil {
		revokedAt := *token.RevokedAt
		copy.RevokedAt = &revokedAt
	}
	if token.ReplacedBy != nil {
		replacedBy := *token.ReplacedBy
		copy.ReplacedBy = &replacedBy
	}
	return &copy
}
