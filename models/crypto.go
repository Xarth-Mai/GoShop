package models

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"database/sql/driver"
	"encoding/base64"
	"errors"
	"fmt"
	"io"

	"GoShop/config"
)

// ==========================================
// AES-GCM 敏感数据加解密与 GORM 自定义类型
// ==========================================

// getAESKey 根据全局 Secret 生成 32 字节 AES 密钥
func getAESKey() []byte {
	secret := "goshop_default_secret_key_32_bytes"
	if config.GlobalConfig != nil && config.GlobalConfig.JWT.Secret != "" {
		secret = config.GlobalConfig.JWT.Secret
	}
	hash := sha256.Sum256([]byte(secret))
	return hash[:]
}

// EncryptAES 采用 AES-GCM 模式加密明文，返回 Base64 字符串
func EncryptAES(plainText string) (string, error) {
	if plainText == "" {
		return "", nil
	}
	key := getAESKey()
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	cipherText := gcm.Seal(nonce, nonce, []byte(plainText), nil)
	return base64.StdEncoding.EncodeToString(cipherText), nil
}

// DecryptAES 采用 AES-GCM 模式解密 Base64 密文，返回明文
func DecryptAES(cryptoText string) (string, error) {
	if cryptoText == "" {
		return "", nil
	}
	key := getAESKey()
	data, err := base64.StdEncoding.DecodeString(cryptoText)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", errors.New("ciphertext too short")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plainText, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}

	return string(plainText), nil
}

// EncryptedString 自定义 GORM 加密字符串类型
type EncryptedString string

// Value 存库加密
func (es EncryptedString) Value() (driver.Value, error) {
	valStr := string(es)
	if valStr == "" {
		return valStr, nil
	}
	encrypted, err := EncryptAES(valStr)
	if err != nil {
		return nil, fmt.Errorf("GORM value encryption failed: %v", err)
	}
	return encrypted, nil
}

// Scan 出库解密
func (es *EncryptedString) Scan(value interface{}) error {
	if value == nil {
		*es = ""
		return nil
	}
	var cryptoText string
	switch v := value.(type) {
	case string:
		cryptoText = v
	case []byte:
		cryptoText = string(v)
	default:
		return fmt.Errorf("unsupported type for EncryptedString: %T", value)
	}

	if cryptoText == "" {
		*es = ""
		return nil
	}

	decrypted, err := DecryptAES(cryptoText)
	if err != nil {
		// 若解密失败，保留原密文，避免崩溃，但记录错误
		*es = EncryptedString(cryptoText)
		return nil
	}
	*es = EncryptedString(decrypted)
	return nil
}
