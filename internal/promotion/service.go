package promotion

import (
	"fmt"
	"time"

	"GoShop/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type CouponCandidate struct {
	UserCouponID   uint
	Available      bool
	Reason         string
	DiscountAmount int
}

type Service struct {
	DB *gorm.DB
}

func NewService(db *gorm.DB) Service {
	return Service{DB: db}
}

func (s Service) CouponCandidates(userID, selectedUserCouponID uint, subtotal int) []CouponCandidate {
	now := time.Now()
	var userCoupons []models.UserCoupon
	query := s.DB.Preload("Coupon").
		Joins("JOIN coupons ON coupons.id = user_coupons.coupon_id").
		Where("user_coupons.user_id = ? AND user_coupons.status = ?", userID, models.UserCouponStatusAvailable).
		Where("coupons.end_time >= ?", now)
	if err := query.Find(&userCoupons).Error; err != nil {
		return nil
	}

	seen := make(map[uint]bool, len(userCoupons)+1)
	candidates := make([]CouponCandidate, 0, len(userCoupons)+1)
	for _, userCoupon := range userCoupons {
		seen[userCoupon.ID] = true
		candidates = append(candidates, buildCandidate(userCoupon, subtotal, now))
	}

	if selectedUserCouponID > 0 && !seen[selectedUserCouponID] {
		var selected models.UserCoupon
		if err := s.DB.Preload("Coupon").
			First(&selected, "id = ? AND user_id = ?", selectedUserCouponID, userID).Error; err == nil {
			candidates = append(candidates, buildCandidate(selected, subtotal, now))
		}
	}

	return candidates
}

func (s Service) LockCouponForOrder(tx *gorm.DB, userID, userCouponID uint, orderID string, subtotal int) (int, error) {
	if userCouponID == 0 {
		return 0, nil
	}

	var userCoupon models.UserCoupon
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Preload("Coupon").
		First(&userCoupon, "id = ? AND user_id = ?", userCouponID, userID).Error; err != nil {
		return 0, err
	}

	if userCoupon.Status == models.UserCouponStatusLocked && userCoupon.LockedOrderID == orderID {
		return CouponDiscount(userCoupon.Coupon, subtotal), nil
	}
	if userCoupon.Status != models.UserCouponStatusAvailable {
		return 0, fmt.Errorf("优惠券不可用或已失效")
	}

	now := time.Now()
	if err := validateCoupon(userCoupon, subtotal, now); err != nil {
		return 0, err
	}

	discount := CouponDiscount(userCoupon.Coupon, subtotal)
	lockedAt := now
	result := tx.Model(&models.UserCoupon{}).
		Where("id = ? AND user_id = ? AND status = ?", userCouponID, userID, models.UserCouponStatusAvailable).
		Updates(map[string]interface{}{
			"status":          models.UserCouponStatusLocked,
			"locked_order_id": orderID,
			"locked_at":       &lockedAt,
			"updated_at":      now,
		})
	if result.Error != nil {
		return 0, result.Error
	}
	if result.RowsAffected != 1 {
		return 0, fmt.Errorf("优惠券不可用或已失效")
	}
	return discount, nil
}

func (s Service) ConfirmCouponUsed(tx *gorm.DB, userID, userCouponID uint, orderID string) error {
	if userCouponID == 0 {
		return nil
	}

	var userCoupon models.UserCoupon
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&userCoupon, "id = ? AND user_id = ?", userCouponID, userID).Error; err != nil {
		return err
	}
	if userCoupon.Status == models.UserCouponStatusUsed {
		return nil
	}
	if userCoupon.Status != models.UserCouponStatusLocked || userCoupon.LockedOrderID != orderID {
		return fmt.Errorf("优惠券未被当前订单锁定")
	}

	now := time.Now()
	return tx.Model(&models.UserCoupon{}).
		Where("id = ? AND user_id = ?", userCouponID, userID).
		Updates(map[string]interface{}{
			"status":          models.UserCouponStatusUsed,
			"used_at":         &now,
			"locked_order_id": "",
			"locked_at":       nil,
			"updated_at":      now,
		}).Error
}

func (s Service) ReleaseCouponLock(tx *gorm.DB, userCouponID uint, orderID string) error {
	if userCouponID == 0 {
		return nil
	}

	var userCoupon models.UserCoupon
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&userCoupon, "id = ?", userCouponID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil
		}
		return err
	}
	if userCoupon.Status != models.UserCouponStatusLocked || userCoupon.LockedOrderID != orderID {
		return nil
	}

	now := time.Now()
	return tx.Model(&models.UserCoupon{}).
		Where("id = ? AND status = ? AND locked_order_id = ?", userCouponID, models.UserCouponStatusLocked, orderID).
		Updates(map[string]interface{}{
			"status":          models.UserCouponStatusAvailable,
			"used_at":         nil,
			"locked_order_id": "",
			"locked_at":       nil,
			"updated_at":      now,
		}).Error
}

func CouponDiscount(coupon models.Coupon, subtotal int) int {
	discount := 0
	switch coupon.Type {
	case 1, 3:
		discount = coupon.Value
	case 2:
		discount = subtotal * (100 - coupon.Value) / 100
	}
	if discount > subtotal {
		return subtotal
	}
	if discount < 0 {
		return 0
	}
	return discount
}

func buildCandidate(userCoupon models.UserCoupon, subtotal int, now time.Time) CouponCandidate {
	candidate := CouponCandidate{UserCouponID: userCoupon.ID, Available: true}
	if err := validateCoupon(userCoupon, subtotal, now); err != nil {
		candidate.Available = false
		candidate.Reason = err.Error()
		return candidate
	}
	candidate.DiscountAmount = CouponDiscount(userCoupon.Coupon, subtotal)
	return candidate
}

func validateCoupon(userCoupon models.UserCoupon, subtotal int, now time.Time) error {
	switch userCoupon.Status {
	case models.UserCouponStatusAvailable:
	case models.UserCouponStatusLocked:
		return fmt.Errorf("优惠券已被订单锁定")
	default:
		return fmt.Errorf("优惠券已使用或已失效")
	}
	if userCoupon.Coupon.StartTime.After(now) || userCoupon.Coupon.EndTime.Before(now) {
		return fmt.Errorf("优惠券不在有效期内")
	}
	if subtotal < userCoupon.Coupon.MinAmount {
		return fmt.Errorf("未达到满 %d 元使用门槛", userCoupon.Coupon.MinAmount/100)
	}
	return nil
}
