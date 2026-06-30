package models

import (
	"time"
)

// CartItem 云端购物车模型
type CartItem struct {
	ID        uint      `gorm:"primaryKey;column:id;autoIncrement" json:"id"`
	UserID    uint      `gorm:"column:user_id;not null;index" json:"userId"`
	SkuID     uint      `gorm:"column:sku_id;not null" json:"skuId"`
	Quantity  int       `gorm:"column:quantity;not null" json:"quantity"`
	CreatedAt time.Time `gorm:"column:created_at;default:CURRENT_TIMESTAMP" json:"createdAt"`
	UpdatedAt time.Time `gorm:"column:updated_at;default:CURRENT_TIMESTAMP" json:"updatedAt"`

	// 关联商品 SKU 基本信息
	Sku       Sku       `gorm:"foreignKey:SkuID" json:"sku"`
}
