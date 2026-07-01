package models

import (
	"testing"

	"github.com/glebarez/sqlite"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

func openSeedTestDB(t *testing.T, models ...interface{}) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(models...); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	return db
}

func TestSeedProductCatalogDoesNotRequireUserTables(t *testing.T) {
	db := openSeedTestDB(t, &Category{}, &Spu{}, &Sku{})
	if err := SeedServiceData(db, "goshop-product-service"); err != nil {
		t.Fatalf("seed product service data: %v", err)
	}

	var categoryCount, spuCount, skuCount int64
	db.Model(&Category{}).Count(&categoryCount)
	db.Model(&Spu{}).Count(&spuCount)
	db.Model(&Sku{}).Count(&skuCount)
	if categoryCount != 4 || spuCount != 4 || skuCount != 10 {
		t.Fatalf("unexpected catalog counts categories=%d spus=%d skus=%d", categoryCount, spuCount, skuCount)
	}
}

func TestSeedPromotionDoesNotRequireUserTablesAndIsIdempotent(t *testing.T) {
	db := openSeedTestDB(t, &Coupon{}, &UserCoupon{})
	for i := 0; i < 2; i++ {
		if err := SeedServiceData(db, "goshop-promotion-service"); err != nil {
			t.Fatalf("seed promotion service data: %v", err)
		}
	}

	var couponCount, userCouponCount int64
	db.Model(&Coupon{}).Count(&couponCount)
	db.Model(&UserCoupon{}).Where("user_id = ?", 1).Count(&userCouponCount)
	if couponCount != 4 || userCouponCount != 3 {
		t.Fatalf("unexpected promotion counts coupons=%d userCoupons=%d", couponCount, userCouponCount)
	}
}

func TestSeedUserPasswordMatchesDocumentedPassword(t *testing.T) {
	db := openSeedTestDB(t, &User{}, &Address{})
	if err := SeedServiceData(db, "goshop-user-service"); err != nil {
		t.Fatalf("seed user service data: %v", err)
	}

	var user User
	if err := db.First(&user, "username = ?", "test_user").Error; err != nil {
		t.Fatalf("find test user: %v", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte("123456")); err != nil {
		t.Fatalf("seeded test_user password does not match 123456: %v", err)
	}
}

func TestSeedInventoryDoesNotRequireSkuTable(t *testing.T) {
	db := openSeedTestDB(t, &SkuInventory{})
	if err := SeedServiceData(db, "goshop-inventory-service"); err != nil {
		t.Fatalf("seed inventory service data: %v", err)
	}

	var inventoryCount int64
	db.Model(&SkuInventory{}).Count(&inventoryCount)
	if inventoryCount != 10 {
		t.Fatalf("expected 10 inventory rows, got %d", inventoryCount)
	}
}
