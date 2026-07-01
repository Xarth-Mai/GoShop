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

	t.Run("Valid Coupon - Success and Consume Coupon", func(t *testing.T) {
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

		// 检查优惠券状态，应该更新为已使用 (status=1)
		var uc models.UserCoupon
		if err := db.First(&uc, "id = ?", 11).Error; err != nil {
			t.Fatalf("failed to check user coupon state: %v", err)
		}
		if uc.Status != 1 {
			t.Errorf("expected user coupon status to be 1 (used), got %d", uc.Status)
		}
		if uc.UsedAt == nil {
			t.Errorf("expected user coupon UsedAt to be populated, got nil")
		}
	})
}
