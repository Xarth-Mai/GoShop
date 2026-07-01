package order

import (
	"testing"
	"time"

	"GoShop/internal/checkout"
	"GoShop/internal/testutil"
	"GoShop/models"
)

func TestCreateOrder_CouponValidation(t *testing.T) {
	db := testutil.SetupTestDB(t)
	svc := NewService(db)

	userID := uint(1)
	addressID := uint(1)

	// 插入一个便宜的 SKU 用于测试未达到优惠券满减门槛的场景
	cheapSku := models.Sku{
		ID:    20,
		SpuID: 1,
		Title: "Cheap Sku for Order Test",
		Price: 5000, // 50元
		Stock: 10,
	}
	if err := db.Create(&cheapSku).Error; err != nil {
		t.Fatalf("failed to create cheap sku: %v", err)
	}

	t.Run("Used Coupon - Reject Order", func(t *testing.T) {
		// 将 UserCoupon ID=1 的状态改为已使用 (status=1)
		err := db.Model(&models.UserCoupon{}).Where("id = ?", 1).Update("status", 1).Error
		if err != nil {
			t.Fatalf("failed to update user coupon to used: %v", err)
		}

		req := CreateRequest{
			Items: []checkout.ItemReq{
				{SkuID: 1, Quantity: 1}, // Sku 1 售价 39900 (399元)
			},
			AddressID:    addressID,
			UserCouponID: 1, // 已使用的优惠券
		}

		_, err = svc.CreateOrder(userID, req)
		if err == nil {
			t.Fatalf("expected error when ordering with an already used coupon")
		}
		expectedErr := "优惠券不可用或已失效"
		if err.Error() != expectedErr {
			t.Errorf("expected error %q, got %q", expectedErr, err.Error())
		}
	})

	t.Run("MinAmount Not Met - Reject Order", func(t *testing.T) {
		// UserCoupon ID=2 为“满500减50”，门槛 50000 (500元)。
		// 购买 cheapSku (5000 = 50元)，数量 1。不满足门槛。
		req := CreateRequest{
			Items: []checkout.ItemReq{
				{SkuID: cheapSku.ID, Quantity: 1},
			},
			AddressID:    addressID,
			UserCouponID: 2, // 门槛未达到
		}

		_, err := svc.CreateOrder(userID, req)
		if err == nil {
			t.Fatalf("expected error when order amount does not meet coupon threshold")
		}
		expectedErr := "优惠券不可用或已失效"
		if err.Error() != expectedErr {
			t.Errorf("expected error %q, got %q", expectedErr, err.Error())
		}
	})

	t.Run("Expired Coupon - Reject Order", func(t *testing.T) {
		// 创建一个已过期的优惠券
		now := time.Now()
		expiredCoupon := models.Coupon{
			ID:        10,
			Name:      "Expired Coupon",
			Type:      3,
			Value:     1000,
			StartTime: now.Add(-2 * time.Hour),
			EndTime:   now.Add(-1 * time.Hour),
		}
		if err := db.Create(&expiredCoupon).Error; err != nil {
			t.Fatalf("failed to create expired coupon: %v", err)
		}

		userCoupon := models.UserCoupon{
			ID:       10,
			UserID:   userID,
			CouponID: expiredCoupon.ID,
			Status:   0, // 未使用
		}
		if err := db.Create(&userCoupon).Error; err != nil {
			t.Fatalf("failed to create user coupon: %v", err)
		}

		req := CreateRequest{
			Items: []checkout.ItemReq{
				{SkuID: 1, Quantity: 1},
			},
			AddressID:    addressID,
			UserCouponID: 10, // 已过期的优惠券
		}

		_, err := svc.CreateOrder(userID, req)
		if err == nil {
			t.Fatalf("expected error when ordering with an expired coupon")
		}
		expectedErr := "优惠券不可用或已失效"
		if err.Error() != expectedErr {
			t.Errorf("expected error %q, got %q", expectedErr, err.Error())
		}
	})

	t.Run("Valid Coupon - Success and Lock Coupon", func(t *testing.T) {
		// 创建一张新的有效无门槛券
		now := time.Now()
		validCoupon := models.Coupon{
			ID:        11,
			Name:      "Valid Coupon",
			Type:      3,
			Value:     1000,
			StartTime: now.Add(-1 * time.Hour),
			EndTime:   now.Add(1 * time.Hour),
		}
		if err := db.Create(&validCoupon).Error; err != nil {
			t.Fatalf("failed to create valid coupon: %v", err)
		}

		userCoupon := models.UserCoupon{
			ID:       11,
			UserID:   userID,
			CouponID: validCoupon.ID,
			Status:   0, // 未使用
		}
		if err := db.Create(&userCoupon).Error; err != nil {
			t.Fatalf("failed to create user coupon: %v", err)
		}

		req := CreateRequest{
			Items: []checkout.ItemReq{
				{SkuID: 1, Quantity: 1}, // 39900
			},
			AddressID:    addressID,
			UserCouponID: 11,
		}

		res, err := svc.CreateOrder(userID, req)
		if err != nil {
			t.Fatalf("CreateOrder failed with valid coupon: %v", err)
		}
		if res.OrderID == "" {
			t.Errorf("expected order ID to be returned, got empty")
		}

		// 创建订单只锁定优惠券，支付成功后才确认使用。
		var uc models.UserCoupon
		if err := db.First(&uc, "id = ?", 11).Error; err != nil {
			t.Fatalf("failed to check user coupon state: %v", err)
		}
		if uc.Status != models.UserCouponStatusLocked {
			t.Errorf("expected user coupon status to be locked, got %d", uc.Status)
		}
		if uc.LockedOrderID != res.OrderID {
			t.Errorf("expected locked order %s, got %s", res.OrderID, uc.LockedOrderID)
		}
		if uc.UsedAt != nil {
			t.Errorf("expected user coupon UsedAt to stay nil before payment")
		}
	})

	t.Run("Cancel Pending Order Releases Coupon Lock", func(t *testing.T) {
		now := time.Now()
		coupon := models.Coupon{
			ID:        12,
			Name:      "Cancel Release Coupon",
			Type:      3,
			Value:     1000,
			StartTime: now.Add(-1 * time.Hour),
			EndTime:   now.Add(1 * time.Hour),
		}
		if err := db.Create(&coupon).Error; err != nil {
			t.Fatalf("failed to create coupon: %v", err)
		}
		userCoupon := models.UserCoupon{ID: 12, UserID: userID, CouponID: coupon.ID, Status: models.UserCouponStatusAvailable}
		if err := db.Create(&userCoupon).Error; err != nil {
			t.Fatalf("failed to create user coupon: %v", err)
		}

		res, err := svc.CreateOrder(userID, CreateRequest{
			Items:        []checkout.ItemReq{{SkuID: 1, Quantity: 1}},
			AddressID:    addressID,
			UserCouponID: 12,
		})
		if err != nil {
			t.Fatalf("CreateOrder failed: %v", err)
		}
		if err := svc.CancelPendingOrder(res.OrderID, "test cancel"); err != nil {
			t.Fatalf("CancelPendingOrder failed: %v", err)
		}

		var uc models.UserCoupon
		if err := db.First(&uc, "id = ?", 12).Error; err != nil {
			t.Fatalf("failed to check user coupon: %v", err)
		}
		if uc.Status != models.UserCouponStatusAvailable || uc.LockedOrderID != "" || uc.LockedAt != nil {
			t.Fatalf("expected coupon lock released, got status=%d lockedOrder=%q lockedAt=%v", uc.Status, uc.LockedOrderID, uc.LockedAt)
		}
	})

	t.Run("Paid Order Is Not Canceled", func(t *testing.T) {
		order := models.Order{
			ID:            "ORDER-PAID-NOT-CANCEL",
			UserID:        userID,
			TotalAmount:   1000,
			PayableAmount: 1000,
			Status:        models.OrderStatusPaid,
			PayStatus:     models.PayStatusPaid,
		}
		if err := db.Create(&order).Error; err != nil {
			t.Fatalf("failed to create paid order: %v", err)
		}
		if err := svc.CancelPendingOrder(order.ID, "should be ignored"); err != nil {
			t.Fatalf("CancelPendingOrder failed: %v", err)
		}
		var got models.Order
		if err := db.First(&got, "id = ?", order.ID).Error; err != nil {
			t.Fatalf("failed to fetch order: %v", err)
		}
		if got.Status != models.OrderStatusPaid {
			t.Fatalf("expected paid order status unchanged, got %d", got.Status)
		}
	})
}
