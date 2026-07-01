package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"GoShop/core"
	"GoShop/internal/inventory"
	ordersvc "GoShop/internal/order"
	"GoShop/internal/promotion"
	"GoShop/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
		internal.GET("/orders/expired-pending", internalGetExpiredPendingOrders)
		internal.GET("/orders/:id/payment-source", internalGetOrderPaymentSource)
		internal.GET("/orders/:id/refund-source", internalGetOrderRefundSource)
		internal.POST("/orders/:id/cancel-pending", internalCancelPendingOrder)
		internal.POST("/orders/:id/refund-apply", internalApplyOrderRefund)
		internal.POST("/orders/:id/refund-complete", internalCompleteOrderRefund)
		internal.POST("/orders/:id/refund-reject", internalRejectOrderRefund)

		// 1.6 用户与购物车服务接口
		internal.GET("/addresses/:id/snapshot", internalGetAddressSnapshot)
		internal.POST("/cart/clear-items", internalClearCartItems)

		// 2. 库存服务接口
		internal.POST("/inventory/reserve", internalReserveStock)
		internal.POST("/inventory/release", internalReleaseStock)
		internal.POST("/inventory/restock", internalRestockStock)
		internal.GET("/inventory/reservations/:orderId", internalGetInventoryReservations)

		// 3. 优惠券/营销服务接口
		internal.POST("/promotion/lock", internalLockCoupon)
		internal.POST("/promotion/release", internalReleaseCoupon)
		internal.POST("/promotion/candidates", internalGetCouponCandidates)

		// 4. 支付与售后服务接口
		internal.GET("/payments/by-order/:orderId", internalGetPaymentByOrder)
		internal.GET("/aftersales/by-order/:orderId", internalGetAfterSalesByOrder)
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

type InternalOrderRefundItem struct {
	OrderItemID     uint `json:"orderItemId"`
	SkuID           uint `json:"skuId"`
	Quantity        int  `json:"quantity"`
	PayableAmount   int  `json:"payableAmount"`
	RefundedAmount  int  `json:"refundedAmount"`
	RefundableQty   int  `json:"refundableQuantity"`
	RefundableValue int  `json:"refundableAmount"`
}

type InternalOrderRefundSource struct {
	OrderID         string                    `json:"orderId"`
	UserID          uint                      `json:"userId"`
	TotalAmount     int                       `json:"totalAmount"`
	Status          int                       `json:"status"`
	PayStatus       int                       `json:"payStatus"`
	AfterSaleStatus int                       `json:"afterSaleStatus"`
	Items           []InternalOrderRefundItem `json:"items"`
}

type InternalAddressSnapshot struct {
	ID            uint   `json:"id"`
	UserID        uint   `json:"userId"`
	ReceiverName  string `json:"receiverName"`
	ReceiverPhone string `json:"receiverPhone"`
	Province      string `json:"province"`
	City          string `json:"city"`
	District      string `json:"district"`
	DetailAddress string `json:"detailAddress"`
	FullAddress   string `json:"fullAddress"`
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

func internalGetOrderRefundSource(c *gin.Context) {
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

	query := core.ReplicaDB.Preload("Items").Where("id = ?", orderID)
	if userIDValue > 0 {
		query = query.Where("user_id = ?", userIDValue)
	}

	var order models.Order
	if err := query.First(&order).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "order not found"})
		return
	}

	items := make([]InternalOrderRefundItem, 0, len(order.Items))
	for _, item := range order.Items {
		refundable := item.PayableAmount - item.RefundedAmount
		if refundable < 0 {
			refundable = 0
		}
		refundableQty := 0
		if item.Quantity > 0 && item.PayableAmount > 0 && refundable > 0 {
			refundableQty = refundable * item.Quantity / item.PayableAmount
			if refundableQty <= 0 {
				refundableQty = 1
			}
		}
		items = append(items, InternalOrderRefundItem{
			OrderItemID:     item.ID,
			SkuID:           item.SkuID,
			Quantity:        item.Quantity,
			PayableAmount:   item.PayableAmount,
			RefundedAmount:  item.RefundedAmount,
			RefundableQty:   refundableQty,
			RefundableValue: refundable,
		})
	}

	c.JSON(http.StatusOK, InternalOrderRefundSource{
		OrderID:         order.ID,
		UserID:          order.UserID,
		TotalAmount:     order.TotalAmount,
		Status:          order.Status,
		PayStatus:       order.PayStatus,
		AfterSaleStatus: order.AfterSaleStatus,
		Items:           items,
	})
}

type InternalCancelPendingOrderReq struct {
	Reason string `json:"reason"`
}

type InternalCancelPendingOrderResp struct {
	OrderID  string `json:"orderId"`
	Status   int    `json:"status"`
	Canceled bool   `json:"canceled"`
}

