package core_test

import (
	"testing"
	"time"

	"GoShop/core"
	"GoShop/internal/testutil"
	"GoShop/models"
)

func TestCallInternalService_Fallback(t *testing.T) {
	db := testutil.SetupTestDB(t)

	// 1. 商品服务降级验证 (GET /api/internal/products/:id)
	t.Run("Product Get Fallback", func(t *testing.T) {
		var sku models.Sku
		// 目标端口 8102 (product)，且网络未开，应该 fallback 到本地 DB 成功拉取 SKU 1
		err := core.CallInternalService(db, 8102, "GET", "/api/internal/products/1", nil, &sku)
		if err != nil {
			t.Fatalf("product fallback failed: %v", err)
		}
		if sku.ID != 1 {
			t.Errorf("expected sku ID 1, got %d", sku.ID)
		}
	})

	t.Run("Product Cart Summary Fallback", func(t *testing.T) {
		var summary struct {
			SkuID   uint   `json:"skuId"`
			SpuID   uint   `json:"spuId"`
			SpuName string `json:"spuName"`
			SkuName string `json:"skuName"`
			Price   int    `json:"price"`
			Image   string `json:"image"`
		}

		err := core.CallInternalService(db, 8102, "GET", "/api/internal/products/1/cart-summary", nil, &summary)
		if err != nil {
			t.Fatalf("product cart summary fallback failed: %v", err)
		}
		if summary.SkuID != 1 || summary.SpuID != 1 || summary.SpuName == "" || summary.SkuName == "" || summary.Price == 0 {
			t.Fatalf("unexpected product summary: %+v", summary)
		}
	})

	t.Run("Order Payment Source Fallback", func(t *testing.T) {
		order := models.Order{
			ID:          "ORDER-INTERNAL-SOURCE-01",
			UserID:      1,
			TotalAmount: 12345,
			Status:      models.OrderStatusPendingPayment,
			PayStatus:   models.PayStatusUnpaid,
		}
		if err := db.Create(&order).Error; err != nil {
			t.Fatalf("create order: %v", err)
		}

		var source struct {
			OrderID     string `json:"orderId"`
			UserID      uint   `json:"userId"`
			TotalAmount int    `json:"totalAmount"`
			Status      int    `json:"status"`
			PayStatus   int    `json:"payStatus"`
		}
		err := core.CallInternalService(db, 8105, "GET", "/api/internal/orders/ORDER-INTERNAL-SOURCE-01/payment-source?userId=1", nil, &source)
		if err != nil {
			t.Fatalf("order payment source fallback failed: %v", err)
		}
		if source.OrderID != order.ID || source.UserID != order.UserID || source.TotalAmount != order.TotalAmount {
			t.Fatalf("unexpected order payment source: %+v", source)
		}
	})

	// 2. 优惠券 Candidates 降级验证 (POST /api/internal/promotion/candidates)
	t.Run("Promotion Candidates Fallback", func(t *testing.T) {
		// 往测试库插入一张可用优惠券关联
		userCoupon := models.UserCoupon{
			ID:       100,
			UserID:   1,
			CouponID: 1, // Seed 里的卡券 ID 1 (10元无门槛券，Value = 1000)
			Status:   models.UserCouponStatusAvailable,
		}
		db.Create(&userCoupon)

		type CouponCandidate struct {
			UserCouponID   uint   `json:"userCouponId"`
			Available      bool   `json:"available"`
			Reason         string `json:"reason"`
			DiscountAmount int    `json:"discountAmount"`
		}
		var candidates []CouponCandidate

		err := core.CallInternalService(db, 8104, "POST", "/api/internal/promotion/candidates", map[string]interface{}{
			"userId":               1,
			"selectedUserCouponId": 0,
			"subtotal":             40000,
		}, &candidates)
		if err != nil {
			t.Fatalf("promotion candidates fallback failed: %v", err)
		}

		if len(candidates) == 0 {
			t.Fatal("expected candidates, got empty list")
		}
		found := false
		for _, c := range candidates {
			if c.UserCouponID == 100 {
				found = true
				if !c.Available {
					t.Error("expected coupon 100 to be available")
				}
				if c.DiscountAmount != 1000 {
					t.Errorf("expected discount 1000, got %d", c.DiscountAmount)
				}
			}
		}
		if !found {
			t.Error("user coupon 100 not found in candidates")
		}
	})

	// 3. 库存预占锁定与释放降级验证
	t.Run("Inventory Reserve and Release Fallback", func(t *testing.T) {
		// 锁定库存
		type ReserveItem struct {
			SkuID int `json:"skuId"`
			Qty   int `json:"qty"`
		}
		items := []ReserveItem{{SkuID: 1, Qty: 2}}

		err := core.CallInternalService(db, 8103, "POST", "/api/internal/inventory/reserve", map[string]interface{}{
			"orderId": "ORD-TEST-FALLBACK",
			"userId":  1,
			"items":   items,
		}, nil)
		if err != nil {
			t.Fatalf("inventory reserve fallback failed: %v", err)
		}

		// 验证预占记录已生成
		var reservation models.InventoryReservation
		err = db.Where("order_id = ?", "ORD-TEST-FALLBACK").First(&reservation).Error
		if err != nil {
			t.Fatalf("failed to find reservation record: %v", err)
		}
		if reservation.Quantity != 2 || reservation.Status != models.ReservationStatusReserved {
			t.Errorf("unexpected reservation quantity or status: %+v", reservation)
		}

		// 释放库存
		err = core.CallInternalService(db, 8103, "POST", "/api/internal/inventory/release", map[string]interface{}{
			"orderId": "ORD-TEST-FALLBACK",
		}, nil)
		if err != nil {
			t.Fatalf("inventory release fallback failed: %v", err)
		}

		// 验证预占记录状态已改为 Released (30)
		db.Where("order_id = ?", "ORD-TEST-FALLBACK").First(&reservation)
		if reservation.Status != models.ReservationStatusReleased {
			t.Errorf("expected reservation status to be Released, got %d", reservation.Status)
		}
	})

	t.Run("User Address Snapshot and Missing Address Fallback", func(t *testing.T) {
		addr := models.Address{
			ID:            101,
			UserID:        1,
			ReceiverName:  "Test Address Receiver",
			ReceiverPhone: "13800000000",
			Province:      "Test Province",
			City:          "Test City",
			District:      "Test District",
			DetailAddress: "Test Detail Address",
		}
		if err := db.Create(&addr).Error; err != nil {
			t.Fatalf("create test address: %v", err)
		}

		var snapshot struct {
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

		err := core.CallInternalService(db, 8101, "GET", "/api/internal/addresses/101/snapshot?userId=1", nil, &snapshot)
		if err != nil {
			t.Fatalf("address snapshot fallback failed: %v", err)
		}
		if snapshot.ID != 101 || snapshot.ReceiverName != "Test Address Receiver" {
			t.Errorf("unexpected snapshot content: %+v", snapshot)
		}

		err = core.CallInternalService(db, 8101, "GET", "/api/internal/addresses/9999/snapshot?userId=1", nil, &snapshot)
		if err == nil {
			t.Fatal("expected error for missing address, got nil")
		}
	})

	t.Run("Cart Clear Items Idempotency Fallback", func(t *testing.T) {
		item := models.CartItem{
			UserID:   1,
			SkuID:    1,
			Quantity: 2,
		}
		db.Create(&item)

		reqBody := map[string]interface{}{
			"userId": 1,
			"skuIds": []uint{1},
		}

		err := core.CallInternalService(db, 8108, "POST", "/api/internal/cart/clear-items", reqBody, nil)
		if err != nil {
			t.Fatalf("first clear-items failed: %v", err)
		}

		var count int64
		db.Model(&models.CartItem{}).Where("user_id = ? AND sku_id = ?", 1, 1).Count(&count)
		if count != 0 {
			t.Errorf("expected 0 cart items, got %d", count)
		}

		err = core.CallInternalService(db, 8108, "POST", "/api/internal/cart/clear-items", reqBody, nil)
		if err != nil {
			t.Fatalf("second clear-items should be idempotent, got: %v", err)
		}
	})

	t.Run("Order Refund Source Fallback", func(t *testing.T) {
		orderID := "ORDER-REFUND-SRC-01"
		order := models.Order{
			ID:          orderID,
			UserID:      1,
			TotalAmount: 10000,
			Status:      models.OrderStatusPaid,
			PayStatus:   models.PayStatusPaid,
			Items: []models.OrderItem{
				{
					SkuID:         1,
					Price:         10000,
					Quantity:      1,
					OriginAmount:  10000,
					PayableAmount: 10000,
				},
			},
		}
		db.Create(&order)

		var refundSrc struct {
			OrderID         string `json:"orderId"`
			UserID          uint   `json:"userId"`
			TotalAmount     int    `json:"totalAmount"`
			Status          int    `json:"status"`
			PayStatus       int    `json:"payStatus"`
			AfterSaleStatus int    `json:"afterSaleStatus"`
		}

		err := core.CallInternalService(db, 8105, "GET", "/api/internal/orders/"+orderID+"/refund-source?userId=1", nil, &refundSrc)
		if err != nil {
			t.Fatalf("order refund-source fallback failed: %v", err)
		}
		if refundSrc.OrderID != orderID || refundSrc.TotalAmount != 10000 {
			t.Errorf("unexpected refund source: %+v", refundSrc)
		}
	})

	t.Run("Order Seckill Create Fallback", func(t *testing.T) {
		orderID := "SK-TEST-0001"
		reqBody := map[string]interface{}{
			"orderId":     orderID,
			"userId":      1,
			"skuId":       1,
			"price":       39900,
			"quantity":    1,
			"payExpireAt": time.Now().Add(15 * time.Minute),
		}

		err := core.CallInternalService(db, 8105, "POST", "/api/internal/orders/seckill-create", reqBody, nil)
		if err != nil {
			t.Fatalf("seckill-create fallback failed: %v", err)
		}

		var o models.Order
		err = db.Preload("Items").First(&o, "id = ?", orderID).Error
		if err != nil {
			t.Fatalf("failed to query seckill order: %v", err)
		}
		if o.TotalAmount != 39900 || len(o.Items) != 1 || o.Items[0].SkuID != 1 {
			t.Errorf("unexpected seckill order: %+v", o)
		}
	})

	t.Run("Order Cancel Pending Fallback", func(t *testing.T) {
		orderID := "ORD-CANCEL-PENDING-01"
		order := models.Order{
			ID:            orderID,
			UserID:        1,
			TotalAmount:   10000,
			PayableAmount: 10000,
			Status:        models.OrderStatusPendingPayment,
			PayStatus:     models.PayStatusUnpaid,
			UserCouponID:  201,
		}
		db.Create(&order)

		userCoupon := models.UserCoupon{
			ID:            201,
			UserID:        1,
			CouponID:      1,
			Status:        models.UserCouponStatusLocked,
			LockedOrderID: orderID,
		}
		db.Save(&userCoupon)

		reqBody := map[string]interface{}{
			"reason": "支付超时自动取消",
		}

		var resp struct {
			OrderID  string `json:"orderId"`
			Status   int    `json:"status"`
			Canceled bool   `json:"canceled"`
		}
		err := core.CallInternalService(db, 8105, "POST", "/api/internal/orders/"+orderID+"/cancel-pending", reqBody, &resp)
		if err != nil {
			t.Fatalf("cancel-pending fallback failed: %v", err)
		}

		var o models.Order
		db.First(&o, "id = ?", orderID)
		if o.Status != models.OrderStatusCanceled {
			t.Errorf("expected status Canceled (60), got %d", o.Status)
		}

		var uc models.UserCoupon
		db.First(&uc, 201)
		if uc.Status != models.UserCouponStatusAvailable {
			t.Errorf("expected coupon status Available (0), got %d", uc.Status)
		}
	})

	t.Run("Order Expired Pending Fallback", func(t *testing.T) {
		orderID := "ORD-EXPIRED-PENDING-01"
		pastTime := time.Now().Add(-10 * time.Minute)
		order := models.Order{
			ID:          orderID,
			UserID:      1,
			TotalAmount: 10000,
			Status:      models.OrderStatusPendingPayment,
			PayStatus:   models.PayStatusUnpaid,
			PayExpireAt: &pastTime,
		}
		db.Create(&order)

		var expiredIDs []string
		err := core.CallInternalService(db, 8105, "GET", "/api/internal/orders/expired-pending?limit=10", nil, &expiredIDs)
		if err != nil {
			t.Fatalf("expired-pending fallback failed: %v", err)
		}

		found := false
		for _, id := range expiredIDs {
			if id == orderID {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected expired order ID %s to be in list: %v", orderID, expiredIDs)
		}
	})
}
