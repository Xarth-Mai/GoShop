package core_test

import (
	"testing"

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
}
