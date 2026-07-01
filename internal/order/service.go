package order

import (
	"fmt"
	"time"

	"GoShop/core"
	"GoShop/internal/checkout"
	"GoShop/internal/outbox"
	"GoShop/models"

	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const PendingPaymentTTL = 30 * time.Minute

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
	DB       *gorm.DB
	Checkout checkout.Service
}

func NewService(db *gorm.DB) Service {
	return Service{
		DB:       db,
		Checkout: checkout.NewService(db),
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

		address, err := checkout.LoadAddressSnapshot(tx, userID, req.AddressID)
		if err != nil {
			return fmt.Errorf("收货地址不存在")
		}

		orderID := fmt.Sprintf("GS-%d-%d", time.Now().Unix(), time.Now().Nanosecond()%1000)
		payExpireAt := time.Now().Add(PendingPaymentTTL)
		type InternalReserveItem struct {
			SkuID int `json:"skuId"`
			Qty   int `json:"qty"`
		}
		items := make([]InternalReserveItem, 0, len(preview.Items))
		for _, item := range preview.Items {
			items = append(items, InternalReserveItem{SkuID: int(item.SkuID), Qty: item.Quantity})
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

		receiverAddrSnapshot := address.FullAddress
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
			ReceiverName:        address.ReceiverName,
			ReceiverPhone:       address.ReceiverPhone,
			ReceiverAddr:        receiverAddrSnapshot,
			PayExpireAt:         &payExpireAt,
			Items:               orderItems,
		}
		if err := tx.Create(&order).Error; err != nil {
			return err
		}
		if req.UserCouponID > 0 && preview.SelectedUserCouponID > 0 {
			var lockResp struct {
				DiscountAmount int `json:"discountAmount"`
			}
			err := core.CallInternalService(tx, 8104, "POST", "/api/internal/promotion/lock", map[string]interface{}{
				"userId":       userID,
				"userCouponId": req.UserCouponID,
				"orderId":      order.ID,
				"subtotal":     preview.GoodsOriginAmount,
			}, &lockResp)
			if err != nil {
				return err
			}
			if lockResp.DiscountAmount != preview.GoodsDiscountAmount {
				return fmt.Errorf("优惠券金额已变化，请重新试算")
			}
		}

		err = core.CallInternalService(tx, 8103, "POST", "/api/internal/inventory/reserve", map[string]interface{}{
			"orderId": order.ID,
			"userId":  userID,
			"items":   items,
		}, nil)
		if err != nil {
			return fmt.Errorf("预占库存失败: %v", err)
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
		if err := outbox.NewService().Publish(tx, "order", order.ID, "OrderCreated", map[string]interface{}{
			"orderId":     order.ID,
			"userId":      order.UserID,
			"totalAmount": order.TotalAmount,
			"payExpireAt": payExpireAt,
		}); err != nil {
			return err
		}

		var skuIDs []uint
		for _, item := range req.Items {
			skuIDs = append(skuIDs, item.SkuID)
		}
		if len(skuIDs) > 0 {
			if err := core.CallInternalService(tx, 8108, "POST", "/api/internal/cart/clear-items", map[string]interface{}{
				"userId": userID,
				"skuIds": skuIDs,
			}, nil); err != nil {
				core.Logger.Warn("清理购物车失败", zap.Uint("userId", userID), zap.Error(err))
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
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Preload("Items").First(&order, "id = ?", orderID).Error; err != nil {
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
			if err := core.CallInternalService(tx, 8104, "POST", "/api/internal/promotion/release", map[string]interface{}{
				"userCouponId": order.UserCouponID,
				"orderId":      order.ID,
			}, nil); err != nil {
				core.Logger.Warn("释放优惠券失败", zap.String("orderId", order.ID), zap.Error(err))
			}
		}
		if err := core.CallInternalService(tx, 8103, "POST", "/api/internal/inventory/release", map[string]interface{}{
			"orderId": order.ID,
		}, nil); err != nil {
			core.Logger.Warn("释放预占库存失败", zap.String("orderId", order.ID), zap.Error(err))
		}
		if err := appendStateLog(tx, order.ID, fromStatus, models.OrderStatusCanceled, order.UserID, "ORDER_TIMEOUT_CANCELED", reason); err != nil {
			return err
		}
		return outbox.NewService().Publish(tx, "order", order.ID, "OrderCanceled", map[string]interface{}{
			"orderId": order.ID,
			"userId":  order.UserID,
			"reason":  reason,
		})
	})
}

func (s Service) GetOrderDetail(userID uint, orderID string) (Detail, error) {
	var detail Detail
	if err := s.DB.Preload("Items").First(&detail.Order, "id = ? AND user_id = ?", orderID, userID).Error; err != nil {
		return detail, err
	}
	EnrichOrderItemSKUs(s.DB, detail.Order.Items)

	if err := s.DB.Where("order_id = ?", orderID).Order("created_at asc").Find(&detail.StateLogs).Error; err != nil {
		return detail, err
	}

	var paymentOrder models.PaymentOrder
	if err := core.CallInternalService(s.DB, 8106, "GET", fmt.Sprintf("/api/internal/payments/by-order/%s", orderID), nil, &paymentOrder); err == nil {
		detail.PaymentOrder = &paymentOrder
	}

	var afterSaleResult struct {
		AfterSales   []models.AfterSaleOrder `json:"afterSales"`
		RefundOrders []models.RefundOrder    `json:"refundOrders"`
	}
	if err := core.CallInternalService(s.DB, 8107, "GET", fmt.Sprintf("/api/internal/aftersales/by-order/%s", orderID), nil, &afterSaleResult); err == nil {
		detail.AfterSales = afterSaleResult.AfterSales
		detail.RefundOrders = afterSaleResult.RefundOrders
	}

	var reservations []models.InventoryReservation
	if err := core.CallInternalService(s.DB, 8103, "GET", fmt.Sprintf("/api/internal/inventory/reservations/%s", orderID), nil, &reservations); err == nil {
		detail.Reservations = reservations
	}

	return detail, nil
}

func EnrichOrdersItemSKUs(db *gorm.DB, orders []models.Order) {
	cache := make(map[uint]models.Sku)
	for i := range orders {
		enrichItemsWithCache(db, orders[i].Items, cache)
	}
}

func EnrichOrderItemSKUs(db *gorm.DB, items []models.OrderItem) {
	enrichItemsWithCache(db, items, make(map[uint]models.Sku))
}

func enrichItemsWithCache(db *gorm.DB, items []models.OrderItem, cache map[uint]models.Sku) {
	for i := range items {
		skuID := items[i].SkuID
		if sku, ok := cache[skuID]; ok {
			items[i].Sku = sku
			continue
		}
		var sku models.Sku
		if err := core.CallInternalService(db, 8102, "GET", fmt.Sprintf("/api/internal/products/%d", skuID), nil, &sku); err != nil {
			continue
		}
		cache[skuID] = sku
		items[i].Sku = sku
	}
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
