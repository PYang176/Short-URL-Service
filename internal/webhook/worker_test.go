package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/PYang176/Short-URL-Service/internal/platform/config"
)

// TestWorkerSendSignsPayload 验证 Worker 发送的请求头和 HMAC 可被接收方复算
func TestWorkerSendSignsPayload(t *testing.T) {
	const secret = "whsec_worker_test"
	var signatureValid bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("读取请求体失败: %v", err)
		}
		timestamp := r.Header.Get("X-ShortURL-Timestamp")
		mac := hmac.New(sha256.New, []byte(secret))
		_, _ = mac.Write([]byte(timestamp + "."))
		_, _ = mac.Write(body)
		expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
		signatureValid = hmac.Equal(
			[]byte(expected),
			[]byte(r.Header.Get("X-ShortURL-Signature")),
		) &&
			r.Header.Get("X-ShortURL-Event-ID") == "event-1" &&
			r.Header.Get("X-ShortURL-Event-Type") == "link.created"
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	worker, job := testWebhookWorker(t, secret, server.URL, time.Second)
	status, err := worker.send(context.Background(), job)
	if err != nil {
		t.Fatalf("send() error = %v", err)
	}
	if status == nil || *status != http.StatusNoContent {
		t.Fatalf("send() status = %v", status)
	}
	if !signatureValid {
		t.Fatal("接收方无法验证 HMAC 或事件请求头")
	}
}

// TestWorkerSendRejectsNon2xxTimeoutAndRedirect 验证异常响应不会被误判为投递成功
func TestWorkerSendRejectsNon2xxTimeoutAndRedirect(t *testing.T) {
	const secret = "whsec_worker_test"
	tests := []struct {
		name    string
		handler http.HandlerFunc
		timeout time.Duration
		status  int
	}{
		{
			name: "non 2xx",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusBadGateway)
			},
			timeout: time.Second,
			status:  http.StatusBadGateway,
		},
		{
			name: "redirect",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Location", "http://127.0.0.1/other")
				w.WriteHeader(http.StatusFound)
			},
			timeout: time.Second,
			status:  http.StatusFound,
		},
		{
			name: "timeout",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				time.Sleep(100 * time.Millisecond)
			},
			timeout: 20 * time.Millisecond,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(test.handler)
			defer server.Close()
			worker, job := testWebhookWorker(t, secret, server.URL, test.timeout)
			status, err := worker.send(context.Background(), job)
			if err == nil {
				t.Fatal("send() error = nil, want delivery failure")
			}
			if test.status == 0 {
				if status != nil {
					t.Fatalf("超时响应状态 = %v, want nil", status)
				}
				return
			}
			if status == nil || *status != test.status {
				t.Fatalf("send() status = %v, want %d", status, test.status)
			}
		})
	}
}

// testWebhookWorker 创建指向测试服务器的 Worker 和一条投递任务
func testWebhookWorker(
	t *testing.T,
	secret, endpointURL string,
	timeout time.Duration,
) (*Worker, *DeliveryJob) {
	t.Helper()
	cipher, err := NewSecretCipher(strings.Repeat("w", 32))
	if err != nil {
		t.Fatalf("NewSecretCipher() error = %v", err)
	}
	ciphertext, err := cipher.Encrypt(secret)
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	worker := NewWorker(
		&memoryRepository{},
		cipher,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		config.WebhookConfig{
			PollInterval:        time.Second,
			MaxAttempts:         3,
			RequestTimeout:      timeout,
			Lease:               30 * time.Second,
			AllowPrivateTargets: true,
		},
		nil,
	)
	lockedUntil := time.Now().UTC().Add(time.Minute)
	return worker, &DeliveryJob{
		Delivery: Delivery{
			ID:           1,
			WebhookID:    "webhook-1",
			EventID:      "event-1",
			EventType:    "link.created",
			Payload:      []byte(`{"id":"event-1","type":"link.created"}`),
			AttemptCount: 1,
			LockedUntil:  &lockedUntil,
		},
		EndpointURL:      endpointURL,
		SecretCiphertext: ciphertext,
	}
}
