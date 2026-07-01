package testutil

import (
	"testing"

	"GoShop/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func SetupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to connect database: %v", err)
	}

	err = db.AutoMigrate(
		&models.User{},
		&models.Address{},
		&models.CartItem{}, // 补充 CartItem 迁移
		&models.Coupon{},
		&models.UserCoupon{},
		&models.Sku{},
		&models.Spu{},
		&models.Category{},
		&models.SkuInventory{},
		&models.InventoryReservation{},
		&models.InventoryJournal{},
		&models.OutboxEvent{},
		&models.InboxEvent{},
		&models.Order{},
		&models.OrderItem{},
		&models.OrderPromotionAllocation{},
		&models.OrderStateLog{},
		&models.PaymentOrder{},
		&models.PaymentTransaction{},
		&models.RefundOrder{},
		&models.AccountingEntry{},
		&models.AfterSaleOrder{},
		&models.AfterSaleItem{},
		&models.DeadLetterOrder{},
	)
	if err != nil {
		t.Fatalf("Failed to auto migrate: %v", err)
	}

	if err := models.SeedProducts(db); err != nil {
		t.Fatalf("Failed to seed data: %v", err)
	}

	return db
}
