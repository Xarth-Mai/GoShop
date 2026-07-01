package core

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"GoShop/config"
	"GoShop/models"
)

// ==========================================
// 原生 JWT (HMAC-SHA256 Token) 实现
// ==========================================

type TokenPayload struct {
	UserID    uint   `json:"userId"`
	Username  string `json:"username"`
	Role      string `json:"role"`
	Exp       int64  `json:"exp"`
	TokenType string `json:"type"` // "access" 或 "refresh"
}

// GenerateToken 签发 Token (Access 或 Refresh)
func GenerateToken(userID uint, username string, duration time.Duration, tokenType string) (string, error) {
	return GenerateTokenWithRole(userID, username, models.UserRoleUser, duration, tokenType)
}

// GenerateTokenWithRole 签发带角色信息的 Token。
func GenerateTokenWithRole(userID uint, username, role string, duration time.Duration, tokenType string) (string, error) {
	secret := "goshop_jwt_hmac_secret"
	if config.GlobalConfig != nil && config.GlobalConfig.JWT.Secret != "" {
		secret = config.GlobalConfig.JWT.Secret
	}
	if role == "" {
		role = models.UserRoleUser
	}

	payload := TokenPayload{
		UserID:    userID,
		Username:  username,
		Role:      role,
		Exp:       time.Now().Add(duration).Unix(),
		TokenType: tokenType,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	payloadBase64 := base64.RawURLEncoding.EncodeToString(payloadBytes)

	// 计算 HMAC 签名
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(payloadBase64))
	signature := hex.EncodeToString(h.Sum(nil))

	tokenStr := payloadBase64 + "." + signature
	return tokenStr, nil
}

// ParseAndVerifyToken 校验并解析 Token
func ParseAndVerifyToken(tokenStr string, expectedType string) (*TokenPayload, error) {
	parts := strings.Split(tokenStr, ".")
	if len(parts) != 2 {
		return nil, errors.New("invalid token format")
	}

	payloadBase64, clientSignature := parts[0], parts[1]

	secret := "goshop_jwt_hmac_secret"
	if config.GlobalConfig != nil && config.GlobalConfig.JWT.Secret != "" {
		secret = config.GlobalConfig.JWT.Secret
	}

	// 重新计算 HMAC 并校验签名
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(payloadBase64))
	serverSignature := hex.EncodeToString(h.Sum(nil))

	if !hmac.Equal([]byte(clientSignature), []byte(serverSignature)) {
		return nil, errors.New("invalid token signature")
	}

	// 解析 payload
	payloadBytes, err := base64.RawURLEncoding.DecodeString(payloadBase64)
	if err != nil {
		return nil, errors.New("failed to decode token payload")
	}

	var payload TokenPayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return nil, errors.New("failed to unmarshal token payload")
	}
	if payload.Role == "" {
		payload.Role = models.UserRoleUser
	}

	// 校验有效期
	if time.Now().Unix() > payload.Exp {
		return nil, errors.New("token has expired")
	}

	// 校验 Token 类型
	if payload.TokenType != expectedType {
		return nil, fmt.Errorf("invalid token type: expected %s, got %s", expectedType, payload.TokenType)
	}

	return &payload, nil
}
