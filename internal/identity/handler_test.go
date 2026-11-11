package identity

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// TestHandlerRegisterLoginAndProfile 验证注册、登录和资料 HTTP 链路
func TestHandlerRegisterLoginAndProfile(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := testIdentityService()
	handler := NewHandler(service)

	router := gin.New()
	router.POST("/register", handler.Register)
	router.POST("/login", handler.Login)
	router.GET("/me", func(c *gin.Context) {
		c.Set(ContextUserID, c.GetHeader("X-Test-User-ID"))
		handler.Profile(c)
	})

	registerResponse := performJSONRequest(
		t,
		router,
		http.MethodPost,
		"/register",
		`{"email":"handler@example.com","password":"password123","display_name":"Handler"}`,
		nil,
	)
	if registerResponse.Code != http.StatusCreated {
		t.Fatalf("register status = %d, want %d; body=%s", registerResponse.Code, http.StatusCreated, registerResponse.Body.String())
	}

	var registered authResponse
	if err := json.Unmarshal(registerResponse.Body.Bytes(), &registered); err != nil {
		t.Fatalf("decode register response: %v", err)
	}
	if registered.Token.AccessToken == "" || registered.Token.RefreshToken == "" {
		t.Fatal("register response is missing tokens")
	}

	loginResponse := performJSONRequest(
		t,
		router,
		http.MethodPost,
		"/login",
		`{"email":"handler@example.com","password":"password123"}`,
		nil,
	)
	if loginResponse.Code != http.StatusOK {
		t.Fatalf("login status = %d, want %d; body=%s", loginResponse.Code, http.StatusOK, loginResponse.Body.String())
	}

	profileResponse := performJSONRequest(
		t,
		router,
		http.MethodGet,
		"/me",
		"",
		map[string]string{"X-Test-User-ID": registered.User.ID},
	)
	if profileResponse.Code != http.StatusOK {
		t.Fatalf("profile status = %d, want %d; body=%s", profileResponse.Code, http.StatusOK, profileResponse.Body.String())
	}
}

// TestHandlerMapsErrors 验证非法请求和错误凭证的 HTTP 状态映射
func TestHandlerMapsErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewHandler(testIdentityService())
	router := gin.New()
	router.POST("/register", handler.Register)
	router.POST("/login", handler.Login)

	badRequest := performJSONRequest(t, router, http.MethodPost, "/register", `{`, nil)
	if badRequest.Code != http.StatusBadRequest {
		t.Fatalf("malformed request status = %d, want 400", badRequest.Code)
	}

	invalidLogin := performJSONRequest(
		t,
		router,
		http.MethodPost,
		"/login",
		`{"email":"missing@example.com","password":"password123"}`,
		nil,
	)
	if invalidLogin.Code != http.StatusUnauthorized {
		t.Fatalf("invalid login status = %d, want 401", invalidLogin.Code)
	}
}

// testIdentityService 创建使用内存仓库和低成本摘要的测试服务
func testIdentityService() *Service {
	return NewService(
		newFakeRepository(),
		testPasswordHasher(),
		NewTokenManager("0123456789abcdef0123456789abcdef", 15*time.Minute),
		time.Hour,
	)
}

// performJSONRequest 构造并执行测试 HTTP 请求
func performJSONRequest(
	t *testing.T,
	handler http.Handler,
	method, path, body string,
	headers map[string]string,
) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}
