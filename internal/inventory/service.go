package inventory

import (
	"fmt"
	"time"

	"GoShop/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ReserveItem struct {
	SkuID    uint
	Quantity int
}

type Service struct {
	DB *gorm.DB
}

func NewService(db *gorm.DB) Service {
	return Service{DB: db}
}

func (s Service) ReserveStock(tx *gorm.DB, orderID string, userID uint, items []ReserveItem, expireAt time.Time) error {
	for _, item := range items {
		if item.Quantity <= 0 {
			return fmt.Errorf("库存预占数量必须大于 0")
		}

		inv, err := ensureInventory(tx, item.SkuID)
		if err != nil {
			return err
		}
		if inv.Available < item.Quantity {
			return fmt.Errorf("SKU %d 库存不足，仅剩 %d 件", item.SkuID, inv.Available)
		}

		inv.Available -= item.Quantity
		inv.Reserved += item.Quantity
		inv.Version++
		if err := tx.Save(&inv).Error; err != nil {
			return err
		}

		reservationID := ReservationID(orderID, item.SkuID)
		reservation := models.InventoryReservation{
			ID:       reservationID,
			OrderID:  orderID,
			UserID:   userID,
			SkuID:    item.SkuID,
			Quantity: item.Quantity,
			Status:   models.ReservationStatusReserved,
			ExpireAt: expireAt,
		}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&reservation).Error; err != nil {
			return err
		}
		if err := journal(tx, item.SkuID, orderID, reservationID, "RESERVE", item.Quantity); err != nil {
			return err
		}
	}
	return nil
}

func (s Service) ConfirmOrderReservations(tx *gorm.DB, orderID string) error {
	var reservations []models.InventoryReservation
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("order_id = ? AND status = ?", orderID, models.ReservationStatusReserved).
		Find(&reservations).Error; err != nil {
		return err
	}
	for _, reservation := range reservations {
		inv, err := ensureInventory(tx, reservation.SkuID)
		if err != nil {
			return err
		}
		if inv.Reserved < reservation.Quantity {
			return fmt.Errorf("SKU %d 预占库存不足，无法确认", reservation.SkuID)
		}
		inv.Reserved -= reservation.Quantity
		inv.Sold += reservation.Quantity
		inv.Version++
		if err := tx.Save(&inv).Error; err != nil {
			return err
		}
		if err := tx.Model(&reservation).Updates(map[string]interface{}{
			"status":     models.ReservationStatusConfirmed,
			"updated_at": time.Now(),
		}).Error; err != nil {
			return err
		}
		if err := journal(tx, reservation.SkuID, orderID, reservation.ID, "CONFIRM", reservation.Quantity); err != nil {
			return err
		}
	}
	return nil
}

func (s Service) ReleaseOrderReservations(tx *gorm.DB, orderID string) error {
	var reservations []models.InventoryReservation
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("order_id = ? AND status = ?", orderID, models.ReservationStatusReserved).
		Find(&reservations).Error; err != nil {
		return err
	}
	for _, reservation := range reservations {
		inv, err := ensureInventory(tx, reservation.SkuID)
		if err != nil {
			return err
		}
		if inv.Reserved < reservation.Quantity {
			return fmt.Errorf("SKU %d 预占库存不足，无法释放", reservation.SkuID)
		}
		inv.Reserved -= reservation.Quantity
		inv.Available += reservation.Quantity
		inv.Version++
		if err := tx.Save(&inv).Error; err != nil {
			return err
		}
		if err := tx.Model(&reservation).Updates(map[string]interface{}{
			"status":     models.ReservationStatusReleased,
			"updated_at": time.Now(),
		}).Error; err != nil {
			return err
		}
		if err := journal(tx, reservation.SkuID, orderID, reservation.ID, "RELEASE", reservation.Quantity); err != nil {
			return err
		}
	}
	return nil
}

func (s Service) RestockSoldForOrder(tx *gorm.DB, orderID string) error {
	var reservations []models.InventoryReservation
	if err := tx.Where("order_id = ? AND status = ?", orderID, models.ReservationStatusConfirmed).Find(&reservations).Error; err != nil {
		return err
	}
	for _, reservation := range reservations {
		inv, err := ensureInventory(tx, reservation.SkuID)
		if err != nil {
			return err
		}
		if inv.Sold < reservation.Quantity {
			return fmt.Errorf("SKU %d 已售库存不足，无法退款回补", reservation.SkuID)
		}
		inv.Sold -= reservation.Quantity
		inv.Available += reservation.Quantity
		inv.Version++
		if err := tx.Save(&inv).Error; err != nil {
			return err
		}
		if err := journal(tx, reservation.SkuID, orderID, reservation.ID, "REFUND_RESTOCK", reservation.Quantity); err != nil {
			return err
		}
	}
	return nil
}

func ReservationID(orderID string, skuID uint) string {
	return fmt.Sprintf("RSV-%s-%d", orderID, skuID)
}

func ensureInventory(tx *gorm.DB, skuID uint) (models.SkuInventory, error) {
	var inv models.SkuInventory
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&inv, "sku_id = ?", skuID).Error
	if err == nil {
		return inv, nil
	}
	if err != gorm.ErrRecordNotFound {
		return inv, err
	}

	var sku models.Sku
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&sku, "id = ?", skuID).Error; err != nil {
		return inv, err
	}
	inv = models.SkuInventory{SkuID: sku.ID, Available: sku.Stock}
	if err := tx.Create(&inv).Error; err != nil {
		return inv, err
	}
	return inv, nil
}

func journal(tx *gorm.DB, skuID uint, orderID, reservationID, changeType string, quantity int) error {
	return tx.Create(&models.InventoryJournal{
		SkuID:         skuID,
		OrderID:       orderID,
		ReservationID: reservationID,
		ChangeType:    changeType,
		Quantity:      quantity,
	}).Error
}
