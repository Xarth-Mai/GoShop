package handlers

import (
	"GoShop/core"
	_ "GoShop/docs"
	"net/http"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
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

	RegisterInternalRoutes(r)
}

func RegisterProductServiceRoutes(r *gin.Engine) {
	api := r.Group("/api")
	api.GET("/categories", GetCategories)
	api.GET("/products", GetProducts)
	api.GET("/products/:id", GetProduct)

	RegisterInternalRoutes(r)
}

func RegisterCartServiceRoutes(r *gin.Engine) {
	protected := protectedAPI(r)
	protected.GET("/cart", GetCart)
	protected.POST("/cart", AddOrUpdateCart)
	protected.DELETE("/cart/:skuId", RemoveFromCart)
	protected.POST("/cart/sync", SyncCart)

	RegisterInternalRoutes(r)
}

func RegisterPromotionServiceRoutes(r *gin.Engine) {
	api := r.Group("/api")
	api.GET("/coupons", GetCoupons)

	protected := protectedAPI(r)
	protected.GET("/user-coupons", GetUserCoupons)
	protected.POST("/user-coupons/receive", ReceiveCoupon)

	RegisterInternalRoutes(r)
}

func RegisterOrderServiceRoutes(r *gin.Engine) {
	protected := protectedAPI(r)
	protected.POST("/checkout/preview", PreviewCheckout)
	protected.POST("/orders", CreateOrder)
	protected.GET("/orders", GetOrders)
	protected.GET("/orders/:id", GetOrderDetail)
	protected.POST("/seckill", Seckill)

	r.GET("/api/metrics", GetMetrics)

	r.GET("/swagger", func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, "/swagger/index.html")
	})
	r.GET("/swagger/*any", func(c *gin.Context) {
		if c.Param("any") == "" || c.Param("any") == "/" {
			c.Redirect(http.StatusMovedPermanently, "/swagger/index.html")
			return
		}
		ginSwagger.WrapHandler(swaggerFiles.Handler)(c)
	})

	RegisterInternalRoutes(r)
}

func RegisterPaymentServiceRoutes(r *gin.Engine) {
	r.POST("/api/payments/callback/mock", MockPaymentCallback)

	protected := protectedAPI(r)
	protected.POST("/payments", CreatePayment)
	protected.GET("/payments/:id", GetPayment)
	protected.POST("/pay", PayOrder)

	RegisterInternalRoutes(r)
}

func RegisterAfterSaleServiceRoutes(r *gin.Engine) {
	protected := protectedAPI(r)
	protected.POST("/orders/:id/refund", ApplyRefund)

	admin := protected.Group("/admin")
	admin.Use(core.AdminRequiredMiddleware())
	admin.POST("/orders/:id/refund/audit", AuditRefund)

	RegisterInternalRoutes(r)
}

func RegisterInventoryServiceRoutes(r *gin.Engine) {
	protected := protectedAPI(r)
	protected.POST("/seckill", Seckill)

	RegisterInternalRoutes(r)
}
