package models

import (
	"time"
)

// Address 用户收货地址模型
type Address struct {
	ID            uint            `gorm:"primaryKey;column:id;autoIncrement" json:"id"`
	UserID        uint            `gorm:"column:user_id;not null;index" json:"userId"`
	ReceiverName  EncryptedString `gorm:"column:receiver_name;type:varchar(256);not null" json:"receiverName"`
	ReceiverPhone EncryptedString `gorm:"column:receiver_phone;type:varchar(256);not null" json:"receiverPhone"`
	Province      string          `gorm:"column:province;type:varchar(64);not null" json:"province"`
	City          string          `gorm:"column:city;type:varchar(64);not null" json:"city"`
	District      string          `gorm:"column:district;type:varchar(64);not null" json:"district"`
	DetailAddress EncryptedString `gorm:"column:detail_address;type:varchar(512);not null" json:"detailAddress"`
	IsDefault     bool            `gorm:"column:is_default;default:false;not null" json:"isDefault"`
	CreatedAt     time.Time       `gorm:"column:created_at;default:CURRENT_TIMESTAMP" json:"createdAt"`
	UpdatedAt     time.Time       `gorm:"column:updated_at;default:CURRENT_TIMESTAMP" json:"updatedAt"`
}
