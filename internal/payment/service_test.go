package payment

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"GoShop/internal/inventory"
	"GoShop/internal/testutil"
	"GoShop/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestPaymentService(t *testing.T) {
	db := testutil.SetupTestDB(t)
	svc := NewService(db)

	userID := uint(1)
	skuID := uint(1) // haiku SKU, price=39900
	quantity := 2
	totalAmount := 39900 * quantity // 79800 分，免运费

	// 辅助方法：在 DB 中生成一个待支付订单和相应的库存预占
	setupPendingOrder := func(orderID string) models.Order {
		// 1. 确保 SKU 的库存可用且为 100
		var inv models.SkuInventory
		if err := db.Where("sku_id = ?", skuID).First(&inv).Error; err != nil {
			inv = models.SkuInventory{
				SkuID:     skuID,
				Available: 100,
				Reserved:  0,
				Sold:      0,
			}
			if err := db.Create(&inv).Error; err != nil {
				t.Fatalf("failed to create sku inventory: %v", err)
			}
		} else {
			inv.Available = 100
			inv.Reserved = 0
			inv.Sold = 0
			if err := db.Save(&inv).Error; err != nil {
				t.Fatalf("failed to save sku inventory: %v", err)
			}
		}

		// 2. 创建 Order
		order := models.Order{
			ID:                orderID,
			UserID:            userID,
			TotalAmount:       totalAmount,
			PayableAmount:     totalAmount,
			GoodsOriginAmount: totalAmount,
			Status:            models.OrderStatusPendingPayment,
			PayStatus:         models.PayStatusUnpaid,
			ReceiverName:      "Test User",
			ReceiverPhone:     "13800138000",
			ReceiverAddr:      "Test Address",
			Items: []models.OrderItem{
				{
					SkuID:        skuID,
					Price:        39900,
					Quantity:     quantity,
					OriginAmount: totalAmount,
				},
			},
		}
		if err := db.Create(&order).Error; err != nil {
			t.Fatalf("failed to create test order: %v", err)
		}

		// 3. 预占库存
		invSvc := inventory.NewService(db)
		err := db.Transaction(func(tx *gorm.DB) error {
			return invSvc.ReserveStock(tx, orderID, userID, []inventory.ReserveItem{
				{SkuID: skuID, Quantity: quantity},
			}, time.Now().Add(10*time.Minute))
		})
		if err != nil {
			t.Fatalf("failed to reserve stock: %v", err)
		}

		return order
	}

	t.Run("PayMockOrder_Idempotent", func(t *testing.T) {
		orderID := "ORDER-PAY-IDEM-01"
		order := setupPendingOrder(orderID)

		// 第一次调用 PayMockOrder，应该正常支付成功
		res1, err := svc.PayMockOrder(userID, order.ID)
		if err != nil {
			t.Fatalf("first PayMockOrder failed: %v", err)
		}
		if res1.AlreadyPaid {
			t.Errorf("expected res1.AlreadyPaid to be false, got true")
		}

		// 验证订单状态已更新为 Paid
		var updatedOrder models.Order
		if err := db.Preload("Items").First(&updatedOrder, "id = ?", order.ID).Error; err != nil {
			t.Fatalf("failed to query updated order: %v", err)
		}
		if updatedOrder.Status != models.OrderStatusPaid || updatedOrder.PayStatus != models.PayStatusPaid {
			t.Errorf("expected order status and pay status to be Paid, got status=%d, payStatus=%d", updatedOrder.Status, updatedOrder.PayStatus)
		}

		// 验证库存确认扣减：Reserved 变回 0，Sold 变为 2，Available 为 98
		var inv models.SkuInventory
		if err := db.First(&inv, "sku_id = ?", skuID).Error; err != nil {
			t.Fatalf("failed to query inventory: %v", err)
		}
		if inv.Reserved != 0 || inv.Sold != 2 || inv.Available != 98 {
			t.Errorf("expected inv reserved=0, sold=2, available=98, got reserved=%d, sold=%d, available=%d", inv.Reserved, inv.Sold, inv.Available)
		}

		// 第二次调用 PayMockOrder，应该幂等，返回 AlreadyPaid == true
		res2, err := svc.PayMockOrder(userID, order.ID)
		if err != nil {
			t.Fatalf("second PayMockOrder failed: %v", err)
		}
		if !res2.AlreadyPaid {
			t.Errorf("expected res2.AlreadyPaid to be true, got false")
		}
	})

	t.Run("HandleMockCallback_Idempotent", func(t *testing.T) {
		orderID := "ORDER-CB-IDEM-02"
		order := setupPendingOrder(orderID)

		// 手动生成 PaymentOrder (HandleMockCallback 需要找到对应的 PaymentOrder)
		payment, err := CreateMockPaymentOrder(db, order)
		if err != nil {
			t.Fatalf("failed to create payment order: %v", err)
		}

		eventID := "event-uniq-callback-1002"
		req := MockCallbackRequest{
			PaymentOrderID: payment.ID,
			OrderID:        order.ID,
			Amount:         payment.Amount,
			EventID:        eventID,
			Status:         "paid",
		}

		// 第一次 Callback 成功
		retID1, err := svc.HandleMockCallback(req)
		if err != nil {
			t.Fatalf("first HandleMockCallback failed: %v", err)
		}
		if retID1 != order.ID {
			t.Errorf("expected returned orderID %s, got %s", order.ID, retID1)
		}

		// 验证订单状态
		var updatedOrder models.Order
		if err := db.First(&updatedOrder, "id = ?", order.ID).Error; err != nil {
			t.Fatalf("failed to query updated order: %v", err)
		}
		if updatedOrder.Status != models.OrderStatusPaid {
			t.Errorf("expected order status to be Paid, got %d", updatedOrder.Status)
		}

		// 再次使用相同的 eventID 调用 HandleMockCallback
		retID2, err := svc.HandleMockCallback(req)
		if err != nil {
			t.Fatalf("second HandleMockCallback failed: %v", err)
		}
		if retID2 != order.ID {
			t.Errorf("expected returned orderID %s, got %s", order.ID, retID2)
		}

		// 验证没有生成重复的 Transaction
		var txCount int64
		db.Model(&models.PaymentTransaction{}).Where("channel_event_id = ?", eventID).Count(&txCount)
		if txCount != 1 {
			t.Errorf("expected exactly 1 transaction for event, got %d", txCount)
		}
	})

	t.Run("HandleMockCallback_AmountMismatchDoesNotPay", func(t *testing.T) {
		orderID := "ORDER-CB-MISMATCH-03"
		order := setupPendingOrder(orderID)
		payment, err := CreateMockPaymentOrder(db, order)
		if err != nil {
			t.Fatalf("failed to create payment order: %v", err)
		}

		_, err = svc.HandleMockCallback(MockCallbackRequest{
			PaymentOrderID: payment.ID,
			OrderID:        order.ID,
			Amount:         payment.Amount - 1,
			EventID:        "event-amount-mismatch-1003",
			Status:         "paid",
		})
		if err == nil {
			t.Fatalf("expected amount mismatch error")
		}

		var updatedOrder models.Order
		if err := db.First(&updatedOrder, "id = ?", order.ID).Error; err != nil {
			t.Fatalf("failed to query order: %v", err)
		}
		if updatedOrder.Status != models.OrderStatusPendingPayment || updatedOrder.PayStatus != models.PayStatusUnpaid {
			t.Fatalf("expected order to remain pending, got status=%d pay=%d", updatedOrder.Status, updatedOrder.PayStatus)
		}
		var transaction models.PaymentTransaction
		if err := db.First(&transaction, "channel_event_id = ?", "event-amount-mismatch-1003").Error; err != nil {
			t.Fatalf("failed to query failed transaction: %v", err)
		}
		if transaction.ProcessStatus != models.TransactionStatusFailed {
			t.Fatalf("expected failed transaction status, got %d", transaction.ProcessStatus)
		}
	})

	t.Run("PayMockOrder_ConfirmsLockedCoupon", func(t *testing.T) {
		orderID := "ORDER-PAY-COUPON-04"
		order := setupPendingOrder(orderID)
		now := time.Now()
		coupon := models.Coupon{
			ID:        30,
			Name:      "Payment Coupon",
			Type:      3,
			Value:     1000,
			StartTime: now.Add(-time.Hour),
			EndTime:   now.Add(time.Hour),
		}
		if err := db.Create(&coupon).Error; err != nil {
			t.Fatalf("failed to create coupon: %v", err)
		}
		userCoupon := models.UserCoupon{
			ID:            30,
			UserID:        userID,
			CouponID:      coupon.ID,
			Status:        models.UserCouponStatusLocked,
			LockedOrderID: order.ID,
			LockedAt:      &now,
		}
		if err := db.Create(&userCoupon).Error; err != nil {
			t.Fatalf("failed to create user coupon: %v", err)
		}
		if err := db.Model(&models.Order{}).Where("id = ?", order.ID).Update("user_coupon_id", userCoupon.ID).Error; err != nil {
			t.Fatalf("failed to attach coupon to order: %v", err)
		}

		if _, err := svc.PayMockOrder(userID, order.ID); err != nil {
			t.Fatalf("PayMockOrder failed: %v", err)
		}

		var updatedCoupon models.UserCoupon
		if err := db.First(&updatedCoupon, "id = ?", userCoupon.ID).Error; err != nil {
			t.Fatalf("failed to query coupon: %v", err)
		}
		if updatedCoupon.Status != models.UserCouponStatusUsed || updatedCoupon.UsedAt == nil || updatedCoupon.LockedOrderID != "" {
			t.Fatalf("expected coupon confirmed used, got status=%d usedAt=%v lockedOrder=%q", updatedCoupon.Status, updatedCoupon.UsedAt, updatedCoupon.LockedOrderID)
		}
		var event models.OutboxEvent
		if err := db.First(&event, "aggregate_id = ? AND event_type = ?", PaymentOrderID(order.ID), "PaymentSucceeded").Error; err != nil {
			t.Fatalf("expected PaymentSucceeded outbox event: %v", err)
		}
	})
}

