package handlers

import (
	"net/http"
	"time"

	"GoShop/config"
	"GoShop/core"
	"GoShop/models"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

type RegisterReq struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
	Email    string `json:"email"`
}

type LoginReq struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type RefreshReq struct {
	RefreshToken string `json:"refreshToken" binding:"required"`
}

// Register 注册接口
func Register(c *gin.Context) {
	var req RegisterReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "参数解析失败"})
		return
	}

	if core.DB == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "数据库未就绪"})
		return
	}

	// 检查用户名是否存在
	var count int64
	core.DB.Model(&models.User{}).Where("username = ?", req.Username).Count(&count)
	if count > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "用户名已存在"})
		return
	}

	// 密码 Hash 加密
	hashed, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "密码加密失败"})
		return
	}

	user := models.User{
		Username:     req.Username,
		PasswordHash: string(hashed),
		Email:        req.Email,
	}

	if err := core.DB.Create(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "用户注册失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "注册成功"})
}

// Login 登录接口
func Login(c *gin.Context) {
	var req LoginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "参数解析失败"})
		return
	}

	if core.DB == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "数据库未就绪"})
		return
	}

	var user models.User
	if err := core.ReplicaDB.Where("username = ?", req.Username).First(&user).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "用户名或密码错误"})
		return
	}

	// 验证密码
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "用户名或密码错误"})
		return
	}

	// 签发 Token
	cfg := config.GlobalConfig.JWT
	accessToken, err := core.GenerateToken(user.ID, user.Username, time.Duration(cfg.Expire)*time.Second, "access")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "生成访问凭证失败"})
		return
	}

	refreshToken, err := core.GenerateToken(user.ID, user.Username, time.Duration(cfg.RefreshExpire)*time.Second, "refresh")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "生成刷新凭证失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":       "success",
		"username":     user.Username,
		"accessToken":  accessToken,
		"refreshToken": refreshToken,
	})
}

// Refresh 无感刷新凭证
func Refresh(c *gin.Context) {
	var req RefreshReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "参数解析失败"})
		return
	}

	payload, err := core.ParseAndVerifyToken(req.RefreshToken, "refresh")
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "刷新凭证无效或已过期: " + err.Error()})
		return
	}

	cfg := config.GlobalConfig.JWT
	newAccessToken, err := core.GenerateToken(payload.UserID, payload.Username, time.Duration(cfg.Expire)*time.Second, "access")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "生成访问凭证失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":      "success",
		"accessToken": newAccessToken,
	})
}
