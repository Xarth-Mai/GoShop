package models

import (
	"time"
)

const (
	UserCouponStatusAvailable = 0
	UserCouponStatusUsed      = 1
	UserCouponStatusExpired   = 2
	UserCouponStatusLocked    = 3
)

// Coupon 优惠券模型
type Coupon struct {
	ID        uint      `gorm:"primaryKey;column:id;autoIncrement" json:"id"`
	Name      string    `gorm:"column:name;type:varchar(64);not null" json:"name"`
	Type      int       `gorm:"column:type;not null" json:"type"`                      // 1: 满减, 2: 折扣, 3: 无门槛
	Value     int       `gorm:"column:value;not null" json:"value"`                    // 满减金额（分）或 折扣百分比（如90代表9折）
	MinAmount int       `gorm:"column:min_amount;default:0;not null" json:"minAmount"` // 最低消费门槛（分）
	StartTime time.Time `gorm:"column:start_time" json:"startTime"`
	EndTime   time.Time `gorm:"column:end_time" json:"endTime"`
	CreatedAt time.Time `gorm:"column:created_at;default:CURRENT_TIMESTAMP" json:"createdAt"`
	UpdatedAt time.Time `gorm:"column:updated_at;default:CURRENT_TIMESTAMP" json:"updatedAt"`
}

// UserCoupon 用户领取的优惠券关联模型
type UserCoupon struct {
	ID            uint       `gorm:"primaryKey;column:id;autoIncrement" json:"id"`
	UserID        uint       `gorm:"column:user_id;not null;index" json:"userId"`
	CouponID      uint       `gorm:"column:coupon_id;not null" json:"couponId"`
	Status        int        `gorm:"column:status;default:0;not null" json:"status"` // 0: 可用, 1: 已使用, 2: 已过期, 3: 已锁定
	UsedAt        *time.Time `gorm:"column:used_at" json:"usedAt,omitempty"`
	LockedOrderID string     `gorm:"column:locked_order_id;type:varchar(64);index" json:"lockedOrderId"`
	LockedAt      *time.Time `gorm:"column:locked_at" json:"lockedAt,omitempty"`
	CreatedAt     time.Time  `gorm:"column:created_at;default:CURRENT_TIMESTAMP" json:"createdAt"`
	UpdatedAt     time.Time  `gorm:"column:updated_at;default:CURRENT_TIMESTAMP" json:"updatedAt"`

	// 关联卡券基本信息
	Coupon Coupon `gorm:"foreignKey:CouponID" json:"coupon"`
}
