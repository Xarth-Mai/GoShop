package handlers

import (
	"net/http"
	"strconv"
	"time"

	"GoShop/core"
	"GoShop/internal/inventory"
	"GoShop/internal/promotion"
	"GoShop/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// RegisterInternalRoutes 注册微服务间安全同步通信接口
func RegisterInternalRoutes(r *gin.Engine) {
	internal := r.Group("/api/internal")
	internal.Use(core.InternalOnlyMiddleware())
	{
		// 1. 商品服务接口
		internal.GET("/products/:id", internalGetProductSku)
		internal.GET("/products/:id/cart-summary", internalGetProductCartSummary)

		// 1.5 订单服务接口
		internal.GET("/orders/:id/payment-source", internalGetOrderPaymentSource)

		// 2. 库存服务接口
		internal.POST("/inventory/reserve", internalReserveStock)
		internal.POST("/inventory/release", internalReleaseStock)

		// 3. 优惠券/营销服务接口
		internal.POST("/promotion/lock", internalLockCoupon)
		internal.POST("/promotion/release", internalReleaseCoupon)
		internal.POST("/promotion/candidates", internalGetCouponCandidates)
	}
}

// ----------------------------------------------------
// 1. 商品微服务内部接口
// ----------------------------------------------------
func internalGetProductSku(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid product/sku id"})
		return
	}

	var sku models.Sku
	if err := core.ReplicaDB.Where("id = ?", id).First(&sku).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "sku not found"})
		return
	}

	c.JSON(http.StatusOK, sku)
}

type InternalProductCartSummary struct {
	SkuID   uint   `json:"skuId"`
	SpuID   uint   `json:"spuId"`
	SpuName string `json:"spuName"`
	SkuName string `json:"skuName"`
	Price   int    `json:"price"`
	Image   string `json:"image"`
}

type InternalOrderPaymentSource struct {
	OrderID      string     `json:"orderId"`
	UserID       uint       `json:"userId"`
	TotalAmount  int        `json:"totalAmount"`
	Status       int        `json:"status"`
	PayStatus    int        `json:"payStatus"`
	UserCouponID uint       `json:"userCouponId"`
	PayExpireAt  *time.Time `json:"payExpireAt,omitempty"`
}

func internalGetOrderPaymentSource(c *gin.Context) {
	orderID := c.Param("id")
	userIDValue := uint(0)
	if userIDRaw := c.Query("userId"); userIDRaw != "" {
		userID, err := strconv.Atoi(userIDRaw)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
			return
		}
		userIDValue = uint(userID)
	}

	query := core.ReplicaDB.Where("id = ?", orderID)
	if userIDValue > 0 {
		query = query.Where("user_id = ?", userIDValue)
	}

	var order models.Order
	if err := query.First(&order).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "order not found"})
		return
	}

	c.JSON(http.StatusOK, InternalOrderPaymentSource{
		OrderID:      order.ID,
		UserID:       order.UserID,
		TotalAmount:  order.TotalAmount,
		Status:       order.Status,
		PayStatus:    order.PayStatus,
		UserCouponID: order.UserCouponID,
		PayExpireAt:  order.PayExpireAt,
	})
}

func internalGetProductCartSummary(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid sku id"})
		return
	}

	var sku models.Sku
	if err := core.ReplicaDB.Where("id = ?", id).First(&sku).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "sku not found"})
		return
	}

	var spu models.Spu
	if err := core.ReplicaDB.Where("id = ?", sku.SpuID).First(&spu).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "spu not found"})
		return
	}

	c.JSON(http.StatusOK, InternalProductCartSummary{
		SkuID:   sku.ID,
		SpuID:   sku.SpuID,
		SpuName: spu.Name,
		SkuName: sku.Title,
		Price:   sku.Price,
		Image:   spu.MainImage,
	})
}

// ----------------------------------------------------
// 2. 库存微服务内部接口
// ----------------------------------------------------
type InternalReserveItem struct {
	SkuID int `json:"skuId"`
	Qty   int `json:"qty"`
}

type InternalReserveReq struct {
	OrderID string                `json:"orderId"`
	UserID  uint                  `json:"userId"`
	Items   []InternalReserveItem `json:"items"`
}

func internalReserveStock(c *gin.Context) {
	var req InternalReserveReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.OrderID == "" || len(req.Items) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing orderId or items"})
		return
	}

	// 转化为 inventory 服务内部的 ReserveItem 结构
	reserveItems := make([]inventory.ReserveItem, len(req.Items))
	for i, item := range req.Items {
		reserveItems[i] = inventory.ReserveItem{
			SkuID:    uint(item.SkuID),
			Quantity: item.Qty,
		}
	}

	// 事务锁定预占库存 (默认 30 分钟过期)
	expireAt := time.Now().Add(30 * time.Minute)
	err := core.DB.Transaction(func(tx *gorm.DB) error {
		svc := inventory.NewService(tx)
		return svc.ReserveStock(tx, req.OrderID, req.UserID, reserveItems, expireAt)
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success"})
}

type InternalReleaseStockReq struct {
	OrderID string `json:"orderId"`
}

func internalReleaseStock(c *gin.Context) {
	var req InternalReleaseStockReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	err := core.DB.Transaction(func(tx *gorm.DB) error {
		svc := inventory.NewService(tx)
		return svc.ReleaseOrderReservations(tx, req.OrderID)
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success"})
}

// ----------------------------------------------------
// 3. 优惠券/营销微服务内部接口
// ----------------------------------------------------
type InternalLockCouponReq struct {
	UserID       uint   `json:"userId"`
	UserCouponID uint   `json:"userCouponId"`
	OrderID      string `json:"orderId"`
	Subtotal     int    `json:"subtotal"`
}

func internalLockCoupon(c *gin.Context) {
	var req InternalLockCouponReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var discountAmount int
	err := core.DB.Transaction(func(tx *gorm.DB) error {
		svc := promotion.NewService(tx)
		discount, err := svc.LockCouponForOrder(tx, req.UserID, req.UserCouponID, req.OrderID, req.Subtotal)
		if err != nil {
			return err
		}
		discountAmount = discount
		return nil
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"discountAmount": discountAmount})
}

type InternalReleaseCouponReq struct {
	UserCouponID uint   `json:"userCouponId"`
	OrderID      string `json:"orderId"`
}

func internalReleaseCoupon(c *gin.Context) {
	var req InternalReleaseCouponReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	err := core.DB.Transaction(func(tx *gorm.DB) error {
		svc := promotion.NewService(tx)
		return svc.ReleaseCouponLock(tx, req.UserCouponID, req.OrderID)
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success"})
}

type InternalCandidatesReq struct {
	UserID               uint `json:"userId"`
	SelectedUserCouponID uint `json:"selectedUserCouponId"`
	Subtotal             int  `json:"subtotal"`
}

func internalGetCouponCandidates(c *gin.Context) {
	var req InternalCandidatesReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	candidates := promotion.NewService(core.ReplicaDB).CouponCandidates(req.UserID, req.SelectedUserCouponID, req.Subtotal)
	c.JSON(http.StatusOK, candidates)
}
