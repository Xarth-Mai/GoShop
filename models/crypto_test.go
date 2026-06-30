package models

import (
	"testing"
)

func TestAESEncryptionDecryption(t *testing.T) {
	tests := []struct {
		name      string
		plainText string
	}{
		{
			name:      "Normal Text",
			plainText: "Hello, GoShop Enterprise World!",
		},
		{
			name:      "Empty Text",
			plainText: "",
		},
		{
			name:      "Special Characters",
			plainText: "张三-13800138000-北京市朝阳区#101!",
		},
		{
			name:      "Long Paragraph",
			plainText: "This is a long paragraph to check if AES-GCM performs correctly under larger payloads when GORM maps values.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 1. 加密
			encrypted, err := EncryptAES(tt.plainText)
			if err != nil {
				t.Fatalf("Failed to encrypt text: %v", err)
			}

			// 2. 检查空字符不加密
			if tt.plainText == "" && encrypted != "" {
				t.Errorf("Expected empty encrypted text, got %s", encrypted)
			}

			// 3. 解密
			decrypted, err := DecryptAES(encrypted)
			if err != nil {
				t.Fatalf("Failed to decrypt text: %v", err)
			}

			// 4. 比对明文
			if decrypted != tt.plainText {
				t.Errorf("Decrypted text mismatch! Expected %q, got %q", tt.plainText, decrypted)
			}
		})
	}
}

func TestAESDecryptionFailures(t *testing.T) {
	// 测试解密非 Base64 脏数据
	_, err := DecryptAES("invalid_base64_data")
	if err == nil {
		t.Error("Expected error when decrypting invalid base64 string, but got nil")
	}

	// 测试解密超短密文
	_, err = DecryptAES("YQ==") // "a" 的 Base64
	if err == nil {
		t.Error("Expected error when ciphertext is shorter than GCM nonce size, but got nil")
	}
}
