package models

import "time"

const (
	ReservationStatusReserved  = 10
	ReservationStatusConfirmed = 20
	ReservationStatusReleased  = 30
	ReservationStatusExpired   = 40
)

type SkuInventory struct {
	SkuID     uint      `gorm:"primaryKey;column:sku_id" json:"skuId"`
	Available int       `gorm:"column:available;not null" json:"available"`
	Reserved  int       `gorm:"column:reserved;default:0;not null" json:"reserved"`
	Sold      int       `gorm:"column:sold;default:0;not null" json:"sold"`
	Version   int       `gorm:"column:version;default:0;not null" json:"version"`
	UpdatedAt time.Time `gorm:"column:updated_at;default:CURRENT_TIMESTAMP" json:"updatedAt"`
}

type InventoryReservation struct {
	ID        string    `gorm:"primaryKey;column:id;type:varchar(64)" json:"id"`
	OrderID   string    `gorm:"column:order_id;type:varchar(64);not null;index;uniqueIndex:idx_inventory_reservation_order_sku" json:"orderId"`
	UserID    uint      `gorm:"column:user_id;not null;index" json:"userId"`
	SkuID     uint      `gorm:"column:sku_id;not null;index;uniqueIndex:idx_inventory_reservation_order_sku" json:"skuId"`
	Quantity  int       `gorm:"column:quantity;not null" json:"quantity"`
	Status    int       `gorm:"column:status;not null;index" json:"status"`
	ExpireAt  time.Time `gorm:"column:expire_at;not null;index" json:"expireAt"`
	CreatedAt time.Time `gorm:"column:created_at;default:CURRENT_TIMESTAMP" json:"createdAt"`
	UpdatedAt time.Time `gorm:"column:updated_at;default:CURRENT_TIMESTAMP" json:"updatedAt"`
}

type InventoryJournal struct {
	ID            uint      `gorm:"primaryKey;column:id;autoIncrement" json:"id"`
	SkuID         uint      `gorm:"column:sku_id;not null;index" json:"skuId"`
	OrderID       string    `gorm:"column:order_id;type:varchar(64);index" json:"orderId"`
	ReservationID string    `gorm:"column:reservation_id;type:varchar(64);index" json:"reservationId"`
	ChangeType    string    `gorm:"column:change_type;type:varchar(64);not null" json:"changeType"`
	Quantity      int       `gorm:"column:quantity;not null" json:"quantity"`
	CreatedAt     time.Time `gorm:"column:created_at;default:CURRENT_TIMESTAMP" json:"createdAt"`
}
