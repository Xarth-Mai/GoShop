package core

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"GoShop/config"

	"github.com/gin-gonic/gin"
)

// ==========================================
// 1. AuthMiddleware JWT 认证中间件
// ==========================================
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"message": "未登录，请先登录"})
			c.Abort()
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if !(len(parts) == 2 && parts[0] == "Bearer") {
			c.JSON(http.StatusUnauthorized, gin.H{"message": "Authorization 格式错误"})
			c.Abort()
			return
		}

		tokenStr := parts[1]
		payload, err := ParseAndVerifyToken(tokenStr, "access")
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"message": "登录已失效，请重新登录: " + err.Error()})
			c.Abort()
			return
		}

		// 将用户信息存入上下文
		c.Set("userId", payload.UserID)
		c.Set("username", payload.Username)
		c.Next()
	}
}

// ==========================================
// 2. RateLimitMiddleware 令牌桶/滑动窗口限流 + IP黑名单中间件
// ==========================================
func RateLimitMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if RedisClient == nil {
			c.Next()
			return
		}

		ctx := context.Background()
		clientIP := c.ClientIP()

		// 2.1 检查是否在黑名单中
		blacklistKey := "blacklist:ip:" + clientIP
		isBlack, err := RedisClient.Exists(ctx, blacklistKey).Result()
		if err == nil && isBlack > 0 {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"message": "您的请求过于频繁，已被系统封锁，请60秒后再试",
			})
			c.Abort()
			return
		}

		// 2.2 滑动窗口频次限制：限制单 IP 每秒最多访问 20 次
		limitKey := "limit:ip:" + clientIP
		count, err := RedisClient.Incr(ctx, limitKey).Result()
		if err != nil {
			c.Next()
			return
		}

		if count == 1 {
			RedisClient.Expire(ctx, limitKey, 1*time.Second)
		}

		if count > 20 {
			// 触发惩罚：加入黑名单封禁 60 秒
			RedisClient.Set(ctx, blacklistKey, "1", 60*time.Second)
			c.JSON(http.StatusTooManyRequests, gin.H{
				"message": "检测到恶意高频刷单，已被限流封锁 60 秒！",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

func RequestSignSecret() string {
	secret := "goshop_jwt_hmac_secret"
	if config.GlobalConfig != nil && config.GlobalConfig.JWT.Secret != "" {
		secret = config.GlobalConfig.JWT.Secret
	}

	sum := sha256.Sum256([]byte("goshop-request-signing:" + secret))
	return hex.EncodeToString(sum[:])
}

// ==========================================
// 3. SignAuthMiddleware 接口防篡改与防重放中间件
// ==========================================
func SignAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 仅对写操作（POST、PUT、DELETE）和特定敏感路由进行签名验证
		if c.Request.Method != "POST" && c.Request.Method != "PUT" && c.Request.Method != "DELETE" {
			c.Next()
			return
		}

		// 如果是系统重置接口，免签放行以方便测试
		if c.Request.URL.Path == "/api/reset" {
			c.Next()
			return
		}

		timestampStr := c.GetHeader("X-Timestamp")
		nonce := c.GetHeader("X-Nonce")
		clientSign := c.GetHeader("X-Sign")

		if timestampStr == "" || nonce == "" || clientSign == "" {
			c.JSON(http.StatusBadRequest, gin.H{"message": "安全验证字段缺失"})
			c.Abort()
			return
		}

		// 3.1 验证时间戳，限 60 秒内
		clientTimeUnix, err := strconv.ParseInt(timestampStr, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": "无效的时间戳"})
			c.Abort()
			return
		}

		serverTimeUnix := time.Now().UnixNano() / int64(time.Millisecond) // 毫秒级时间戳
		diff := serverTimeUnix - clientTimeUnix
		if diff < 0 {
			diff = -diff
		}
		if diff > 60000 { // 超过 60 秒
			c.JSON(http.StatusForbidden, gin.H{"message": "请求已超时，疑似重放攻击"})
			c.Abort()
			return
		}

		// 3.2 验证 Nonce，防止重放
		if RedisClient != nil {
			ctx := context.Background()
			nonceKey := "nonce:" + nonce
			setOk, err := RedisClient.SetNX(ctx, nonceKey, "1", 60*time.Second).Result()
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"message": "系统防重放校验异常"})
				c.Abort()
				return
			}
			if !setOk {
				c.JSON(http.StatusForbidden, gin.H{"message": "检测到重复请求，疑似重放攻击"})
				c.Abort()
				return
			}
		}

		// 3.3 验证签名 (Signature)
		// 读取 Body 以防止中途参数被篡改。
		var bodyBytes []byte
		if c.Request.Body != nil {
			bodyBytes, _ = io.ReadAll(c.Request.Body)
			// 必须把读取出的 body 重写回去，否则后续 Handler 无法拿到 body
			c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		}

		// 签名计算规则：hmac_sha256(signSecret, timestamp + nonce + path + body)
		path := c.Request.URL.Path
		mac := hmac.New(sha256.New, []byte(RequestSignSecret()))
		mac.Write([]byte(timestampStr + nonce + path + string(bodyBytes)))
		expectedSign := hex.EncodeToString(mac.Sum(nil))

		if clientSign != expectedSign {
			c.JSON(http.StatusForbidden, gin.H{
				"message":      "接口数据签名验证失败，请求可能已被串改",
				"expectedSign": expectedSign, // 方便本地调试对齐
			})
			c.Abort()
			return
		}

		c.Next()
	}
}
