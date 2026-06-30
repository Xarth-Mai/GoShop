package core

import (
	"testing"
	"time"
)

func TestTokenLifecycle(t *testing.T) {
	userID := uint(99)
	username := "test_runner"

	// 1. 生成 Access Token
	token, err := GenerateToken(userID, username, time.Minute, "access")
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	// 2. 正常解析校验
	payload, err := ParseAndVerifyToken(token, "access")
	if err != nil {
		t.Fatalf("Failed to verify token: %v", err)
	}

	if payload.UserID != userID || payload.Username != username {
		t.Errorf("Token payload values mismatch! Expected ID %d and username %q, got ID %d and %q",
			userID, username, payload.UserID, payload.Username)
	}

	// 3. 校验 Token 类型不匹配 (Expected "refresh" but parsed "access")
	_, err = ParseAndVerifyToken(token, "refresh")
	if err == nil {
		t.Error("Expected failure when parsing access token as refresh token, but got success")
	}

	// 4. 使用已过期 Token 校验过期逻辑，避免测试依赖真实等待
	expiredToken, err := GenerateToken(userID, username, -time.Second, "access")
	if err != nil {
		t.Fatalf("Failed to generate expired token: %v", err)
	}
	_, err = ParseAndVerifyToken(expiredToken, "access")
	if err == nil || err.Error() != "token has expired" {
		t.Errorf("Expected token expiration error, got: %v", err)
	}
}

func TestTokenSignatureTampering(t *testing.T) {
	userID := uint(101)
	username := "security_check"

	token, err := GenerateToken(userID, username, 10*time.Second, "access")
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	// 1. 测试损坏 Token 格式
	_, err = ParseAndVerifyToken("invalid_token_format", "access")
	if err == nil {
		t.Error("Expected error for bad token format, got nil")
	}

	// 2. 模拟篡改签名
	tamperedToken := token + "added_garbage"
	_, err = ParseAndVerifyToken(tamperedToken, "access")
	if err == nil || err.Error() != "invalid token signature" {
		t.Errorf("Expected invalid signature error, got: %v", err)
	}
}
