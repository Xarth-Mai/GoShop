package order

import (
	"fmt"
	"time"

	"GoShop/internal/checkout"
	"GoShop/internal/inventory"
	"GoShop/models"

	"gorm.io/gorm"
)

type CreateRequest = checkout.PreviewRequest

type CreateResult struct {
	OrderID     string
	PayExpireAt time.Time
	TotalAmount int
}

type Detail struct {
	Order        models.Order                  `json:"order"`
	StateLogs    []models.OrderStateLog        `json:"stateLogs"`
	PaymentOrder *models.PaymentOrder          `json:"paymentOrder,omitempty"`
	AfterSales   []models.AfterSaleOrder       `json:"afterSales"`
	RefundOrders []models.RefundOrder          `json:"refundOrders"`
	Reservations []models.InventoryReservation `json:"reservations"`
}

type Service struct {
	DB        *gorm.DB
	Checkout  checkout.Service
	Inventory inventory.Service
}

func NewService(db *gorm.DB) Service {
	return Service{
		DB:        db,
		Checkout:  checkout.NewService(db),
		Inventory: inventory.NewService(db),
	}
}

func (s Service) CreateOrder(userID uint, req CreateRequest) (CreateResult, error) {
	var result CreateResult
	err := s.DB.Transaction(func(tx *gorm.DB) error {
		preview, err := checkout.NewService(tx).Calculate(userID, req)
		if err != nil {
			return err
		}
		if req.UserCouponID > 0 && preview.SelectedUserCouponID == 0 {
			return fmt.Errorf("优惠券不可用或已失效")
		}

		var address models.Address
		if err := tx.Where("id = ? AND user_id = ?", req.AddressID, userID).First(&address).Error; err != nil {
			return fmt.Errorf("收货地址不存在")
		}

		orderID := fmt.Sprintf("GS-%d-%d", time.Now().Unix(), time.Now().Nanosecond()%1000)
		payExpireAt := time.Now().Add(60 * time.Second)
		reserveItems := make([]inventory.ReserveItem, 0, len(preview.Items))
		for _, item := range preview.Items {
			reserveItems = append(reserveItems, inventory.ReserveItem{SkuID: item.SkuID, Quantity: item.Quantity})
		}

		orderItems := make([]models.OrderItem, 0, len(preview.Items))
		for _, item := range preview.Items {
			orderItems = append(orderItems, models.OrderItem{
				SkuID:              item.SkuID,
				Price:              item.Price,
				Quantity:           item.Quantity,
				OriginAmount:       item.OriginAmount,
				ItemDiscountAmount: item.ItemDiscountAmount,
				PayableAmount:      item.PayableAmount,
			})
		}

		receiverAddrSnapshot := fmt.Sprintf("%s%s%s%s", address.Province, address.City, address.District, string(address.DetailAddress))
		order := models.Order{
			ID:                  orderID,
			UserID:              userID,
			TotalAmount:         preview.PayableAmount,
			DiscountAmount:      preview.GoodsDiscountAmount,
			GoodsOriginAmount:   preview.GoodsOriginAmount,
			GoodsDiscountAmount: preview.GoodsDiscountAmount,
			ShippingFee:         preview.ShippingFee,
			TaxFee:              preview.TaxFee,
			PayableAmount:       preview.PayableAmount,
			Status:              models.OrderStatusPendingPayment,
			PayStatus:           models.PayStatusUnpaid,
			AfterSaleStatus:     models.AfterSaleStatusNone,
			UserCouponID:        preview.SelectedUserCouponID,
			ReceiverName:        string(address.ReceiverName),
			ReceiverPhone:       string(address.ReceiverPhone),
			ReceiverAddr:        receiverAddrSnapshot,
			PayExpireAt:         &payExpireAt,
			Items:               orderItems,
		}
		if err := tx.Create(&order).Error; err != nil {
			return err
		}
		if err := inventory.NewService(tx).ReserveStock(tx, order.ID, userID, reserveItems, payExpireAt); err != nil {
			return err
		}
		if req.UserCouponID > 0 && preview.SelectedUserCouponID > 0 {
			now := time.Now()
			if err := tx.Model(&models.UserCoupon{}).
				Where("id = ? AND user_id = ? AND status = ?", req.UserCouponID, userID, 0).
				Updates(map[string]interface{}{"status": 1, "used_at": &now, "updated_at": now}).Error; err != nil {
				return err
			}
		}

		if preview.SelectedUserCouponID > 0 && preview.GoodsDiscountAmount > 0 {
			for _, item := range order.Items {
				if item.ItemDiscountAmount <= 0 {
					continue
				}
				allocation := models.OrderPromotionAllocation{
					OrderID:            order.ID,
					OrderItemID:        item.ID,
					SkuID:              item.SkuID,
					UserCouponID:       preview.SelectedUserCouponID,
					DiscountType:       1,
					DiscountAmount:     item.ItemDiscountAmount,
					AllocationSnapshot: fmt.Sprintf(`{"origin_amount":%d,"payable_amount":%d}`, item.OriginAmount, item.PayableAmount),
				}
				if err := tx.Create(&allocation).Error; err != nil {
					return err
				}
			}
		}

		if err := appendStateLog(tx, order.ID, 0, models.OrderStatusPendingPayment, userID, "ORDER_CREATED", "订单创建并预占库存"); err != nil {
			return err
		}

		var skuIDs []uint
		for _, item := range req.Items {
			skuIDs = append(skuIDs, item.SkuID)
		}
		if len(skuIDs) > 0 {
			if err := tx.Where("user_id = ? AND sku_id IN ?", userID, skuIDs).Delete(&models.CartItem{}).Error; err != nil {
				return err
			}
		}

		result = CreateResult{OrderID: orderID, PayExpireAt: payExpireAt, TotalAmount: preview.PayableAmount}
		return nil
	})
	return result, err
}

