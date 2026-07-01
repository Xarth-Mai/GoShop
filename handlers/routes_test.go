package handlers

import (
	"testing"

	"github.com/gin-gonic/gin"
)

func assertRouteRegistered(t *testing.T, routes gin.RoutesInfo, method, path string) {
	t.Helper()
	found := false
	for _, route := range routes {
		if route.Method == method && route.Path == path {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected route %s %s to be registered, but it was not", method, path)
	}
}

func TestRegisterRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("RegisterRoutes", func(t *testing.T) {
		r := gin.New()
		RegisterRoutes(r)
		routes := r.Routes()

		// 检查部分核心路由是否注册成功
		assertRouteRegistered(t, routes, "GET", "/metrics")
		assertRouteRegistered(t, routes, "POST", "/api/auth/login")
		assertRouteRegistered(t, routes, "GET", "/api/addresses")
		assertRouteRegistered(t, routes, "POST", "/api/orders")
		assertRouteRegistered(t, routes, "POST", "/api/admin/orders/:id/refund/audit")
	})

	t.Run("RegisterUserServiceRoutes", func(t *testing.T) {
		r := gin.New()
		RegisterUserServiceRoutes(r)
		routes := r.Routes()

		assertRouteRegistered(t, routes, "POST", "/api/auth/login")
		assertRouteRegistered(t, routes, "GET", "/api/addresses")
		assertRouteRegistered(t, routes, "PUT", "/api/addresses/:id/default")
	})

	t.Run("RegisterProductServiceRoutes", func(t *testing.T) {
		r := gin.New()
		RegisterProductServiceRoutes(r)
		routes := r.Routes()

		assertRouteRegistered(t, routes, "GET", "/api/categories")
		assertRouteRegistered(t, routes, "GET", "/api/products")
		assertRouteRegistered(t, routes, "GET", "/api/products/:id")
	})

	t.Run("RegisterCartServiceRoutes", func(t *testing.T) {
		r := gin.New()
		RegisterCartServiceRoutes(r)
		routes := r.Routes()

		assertRouteRegistered(t, routes, "GET", "/api/cart")
		assertRouteRegistered(t, routes, "POST", "/api/cart")
		assertRouteRegistered(t, routes, "DELETE", "/api/cart/:skuId")
	})

	t.Run("RegisterPromotionServiceRoutes", func(t *testing.T) {
		r := gin.New()
		RegisterPromotionServiceRoutes(r)
		routes := r.Routes()

		assertRouteRegistered(t, routes, "GET", "/api/coupons")
		assertRouteRegistered(t, routes, "GET", "/api/user-coupons")
		assertRouteRegistered(t, routes, "POST", "/api/user-coupons/receive")
	})

	t.Run("RegisterOrderServiceRoutes", func(t *testing.T) {
		r := gin.New()
		RegisterOrderServiceRoutes(r)
		routes := r.Routes()

		assertRouteRegistered(t, routes, "POST", "/api/checkout/preview")
		assertRouteRegistered(t, routes, "POST", "/api/orders")
		assertRouteRegistered(t, routes, "GET", "/api/orders/:id")
		assertRouteRegistered(t, routes, "POST", "/api/seckill")
	})

	t.Run("RegisterPaymentServiceRoutes", func(t *testing.T) {
		r := gin.New()
		RegisterPaymentServiceRoutes(r)
		routes := r.Routes()

		assertRouteRegistered(t, routes, "POST", "/api/payments/callback/mock")
		assertRouteRegistered(t, routes, "POST", "/api/payments")
		assertRouteRegistered(t, routes, "POST", "/api/pay")
	})

	t.Run("RegisterAfterSaleServiceRoutes", func(t *testing.T) {
		r := gin.New()
		RegisterAfterSaleServiceRoutes(r)
		routes := r.Routes()

		assertRouteRegistered(t, routes, "POST", "/api/orders/:id/refund")
		assertRouteRegistered(t, routes, "POST", "/api/admin/orders/:id/refund/audit")
	})

	t.Run("RegisterInventoryServiceRoutes", func(t *testing.T) {
		r := gin.New()
		RegisterInventoryServiceRoutes(r)
		routes := r.Routes()

		assertRouteRegistered(t, routes, "POST", "/api/seckill")
	})
}
