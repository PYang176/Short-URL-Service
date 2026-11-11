package identity

import "testing"

// TestPasswordHasher 验证随机盐、正确密码和错误密码的摘要行为
func TestPasswordHasher(t *testing.T) {
	hasher := NewPasswordHasher(PasswordParams{
		Memory:      8 * 1024,
		Iterations:  1,
		Parallelism: 1,
		SaltLength:  16,
		KeyLength:   32,
	})

	firstHash, err := hasher.Hash("correct horse battery staple")
	if err != nil {
		t.Fatalf("Hash() error = %v", err)
	}
	secondHash, err := hasher.Hash("correct horse battery staple")
	if err != nil {
		t.Fatalf("Hash() second error = %v", err)
	}
	if firstHash == secondHash {
		t.Fatal("two hashes of the same password must use different salts")
	}

	matches, err := hasher.Verify("correct horse battery staple", firstHash)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if !matches {
		t.Fatal("Verify() = false, want true")
	}

	matches, err = hasher.Verify("wrong password", firstHash)
	if err != nil {
		t.Fatalf("Verify() wrong password error = %v", err)
	}
	if matches {
		t.Fatal("Verify() wrong password = true, want false")
	}
}
