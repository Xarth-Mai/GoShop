package checkout

import (
	"testing"

	"GoShop/internal/testutil"
	"GoShop/models"
)

func TestCalculate(t *testing.T) {
	db := testutil.SetupTestDB(t)
	svc := NewService(db)

	userID := uint(1)    // SeedServiceData 默认会生成 test_user (ID=1)
	addressID := uint(1) // 默认生成的张小华地址 ID=1

	// 准备一个便宜的 SKU 用于测试运费计算
	cheapSku := models.Sku{
		ID:    20,
		SpuID: 1,
		Title: "Cheap Sku for Shipping Test",
		Price: 5000, // 50元
		Stock: 10,
	}
	if err := db.Create(&cheapSku).Error; err != nil {
		t.Fatalf("failed to create cheap sku: %v", err)
	}

	t.Run("empty items", func(t *testing.T) {
		req := PreviewRequest{
			Items:     []ItemReq{},
			AddressID: addressID,
		}
		_, err := svc.Calculate(userID, req)
		if err == nil {
			t.Fatalf("expected error for empty items")
		}
	})

	t.Run("invalid quantity", func(t *testing.T) {
		req := PreviewRequest{
			Items: []ItemReq{
				{SkuID: cheapSku.ID, Quantity: 0},
			},
			AddressID: addressID,
		}
		_, err := svc.Calculate(userID, req)
		if err == nil {
			t.Fatalf("expected error for zero quantity")
		}
	})

	t.Run("sku not found", func(t *testing.T) {
		req := PreviewRequest{
			Items: []ItemReq{
				{SkuID: 9999, Quantity: 1},
			},
			AddressID: addressID,
		}
		_, err := svc.Calculate(userID, req)
		if err == nil {
			t.Fatalf("expected error for non-existent sku")
		}
	})

	t.Run("address not found", func(t *testing.T) {
		req := PreviewRequest{
			Items: []ItemReq{
				{SkuID: cheapSku.ID, Quantity: 1},
			},
			AddressID: 9999,
		}
		_, err := svc.Calculate(userID, req)
		if err == nil {
			t.Fatalf("expected error for non-existent address")
		}
	})

	t.Run("shipping fee and tax calculation - cheap sku", func(t *testing.T) {
		// 50元商品，不足99元。运费应为 10元 (1000)。税费应为 50元 * 5% = 2.5元 (250)。
		req := PreviewRequest{
			Items: []ItemReq{
				{SkuID: cheapSku.ID, Quantity: 1},
			},
			AddressID: addressID,
		}
		preview, err := svc.Calculate(userID, req)
		if err != nil {
			t.Fatalf("calculate failed: %v", err)
		}
		if preview.GoodsOriginAmount != 5000 {
			t.Errorf("expected origin amount 5000, got %d", preview.GoodsOriginAmount)
		}
		if preview.ShippingFee != 1000 {
			t.Errorf("expected shipping fee 1000, got %d", preview.ShippingFee)
		}
		if preview.TaxFee != 250 {
			t.Errorf("expected tax fee 250, got %d", preview.TaxFee)
		}
		expectedPayable := 5000 + 1000 + 250
		if preview.PayableAmount != expectedPayable {
			t.Errorf("expected payable %d, got %d", expectedPayable, preview.PayableAmount)
		}
	})

	t.Run("free shipping - expensive sku", func(t *testing.T) {
		// SKU 1 Haiku (128GB) 价格 39900 (399元)，满99免运费。税费 39900 * 5% = 1995。
		req := PreviewRequest{
			Items: []ItemReq{
				{SkuID: 1, Quantity: 1},
			},
			AddressID: addressID,
		}
		preview, err := svc.Calculate(userID, req)
		if err != nil {
			t.Fatalf("calculate failed: %v", err)
		}
		if preview.ShippingFee != 0 {
			t.Errorf("expected shipping fee 0, got %d", preview.ShippingFee)
		}
		if preview.TaxFee != 1995 {
			t.Errorf("expected tax fee 1995, got %d", preview.TaxFee)
		}
		expectedPayable := 39900 + 0 + 1995
		if preview.PayableAmount != expectedPayable {
			t.Errorf("expected payable %d, got %d", expectedPayable, preview.PayableAmount)
		}
	})

	t.Run("with 10 yuan coupon - success", func(t *testing.T) {
		// UserCouponID=1 对应 10元无门槛券。
		// 购买 Sku 1 (39900)，免运费，税 1995。
		// 折扣 1000。
		// 最终应付：39900 + 1995 - 1000 = 40895。
		req := PreviewRequest{
			Items: []ItemReq{
				{SkuID: 1, Quantity: 1},
			},
			AddressID:    addressID,
			UserCouponID: 1,
		}
		preview, err := svc.Calculate(userID, req)
		if err != nil {
			t.Fatalf("calculate failed: %v", err)
		}
		if preview.SelectedUserCouponID != 1 {
			t.Errorf("expected coupon 1 to be selected, got %d", preview.SelectedUserCouponID)
		}
		if preview.GoodsDiscountAmount != 1000 {
			t.Errorf("expected discount 1000, got %d", preview.GoodsDiscountAmount)
		}
		expectedPayable := 39900 + 1995 - 1000
		if preview.PayableAmount != expectedPayable {
			t.Errorf("expected payable %d, got %d", expectedPayable, preview.PayableAmount)
		}
	})

	t.Run("with coupon min amount not met - fail to apply but calculate proceeds", func(t *testing.T) {
		// UserCouponID=2 对应 满500减50。门槛为 50000。
		// 购买 Sku 1 Haiku (39900)，不足500元，优惠券不满足门槛。
		// 不应被选用，SelectedUserCouponID 应为 0。
		req := PreviewRequest{
			Items: []ItemReq{
				{SkuID: 1, Quantity: 1},
			},
			AddressID:    addressID,
			UserCouponID: 2,
		}
		preview, err := svc.Calculate(userID, req)
		if err != nil {
			t.Fatalf("calculate failed: %v", err)
		}
		if preview.SelectedUserCouponID != 0 {
			t.Errorf("expected no coupon selected, got %d", preview.SelectedUserCouponID)
		}
		if preview.GoodsDiscountAmount != 0 {
			t.Errorf("expected discount 0, got %d", preview.GoodsDiscountAmount)
		}
	})

	t.Run("with coupon min amount met - success", func(t *testing.T) {
		// UserCouponID=2 满500减50。
		// 购买 2 个 Sku 1 (39900 * 2 = 79800)，满 500 元。
		// 折扣 5000。
		req := PreviewRequest{
			Items: []ItemReq{
				{SkuID: 1, Quantity: 2},
			},
			AddressID:    addressID,
			UserCouponID: 2,
		}
		preview, err := svc.Calculate(userID, req)
		if err != nil {
			t.Fatalf("calculate failed: %v", err)
		}
		if preview.SelectedUserCouponID != 2 {
			t.Errorf("expected coupon 2 selected, got %d", preview.SelectedUserCouponID)
		}
		if preview.GoodsDiscountAmount != 5000 {
			t.Errorf("expected discount 5000, got %d", preview.GoodsDiscountAmount)
		}
		expectedPayable := 79800 + 0 + (79800 * 5 / 100) - 5000
		if preview.PayableAmount != expectedPayable {
			t.Errorf("expected payable %d, got %d", expectedPayable, preview.PayableAmount)
		}
	})
}
