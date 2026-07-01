package payment

import (
	"testing"
	"time"

	"GoShop/internal/inventory"
	"GoShop/internal/testutil"
	"GoShop/models"

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
}