func internalCancelPendingOrder(c *gin.Context) {
	orderID := c.Param("id")
	var req InternalCancelPendingOrderReq
	_ = c.ShouldBindJSON(&req)
	if req.Reason == "" {
		req.Reason = "支付超时自动取消并释放库存"
	}

	var before models.Order
	if err := core.ReplicaDB.Select("id", "status").First(&before, "id = ?", orderID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "order not found"})
		return
	}

	if err := ordersvc.NewService(core.DB).CancelPendingOrder(orderID, req.Reason); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var after models.Order
	if err := core.ReplicaDB.Select("id", "status").First(&after, "id = ?", orderID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, InternalCancelPendingOrderResp{
		OrderID:  orderID,
		Status:   after.Status,
		Canceled: before.Status == models.OrderStatusPendingPayment && after.Status == models.OrderStatusCanceled,
	})
}

func internalGetExpiredPendingOrders(c *gin.Context) {
	limit := 100
	if rawLimit := c.Query("limit"); rawLimit != "" {
		if parsed, err := strconv.Atoi(rawLimit); err == nil && parsed > 0 && parsed <= 1000 {
			limit = parsed
		}
	}

	var orders []models.Order
	if err := core.ReplicaDB.Select("id").
		Where("status = ? AND pay_expire_at IS NOT NULL AND pay_expire_at < ?", models.OrderStatusPendingPayment, time.Now()).
		Limit(limit).
		Find(&orders).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ids := make([]string, 0, len(orders))
	for _, order := range orders {
		ids = append(ids, order.ID)
	}
	c.JSON(http.StatusOK, ids)
}

type InternalOrderRefundApplyReq struct {
	UserID uint   `json:"userId"`
	Reason string `json:"reason"`
	Proof  string `json:"proof"`
}

func internalApplyOrderRefund(c *gin.Context) {
	orderID := c.Param("id")
	var req InternalOrderRefundApplyReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	err := core.DB.Transaction(func(tx *gorm.DB) error {
		var order models.Order
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&order, "id = ? AND user_id = ?", orderID, req.UserID).Error; err != nil {
			return err
		}
		if order.Status != models.OrderStatusPaid || (order.PayStatus != models.PayStatusPaid && order.PayStatus != models.PayStatusPartialRefunded) {
			return fmt.Errorf("该订单当前状态不支持申请退款")
		}
		fromStatus := order.Status
		order.Status = models.OrderStatusRefundApplying
		order.AfterSaleStatus = models.AfterSaleStatusApplying
		order.RefundReason = req.Reason
		order.RefundProof = req.Proof
		if err := tx.Save(&order).Error; err != nil {
			return err
		}
		return tx.Create(&models.OrderStateLog{
			OrderID:      order.ID,
			FromStatus:   fromStatus,
			ToStatus:     models.OrderStatusRefundApplying,
			OperatorType: 1,
			OperatorID:   req.UserID,
			Event:        "AFTERSALE_APPLIED",
			Remark:       req.Reason,
		}).Error
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success"})
}

type InternalOrderRefundCompleteItem struct {
	OrderItemID uint `json:"orderItemId"`
	Amount      int  `json:"amount"`
}

type InternalOrderRefundCompleteReq struct {
	Items  []InternalOrderRefundCompleteItem `json:"items"`
	Remark string                            `json:"remark"`
}

func internalCompleteOrderRefund(c *gin.Context) {
	orderID := c.Param("id")
	var req InternalOrderRefundCompleteReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	err := core.DB.Transaction(func(tx *gorm.DB) error {
		var order models.Order
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&order, "id = ?", orderID).Error; err != nil {
			return err
		}
		if order.AfterSaleStatus == models.AfterSaleStatusRefunded && (order.PayStatus == models.PayStatusRefunded || order.PayStatus == models.PayStatusPartialRefunded) {
			return nil
		}
		for _, item := range req.Items {
			result := tx.Model(&models.OrderItem{}).
				Where("id = ? AND order_id = ? AND refunded_amount + ? <= payable_amount", item.OrderItemID, orderID, item.Amount).
				Update("refunded_amount", gorm.Expr("refunded_amount + ?", item.Amount))
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return fmt.Errorf("订单行 %d 可退金额不足", item.OrderItemID)
			}
		}

		var totalRefunded int64
		if err := tx.Model(&models.OrderItem{}).
			Where("order_id = ?", orderID).
			Select("COALESCE(SUM(refunded_amount), 0)").
			Scan(&totalRefunded).Error; err != nil {
			return err
		}
		fromStatus := order.Status
		if int(totalRefunded) >= order.TotalAmount {
			order.Status = models.OrderStatusRefunded
			order.PayStatus = models.PayStatusRefunded
		} else {
			order.Status = models.OrderStatusPaid
			order.PayStatus = models.PayStatusPartialRefunded
		}
		order.AfterSaleStatus = models.AfterSaleStatusRefunded
		if err := tx.Save(&order).Error; err != nil {
			return err
		}
		return tx.Create(&models.OrderStateLog{
			OrderID:      order.ID,
			FromStatus:   fromStatus,
			ToStatus:     order.Status,
			OperatorType: 1,
			OperatorID:   0,
			Event:        "AFTERSALE_APPROVED",
			Remark:       req.Remark,
		}).Error
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success"})
}

func internalRejectOrderRefund(c *gin.Context) {
	orderID := c.Param("id")
	err := core.DB.Transaction(func(tx *gorm.DB) error {
		var order models.Order
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&order, "id = ?", orderID).Error; err != nil {
			return err
		}
		if order.AfterSaleStatus == models.AfterSaleStatusRejected {
			return nil
		}
		fromStatus := order.Status
		order.Status = models.OrderStatusRefundRejected
		order.AfterSaleStatus = models.AfterSaleStatusRejected
		if err := tx.Save(&order).Error; err != nil {
			return err
		}
		return tx.Create(&models.OrderStateLog{
			OrderID:      order.ID,
			FromStatus:   fromStatus,
			ToStatus:     models.OrderStatusRefundRejected,
			OperatorType: 1,
			OperatorID:   0,
			Event:        "AFTERSALE_REJECTED",
			Remark:       "商家拒绝退款申请",
		}).Error
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success"})
}

func internalGetAddressSnapshot(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid address id"})
		return
	}
	userIDValue := uint(0)
	if userIDRaw := c.Query("userId"); userIDRaw != "" {
		userID, err := strconv.Atoi(userIDRaw)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
			return
		}
		userIDValue = uint(userID)
	}

	query := core.ReplicaDB.Where("id = ?", id)
	if userIDValue > 0 {
		query = query.Where("user_id = ?", userIDValue)
	}

	var address models.Address
	if err := query.First(&address).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "address not found"})
		return
	}

	fullAddress := fmt.Sprintf("%s%s%s%s", address.Province, address.City, address.District, address.DetailAddress)
	c.JSON(http.StatusOK, InternalAddressSnapshot{
		ID:            address.ID,
		UserID:        address.UserID,
		ReceiverName:  string(address.ReceiverName),
		ReceiverPhone: string(address.ReceiverPhone),
		Province:      address.Province,
		City:          address.City,
		District:      address.District,
		DetailAddress: string(address.DetailAddress),
		FullAddress:   fullAddress,
	})
}

