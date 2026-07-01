package inventory

import (
	"testing"
	"time"

	"GoShop/internal/testutil"
	"GoShop/models"
)

func TestInventoryService(t *testing.T) {
	db := testutil.SetupTestDB(t)
	svc := NewService(db)

	userID := uint(1)
	orderID := "TEST-ORDER-1001"
	skuID := uint(1) // SeedProducts 中 SKU 1 初始库存为 87

	t.Run("ReserveStock_Success", func(t *testing.T) {
		tx := db.Begin()
		defer tx.Rollback()

		items := []ReserveItem{
			{SkuID: skuID, Quantity: 5},
		}
		expireAt := time.Now().Add(10 * time.Minute)

		err := svc.ReserveStock(tx, orderID, userID, items, expireAt)
		if err != nil {
			t.Fatalf("ReserveStock failed: %v", err)
		}

		// 验证库存表
		var inv models.SkuInventory
		if err := tx.First(&inv, "sku_id = ?", skuID).Error; err != nil {
			t.Fatalf("failed to query inventory: %v", err)
		}
		if inv.Available != 82 { // 87 - 5
			t.Errorf("expected available 82, got %d", inv.Available)
		}
		if inv.Reserved != 5 {
			t.Errorf("expected reserved 5, got %d", inv.Reserved)
		}

		// 验证预占记录
		var rsv models.InventoryReservation
		rsvID := ReservationID(orderID, skuID)
		if err := tx.First(&rsv, "id = ?", rsvID).Error; err != nil {
			t.Fatalf("failed to query reservation: %v", err)
		}
		if rsv.Quantity != 5 {
			t.Errorf("expected quantity 5, got %d", rsv.Quantity)
		}
		if rsv.Status != models.ReservationStatusReserved {
			t.Errorf("expected status %d, got %d", models.ReservationStatusReserved, rsv.Status)
		}

		// 验证 Journal
		var journals []models.InventoryJournal
		if err := tx.Where("order_id = ? AND change_type = ?", orderID, "RESERVE").Find(&journals).Error; err != nil {
			t.Fatalf("failed to query journal: %v", err)
		}
		if len(journals) != 1 {
			t.Errorf("expected 1 reserve journal, got %d", len(journals))
		}
		if journals[0].Quantity != 5 {
			t.Errorf("expected journal quantity 5, got %d", journals[0].Quantity)
		}

		tx.Commit()
	})

	t.Run("ReserveStock_Insufficient", func(t *testing.T) {
		tx := db.Begin()
		defer tx.Rollback()

		// 试图预占 100 件（当前只剩 82 件，因为上一段测试已经 commit 扣减了 5 件）
		items := []ReserveItem{
			{SkuID: skuID, Quantity: 100},
		}
		expireAt := time.Now().Add(10 * time.Minute)

		err := svc.ReserveStock(tx, "TEST-ORDER-1002", userID, items, expireAt)
		if err == nil {
			t.Fatalf("expected error due to insufficient stock")
		}
	})

	t.Run("ConfirmOrderReservations", func(t *testing.T) {
		tx := db.Begin()
		defer tx.Rollback()

		// 对已预占的订单 "TEST-ORDER-1001" 进行确认扣减
		err := svc.ConfirmOrderReservations(tx, orderID)
		if err != nil {
			t.Fatalf("ConfirmOrderReservations failed: %v", err)
		}

		// 验证库存表
		var inv models.SkuInventory
		if err := tx.First(&inv, "sku_id = ?", skuID).Error; err != nil {
			t.Fatalf("failed to query inventory: %v", err)
		}
		if inv.Reserved != 0 { // 5 -> 0
			t.Errorf("expected reserved 0, got %d", inv.Reserved)
		}
		if inv.Sold != 5 { // 0 -> 5
			t.Errorf("expected sold 5, got %d", inv.Sold)
		}

		// 验证预占记录状态
		var rsv models.InventoryReservation
		rsvID := ReservationID(orderID, skuID)
		if err := tx.First(&rsv, "id = ?", rsvID).Error; err != nil {
			t.Fatalf("failed to query reservation: %v", err)
		}
		if rsv.Status != models.ReservationStatusConfirmed {
			t.Errorf("expected status %d, got %d", models.ReservationStatusConfirmed, rsv.Status)
		}

		tx.Commit()
	})

	t.Run("ReleaseOrderReservations", func(t *testing.T) {
		tx := db.Begin()
		defer tx.Rollback()

		// 先为新订单预占库存
		newOrderID := "TEST-ORDER-1003"
		items := []ReserveItem{
			{SkuID: skuID, Quantity: 10},
		}
		expireAt := time.Now().Add(10 * time.Minute)
		err := svc.ReserveStock(tx, newOrderID, userID, items, expireAt)
		if err != nil {
			t.Fatalf("ReserveStock failed: %v", err)
		}

		// 释放预占
		err = svc.ReleaseOrderReservations(tx, newOrderID)
		if err != nil {
			t.Fatalf("ReleaseOrderReservations failed: %v", err)
		}

		// 验证库存：Available 应该加回 10
		var inv models.SkuInventory
		if err := tx.First(&inv, "sku_id = ?", skuID).Error; err != nil {
			t.Fatalf("failed to query inventory: %v", err)
		}
		// Sku 初始 87，第一个测试扣 5 (剩 82) 并 commit，第三个测试确认扣减（Available 依然是 82，Sold=5, Reserved=0）。
		// 第四个测试这里：预占 10（Available 变为 72，Reserved=10），释放后（Available 变回 82，Reserved=0）。
		if inv.Available != 82 {
			t.Errorf("expected available 82 after release, got %d", inv.Available)
		}
		if inv.Reserved != 0 {
			t.Errorf("expected reserved 0 after release, got %d", inv.Reserved)
		}

		// 验证预占记录状态为 Released
		var rsv models.InventoryReservation
		rsvID := ReservationID(newOrderID, skuID)
		if err := tx.First(&rsv, "id = ?", rsvID).Error; err != nil {
			t.Fatalf("failed to query reservation: %v", err)
		}
		if rsv.Status != models.ReservationStatusReleased {
			t.Errorf("expected status %d, got %d", models.ReservationStatusReleased, rsv.Status)
		}

		tx.Commit()
	})
}
