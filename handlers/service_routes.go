package handlers

import (
	"GoShop/core"

	"github.com/gin-gonic/gin"
)

func protectedAPI(r *gin.Engine) *gin.RouterGroup {
	protected := r.Group("/api")
	protected.Use(core.AuthMiddleware(), core.RateLimitMiddleware(), core.SignAuthMiddleware())
	return protected
}

func RegisterUserServiceRoutes(r *gin.Engine) {
	api := r.Group("/api")
	auth := api.Group("/auth")
	auth.Use(core.RateLimitMiddleware())
	auth.GET("/sign-key", SignKey)
	auth.POST("/register", Register)
	auth.POST("/login", Login)
	auth.POST("/refresh", Refresh)

	protected := protectedAPI(r)
	protected.GET("/addresses", GetAddresses)
	protected.POST("/addresses", SaveAddress)
	protected.DELETE("/addresses/:id", DeleteAddress)
	protected.PUT("/addresses/:id/default", SetDefaultAddress)
}

func RegisterProductServiceRoutes(r *gin.Engine) {
	api := r.Group("/api")
	api.GET("/categories", GetCategories)
	api.GET("/products", GetProducts)
	api.GET("/products/:id", GetProduct)
}

func RegisterCartServiceRoutes(r *gin.Engine) {
	protected := protectedAPI(r)
	protected.GET("/cart", GetCart)
	protected.POST("/cart", AddOrUpdateCart)
	protected.DELETE("/cart/:skuId", RemoveFromCart)
	protected.POST("/cart/sync", SyncCart)
}

func RegisterPromotionServiceRoutes(r *gin.Engine) {
	api := r.Group("/api")
	api.GET("/coupons", GetCoupons)

	protected := protectedAPI(r)
	protected.GET("/user-coupons", GetUserCoupons)
	protected.POST("/user-coupons/receive", ReceiveCoupon)
}

func RegisterOrderServiceRoutes(r *gin.Engine) {
	protected := protectedAPI(r)
	protected.POST("/checkout/preview", PreviewCheckout)
	protected.POST("/orders", CreateOrder)
	protected.GET("/orders", GetOrders)
	protected.GET("/orders/:id", GetOrderDetail)
	protected.POST("/seckill", Seckill)
}

func RegisterPaymentServiceRoutes(r *gin.Engine) {
	r.POST("/api/payments/callback/mock", MockPaymentCallback)

	protected := protectedAPI(r)
	protected.POST("/payments", CreatePayment)
	protected.GET("/payments/:id", GetPayment)
	protected.POST("/pay", PayOrder)
}

func RegisterAfterSaleServiceRoutes(r *gin.Engine) {
	protected := protectedAPI(r)
	protected.POST("/orders/:id/refund", ApplyRefund)

	admin := protected.Group("/admin")
	admin.Use(core.AdminRequiredMiddleware())
	admin.POST("/orders/:id/refund/audit", AuditRefund)
}

func RegisterInventoryServiceRoutes(r *gin.Engine) {
	protected := protectedAPI(r)
	protected.POST("/seckill", Seckill)
}