func TestPaymentService_IsolatedDB(t *testing.T) {
	// 1. 初始化独立数据库，只做支付相关的 Table Migrate
	dbIsolated, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open sqlite: %v", err)
	}

	err = dbIsolated.AutoMigrate(
		&models.PaymentOrder{},
		&models.PaymentTransaction{},
		&models.OutboxEvent{},
	)
	if err != nil {
		t.Fatalf("AutoMigrate failed: %v", err)
	}

	// 确认 orders 表不存在
	if dbIsolated.Migrator().HasTable("orders") {
		t.Fatal("orders table should not exist in payment isolated database")
	}

	// 2. 启动模拟的订单微服务 HTTP 服务监听 8105 端口，响应 payment-source RPC 请求
	listener, err := net.Listen("tcp", "127.0.0.1:8105")
	if err != nil {
		t.Fatalf("Failed to listen on 127.0.0.1:8105: %v", err)
	}
	defer listener.Close()

	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == "GET" && strings.Contains(r.URL.Path, "/payment-source") {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"orderId":      "ORDER-ISO-1234",
					"userId":       1,
					"totalAmount":  99900,
					"status":       10, // Pending Payment
					"payStatus":    0,  // Unpaid
					"userCouponId": 0,
				})
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}),
	}
	go srv.Serve(listener)
	defer srv.Shutdown(context.Background())

	// 3. 执行支付逻辑
	svc := NewService(dbIsolated)

	// 创建支付单
	res, err := svc.CreateOrGetPaymentOrder(1, "ORDER-ISO-1234")
	if err != nil {
		t.Fatalf("CreateOrGetPaymentOrder failed: %v", err)
	}
	if res.Amount != 99900 || res.OrderID != "ORDER-ISO-1234" {
		t.Errorf("unexpected payment order result: %+v", res)
	}

	// 验证支付单已落库
	var payment models.PaymentOrder
	if err := dbIsolated.First(&payment, "id = ?", res.PaymentOrderID).Error; err != nil {
		t.Fatalf("failed to query payment order: %v", err)
	}
	if payment.Amount != 99900 || payment.Status != models.PaymentStatusCreated {
		t.Errorf("unexpected payment status: %+v", payment)
	}

	// 执行支付
	payRes, err := svc.PayMockOrder(1, "ORDER-ISO-1234")
	if err != nil {
		t.Fatalf("PayMockOrder failed: %v", err)
	}
	if payRes.PaymentOrderID != res.PaymentOrderID {
		t.Errorf("unexpected pay result: %+v", payRes)
	}

	// 验证支付单状态更新为已支付
	if err := dbIsolated.First(&payment, "id = ?", res.PaymentOrderID).Error; err != nil {
		t.Fatalf("failed to query payment order: %v", err)
	}
	if payment.Status != models.PaymentStatusPaid {
		t.Errorf("expected status Paid, got %d", payment.Status)
	}

	// 验证支付流水已生成
	var count int64
	dbIsolated.Model(&models.PaymentTransaction{}).Where("payment_order_id = ?", res.PaymentOrderID).Count(&count)
	if count != 1 {
		t.Errorf("expected 1 transaction, got %d", count)
	}
}
