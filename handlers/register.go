package handlers

import (
	"GoShop/core"

	"github.com/gin-gonic/gin"
)

// RegisterRoutes 注册系统所有重构后的 API 路由
func RegisterRoutes(r *gin.Engine) {
	// 普罗米修斯监控指标端点 (公开，由 Prometheus 定时抓取)
	r.GET("/metrics", core.ExportMetrics)

	// API 路由分组
	api := r.Group("/api")
	{
		// 1. 公开身份认证接口 (无需鉴权，但加限流防护)
		auth := api.Group("/auth")
		auth.Use(core.RateLimitMiddleware())
		{
			auth.POST("/register", Register)
			auth.POST("/login", Login)
			auth.POST("/refresh", Refresh)
		}

		// 2. 鉴权与安全防护核心路由组 (引入 Token 验证、限流、以及防篡改防重放签名校验)
		protected := api.Group("")
		protected.Use(core.AuthMiddleware(), core.RateLimitMiddleware(), core.SignAuthMiddleware())
		{
			// 收货地址管理
			protected.GET("/addresses", GetAddresses)
			protected.POST("/addresses", SaveAddress)
			protected.DELETE("/addresses/:id", DeleteAddress)
			protected.PUT("/addresses/:id/default", SetDefaultAddress)

			// 优惠券管理
			protected.GET("/coupons", GetCoupons)
			protected.GET("/user-coupons", GetUserCoupons)
			protected.POST("/user-coupons/receive", ReceiveCoupon)

			// 购物车管理
			protected.GET("/cart", GetCart)
			protected.POST("/cart", AddOrUpdateCart)
			protected.DELETE("/cart/:skuId", RemoveFromCart)
			protected.POST("/cart/sync", SyncCart)

			// 订单管理
			protected.POST("/orders", CreateOrder)
			protected.GET("/orders", GetOrders)
			protected.POST("/orders/:id/refund", ApplyRefund)

			// 高并发秒杀及支付确认
			protected.POST("/seckill", Seckill)
			protected.POST("/pay", PayOrder)

			// 商家售后退款审核
			protected.POST("/admin/orders/:id/refund/audit", AuditRefund)
		}
	}
}