type InternalClearCartItemsReq struct {
	UserID uint   `json:"userId"`
	SkuIDs []uint `json:"skuIds"`
}

func internalClearCartItems(c *gin.Context) {
	var req InternalClearCartItemsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.UserID == 0 || len(req.SkuIDs) == 0 {
		c.JSON(http.StatusOK, gin.H{"status": "success"})
		return
	}

	if err := core.DB.Where("user_id = ? AND sku_id IN ?", req.UserID, req.SkuIDs).Delete(&models.CartItem{}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success"})
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

type InternalRestockItem struct {
	SkuID    uint `json:"skuId"`
	Quantity int  `json:"quantity"`
}

type InternalRestockStockReq struct {
	OrderID string                `json:"orderId"`
	Items   []InternalRestockItem `json:"items"`
}

func internalRestockStock(c *gin.Context) {
	var req InternalRestockStockReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.OrderID == "" || len(req.Items) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing orderId or items"})
		return
	}

	items := make([]inventory.RestockItem, 0, len(req.Items))
	for _, item := range req.Items {
		items = append(items, inventory.RestockItem{SkuID: item.SkuID, Quantity: item.Quantity})
	}

	err := core.DB.Transaction(func(tx *gorm.DB) error {
		svc := inventory.NewService(tx)
		return svc.RestockItemsForOrder(tx, req.OrderID, items)
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success"})
}

func internalGetInventoryReservations(c *gin.Context) {
	orderID := c.Param("orderId")
	var reservations []models.InventoryReservation
	if err := core.ReplicaDB.Where("order_id = ?", orderID).Order("created_at asc").Find(&reservations).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, reservations)
}

func internalGetPaymentByOrder(c *gin.Context) {
	orderID := c.Param("orderId")
	var payment models.PaymentOrder
	if err := core.ReplicaDB.Where("order_id = ?", orderID).Order("created_at desc").First(&payment).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "payment not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, payment)
}

type InternalAfterSalesByOrder struct {
	AfterSales   []models.AfterSaleOrder `json:"afterSales"`
	RefundOrders []models.RefundOrder    `json:"refundOrders"`
}

func internalGetAfterSalesByOrder(c *gin.Context) {
	orderID := c.Param("orderId")
	var result InternalAfterSalesByOrder
	if err := core.ReplicaDB.Preload("Items").Where("order_id = ?", orderID).Order("created_at desc").Find(&result.AfterSales).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := core.ReplicaDB.Where("order_id = ?", orderID).Order("created_at desc").Find(&result.RefundOrders).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
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