func (s Service) CancelPendingOrder(orderID, reason string) error {
	return s.DB.Transaction(func(tx *gorm.DB) error {
		var order models.Order
		if err := tx.Preload("Items").First(&order, "id = ?", orderID).Error; err != nil {
			return err
		}
		if order.Status != models.OrderStatusPendingPayment {
			return nil
		}

		fromStatus := order.Status
		order.Status = models.OrderStatusCanceled
		if err := tx.Save(&order).Error; err != nil {
			return err
		}
		if order.UserCouponID > 0 {
			now := time.Now()
			if err := tx.Model(&models.UserCoupon{}).Where("id = ? AND status = ?", order.UserCouponID, 1).Updates(map[string]interface{}{
				"status":     0,
				"used_at":    nil,
				"updated_at": now,
			}).Error; err != nil {
				return err
			}
		}
		if err := inventory.NewService(tx).ReleaseOrderReservations(tx, order.ID); err != nil {
			return err
		}
		return appendStateLog(tx, order.ID, fromStatus, models.OrderStatusCanceled, order.UserID, "ORDER_TIMEOUT_CANCELED", reason)
	})
}

func (s Service) GetOrderDetail(userID uint, orderID string) (Detail, error) {
	var detail Detail
	if err := s.DB.Preload("Items.Sku").First(&detail.Order, "id = ? AND user_id = ?", orderID, userID).Error; err != nil {
		return detail, err
	}

	if err := s.DB.Where("order_id = ?", orderID).Order("created_at asc").Find(&detail.StateLogs).Error; err != nil {
		return detail, err
	}

	var paymentOrder models.PaymentOrder
	if err := s.DB.Where("order_id = ?", orderID).Order("created_at desc").First(&paymentOrder).Error; err == nil {
		detail.PaymentOrder = &paymentOrder
	} else if err != gorm.ErrRecordNotFound {
		return detail, err
	}

	if err := s.DB.Preload("Items").Where("order_id = ?", orderID).Order("created_at desc").Find(&detail.AfterSales).Error; err != nil {
		return detail, err
	}
	if err := s.DB.Where("order_id = ?", orderID).Order("created_at desc").Find(&detail.RefundOrders).Error; err != nil {
		return detail, err
	}
	if err := s.DB.Where("order_id = ?", orderID).Order("created_at asc").Find(&detail.Reservations).Error; err != nil {
		return detail, err
	}

	return detail, nil
}

func appendStateLog(tx *gorm.DB, orderID string, fromStatus, toStatus int, operatorID uint, event, remark string) error {
	return tx.Create(&models.OrderStateLog{
		OrderID:      orderID,
		FromStatus:   fromStatus,
		ToStatus:     toStatus,
		OperatorType: 1,
		OperatorID:   operatorID,
		Event:        event,
		Remark:       remark,
	}).Error
}
