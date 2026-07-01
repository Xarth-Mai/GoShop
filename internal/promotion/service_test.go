package promotion

import (
	"fmt"
	"testing"
	"time"

	"GoShop/internal/testutil"
	"GoShop/models"

	"gorm.io/gorm"
)

func TestCouponLockConfirmRelease(t *testing.T) {
	db := testutil.SetupTestDB(t)
	svc := NewService(db)
	userID := uint(1)
	now := time.Now()

	coupon := models.Coupon{
		ID:        20,
		Name:      "Promotion Test Coupon",
		Type:      3,
		Value:     1200,
		StartTime: now.Add(-time.Hour),
		EndTime:   now.Add(time.Hour),
	}
	if err := db.Create(&coupon).Error; err != nil {
		t.Fatalf("create coupon: %v", err)
	}
	userCoupon := models.UserCoupon{ID: 20, UserID: userID, CouponID: coupon.ID, Status: models.UserCouponStatusAvailable}
	if err := db.Create(&userCoupon).Error; err != nil {
		t.Fatalf("create user coupon: %v", err)
	}

	err := db.Transaction(func(tx *gorm.DB) error {
		discount, err := svc.LockCouponForOrder(tx, userID, userCoupon.ID, "ORDER-PROMO-1", 5000)
		if err != nil {
			return err
		}
		if discount != 1200 {
			return fmt.Errorf("discount = %d, want 1200", discount)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("lock coupon: %v", err)
	}

	var locked models.UserCoupon
	if err := db.First(&locked, "id = ?", userCoupon.ID).Error; err != nil {
		t.Fatalf("fetch locked coupon: %v", err)
	}
	if locked.Status != models.UserCouponStatusLocked || locked.LockedOrderID != "ORDER-PROMO-1" {
		t.Fatalf("unexpected lock state: status=%d order=%q", locked.Status, locked.LockedOrderID)
	}

	err = db.Transaction(func(tx *gorm.DB) error {
		return svc.ConfirmCouponUsed(tx, userID, userCoupon.ID, "ORDER-PROMO-1")
	})
	if err != nil {
		t.Fatalf("confirm coupon: %v", err)
	}
	var used models.UserCoupon
	if err := db.First(&used, "id = ?", userCoupon.ID).Error; err != nil {
		t.Fatalf("fetch used coupon: %v", err)
	}
	if used.Status != models.UserCouponStatusUsed || used.UsedAt == nil || used.LockedOrderID != "" {
		t.Fatalf("unexpected used state: status=%d usedAt=%v lockedOrder=%q", used.Status, used.UsedAt, used.LockedOrderID)
	}
}

func TestCouponLockFailuresAndRelease(t *testing.T) {
	db := testutil.SetupTestDB(t)
	svc := NewService(db)
	userID := uint(1)
	now := time.Now()

	expired := models.Coupon{ID: 21, Name: "Expired", Type: 3, Value: 100, StartTime: now.Add(-2 * time.Hour), EndTime: now.Add(-time.Hour)}
	if err := db.Create(&expired).Error; err != nil {
		t.Fatalf("create expired coupon: %v", err)
	}
	expiredUC := models.UserCoupon{ID: 21, UserID: userID, CouponID: expired.ID, Status: models.UserCouponStatusAvailable}
	if err := db.Create(&expiredUC).Error; err != nil {
		t.Fatalf("create expired user coupon: %v", err)
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		_, err := svc.LockCouponForOrder(tx, userID, expiredUC.ID, "ORDER-X", 5000)
		return err
	}); err == nil {
		t.Fatalf("expected expired coupon lock to fail")
	}

	active := models.Coupon{ID: 22, Name: "Active", Type: 3, Value: 100, StartTime: now.Add(-time.Hour), EndTime: now.Add(time.Hour)}
	if err := db.Create(&active).Error; err != nil {
		t.Fatalf("create active coupon: %v", err)
	}
	lockedUC := models.UserCoupon{ID: 22, UserID: userID, CouponID: active.ID, Status: models.UserCouponStatusLocked, LockedOrderID: "ORDER-OLD"}
	if err := db.Create(&lockedUC).Error; err != nil {
		t.Fatalf("create locked user coupon: %v", err)
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		_, err := svc.LockCouponForOrder(tx, userID, lockedUC.ID, "ORDER-NEW", 5000)
		return err
	}); err == nil {
		t.Fatalf("expected already locked coupon to fail")
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		return svc.ReleaseCouponLock(tx, lockedUC.ID, "ORDER-OLD")
	}); err != nil {
		t.Fatalf("release lock: %v", err)
	}
	var released models.UserCoupon
	if err := db.First(&released, "id = ?", lockedUC.ID).Error; err != nil {
		t.Fatalf("fetch released coupon: %v", err)
	}
	if released.Status != models.UserCouponStatusAvailable || released.LockedOrderID != "" {
		t.Fatalf("expected released coupon, got status=%d order=%q", released.Status, released.LockedOrderID)
	}
}
