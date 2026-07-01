package handlers

import (
	"net/http"
	"time"

	"GoShop/core"
	"GoShop/models"

	"github.com/gin-gonic/gin"
)

type ReceiveCouponReq struct {
	CouponID uint `json:"couponId" binding:"required"`
}

// GetCoupons 获取可领取的优惠券列表
func GetCoupons(c *gin.Context) {
	if core.DB == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "数据库未就绪"})
		return
	}

	now := time.Now()
	var coupons []models.Coupon
	// 查询在有效期内的卡券
	if err := core.ReplicaDB.Where("start_time <= ? AND end_time >= ?", now, now).Find(&coupons).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "获取优惠券列表失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, coupons)
}

// GetUserCoupons 获取当前用户已拥有的可用卡券
func GetUserCoupons(c *gin.Context) {
	userIDVal, exists := c.Get("userId")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "未登录"})
		return
	}
	userID := userIDVal.(uint)

	if core.DB == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "数据库未就绪"})
		return
	}

	var userCoupons []models.UserCoupon
	// 查询未使用的、且关联的优惠券依然有效的卡券 (status = 0)
	err := core.ReplicaDB.Preload("Coupon").
		Joins("JOIN coupons ON coupons.id = user_coupons.coupon_id").
		Where("user_coupons.user_id = ? AND user_coupons.status = ? AND coupons.end_time >= ?", userID, models.UserCouponStatusAvailable, time.Now()).
		Find(&userCoupons).Error

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "获取我的卡券失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, userCoupons)
}

// ReceiveCoupon 领取优惠券
func ReceiveCoupon(c *gin.Context) {
	userIDVal, exists := c.Get("userId")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "未登录"})
		return
	}
	userID := userIDVal.(uint)

	var req ReceiveCouponReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "参数解析失败"})
		return
	}

	if core.DB == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "数据库未就绪"})
		return
	}

	// 1. 确认卡券在有效期内
	var coupon models.Coupon
	now := time.Now()
	if err := core.ReplicaDB.Where("id = ? AND start_time <= ? AND end_time >= ?", req.CouponID, now, now).First(&coupon).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "优惠券不存在或已过期，无法领取"})
		return
	}

	// 2. 检查用户是否已经领过
	var count int64
	core.DB.Model(&models.UserCoupon{}).Where("user_id = ? AND coupon_id = ?", userID, req.CouponID).Count(&count)
	if count > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "您已经领过此优惠券了"})
		return
	}

	// 3. 写入领取记录
	userCoupon := models.UserCoupon{
		UserID:   userID,
		CouponID: req.CouponID,
		Status:   models.UserCouponStatusAvailable,
	}
	if err := core.DB.Create(&userCoupon).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "领取卡券失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "优惠券领取成功"})
}
