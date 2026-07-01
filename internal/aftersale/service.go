package aftersale

import (
	"fmt"
	"time"

	"GoShop/internal/inventory"
	"GoShop/internal/outbox"
	"GoShop/internal/payment"
	"GoShop/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ApplyRequest struct {
	Type         int
	RefundReason string
	RefundProof  string
	Items        []ApplyItem
}

type ApplyItem struct {
	OrderItemID uint
	Quantity    int
}

type AuditRequest struct {
	Action string
}

type Service struct {
	DB        *gorm.DB
	Inventory inventory.Service
}

func NewService(db *gorm.DB) Service {
	return Service{DB: db, Inventory: inventory.NewService(db)}
}

func (s Service) ApplyRefund(userID uint, orderID string, req ApplyRequest) error {
	return s.DB.Transaction(func(tx *gorm.DB) error {
		var order models.Order
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Preload("Items").Where("id = ? AND user_id = ?", orderID, userID).First(&order).Error; err != nil {
			return err
		}
		if order.Status != models.OrderStatusPaid || (order.PayStatus != models.PayStatusPaid && order.PayStatus != models.PayStatusPartialRefunded) {
			return fmt.Errorf("该订单当前状态不支持申请退款")
		}
		var applyingCount int64
		if err := tx.Model(&models.AfterSaleOrder{}).
			Where("order_id = ? AND status = ?", order.ID, models.AfterSaleStatusApplying).
			Count(&applyingCount).Error; err != nil {
			return err
		}
		if applyingCount > 0 {
			return fmt.Errorf("该订单已有售后申请处理中")
		}

		afterSaleID := fmt.Sprintf("AS-%s-%d", order.ID, time.Now().UnixNano()%1000000)
		afterSale := models.AfterSaleOrder{
			ID:             afterSaleID,
			OrderID:        order.ID,
			UserID:         userID,
			Type:           req.Type,
			Status:         models.AfterSaleStatusApplying,
			Reason:         req.RefundReason,
			ProofURLs:      req.RefundProof,
			ApprovedAmount: 0,
		}
		if afterSale.Type == 0 {
			afterSale.Type = 1
		}

		itemsByID := make(map[uint]models.OrderItem, len(order.Items))
		for _, item := range order.Items {
			itemsByID[item.ID] = item
		}

		fullOrderRefund := len(req.Items) == 0
		if fullOrderRefund {
			afterSale.ApplyAmount = remainingOrderRefundAmount(tx, order)
			if afterSale.ApplyAmount <= 0 {
				return fmt.Errorf("该订单已无可退金额")
			}
			for _, item := range order.Items {
				maxRefundable := item.PayableAmount - item.RefundedAmount
				if maxRefundable < 0 {
					maxRefundable = 0
				}
				if maxRefundable == 0 {
					continue
				}
				afterSale.Items = append(afterSale.Items, models.AfterSaleItem{
					AfterSaleID:         afterSaleID,
					OrderItemID:         item.ID,
					SkuID:               item.SkuID,
					Quantity:            refundableQuantity(item),
					MaxRefundableAmount: maxRefundable,
					ApplyAmount:         maxRefundable,
				})
			}
		} else {
			for _, reqItem := range req.Items {
				item, ok := itemsByID[reqItem.OrderItemID]
				if !ok {
					return fmt.Errorf("退款商品不属于该订单")
				}
				maxRefundable := item.PayableAmount - item.RefundedAmount
				if maxRefundable <= 0 {
					return fmt.Errorf("订单行 %d 已无可退金额", item.ID)
				}
				maxQuantity := refundableQuantity(item)
				if reqItem.Quantity <= 0 || reqItem.Quantity > maxQuantity {
					return fmt.Errorf("退款数量不合法")
				}
				applyAmount := item.PayableAmount * reqItem.Quantity / item.Quantity
				if applyAmount <= 0 {
					applyAmount = maxRefundable
				}
				if applyAmount > maxRefundable {
					applyAmount = maxRefundable
				}
				afterSale.ApplyAmount += applyAmount
				afterSale.Items = append(afterSale.Items, models.AfterSaleItem{
					AfterSaleID:         afterSaleID,
					OrderItemID:         item.ID,
					SkuID:               item.SkuID,
					Quantity:            reqItem.Quantity,
					MaxRefundableAmount: maxRefundable,
					ApplyAmount:         applyAmount,
				})
			}
		}
		if len(afterSale.Items) == 0 || afterSale.ApplyAmount <= 0 {
			return fmt.Errorf("该订单已无可退商品")
		}
		if err := tx.Create(&afterSale).Error; err != nil {
			return err
		}

		fromStatus := order.Status
		order.Status = models.OrderStatusRefundApplying
		order.AfterSaleStatus = models.AfterSaleStatusApplying
		order.RefundReason = req.RefundReason
		order.RefundProof = req.RefundProof
		if err := tx.Save(&order).Error; err != nil {
			return err
		}
		if err := appendStateLog(tx, order.ID, fromStatus, models.OrderStatusRefundApplying, userID, "AFTERSALE_APPLIED", req.RefundReason); err != nil {
			return err
		}
		return outbox.NewService().Publish(tx, "aftersale", afterSale.ID, "AfterSaleApplied", map[string]interface{}{
			"afterSaleId": afterSale.ID,
			"orderId":     order.ID,
			"userId":      userID,
			"applyAmount": afterSale.ApplyAmount,
		})
	})
}

func (s Service) AuditRefund(orderID string, req AuditRequest) error {
	return s.DB.Transaction(func(tx *gorm.DB) error {
		var order models.Order
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Preload("Items").First(&order, "id = ?", orderID).Error; err != nil {
			return err
		}
		if order.Status != models.OrderStatusRefundApplying {
			if req.Action == "approve" && order.AfterSaleStatus == models.AfterSaleStatusRefunded {
				return nil
			}
			return fmt.Errorf("订单状态非退款申请中")
		}

		switch req.Action {
		case "approve":
			return s.approve(tx, order)
		case "reject":
			return s.reject(tx, order)
		default:
			return fmt.Errorf("无效的操作指令")
		}
	})
}

func (s Service) approve(tx *gorm.DB, order models.Order) error {
	var afterSale models.AfterSaleOrder
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Preload("Items").Where("order_id = ? AND status = ?", order.ID, models.AfterSaleStatusApplying).First(&afterSale).Error; err != nil {
		if err == gorm.ErrRecordNotFound && (order.Status == models.OrderStatusRefunded || order.PayStatus == models.PayStatusRefunded) {
			return nil
		}
		return err
	}

	var payOrder models.PaymentOrder
	if err := tx.Where("order_id = ? AND status = ?", order.ID, models.PaymentStatusPaid).First(&payOrder).Error; err != nil {
		payOrder = models.PaymentOrder{
			ID:             payment.PaymentOrderID(order.ID),
			OrderID:        order.ID,
			UserID:         order.UserID,
			Channel:        models.PaymentChannelMock,
			Amount:         order.TotalAmount,
			Currency:       "CNY",
			Status:         models.PaymentStatusPaid,
			ChannelTradeNo: "MOCK-" + order.ID,
			IdempotencyKey: "mock:create:" + order.ID,
		}
		now := time.Now()
		payOrder.PaidAt = &now
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&payOrder).Error; err != nil {
			return err
		}
	}

	now := time.Now()
	refund := models.RefundOrder{
		ID:              "REF-" + afterSale.ID,
		PaymentOrderID:  payOrder.ID,
		OrderID:         order.ID,
		AfterSaleID:     afterSale.ID,
		Amount:          afterSale.ApplyAmount,
		Reason:          order.RefundReason,
		Status:          models.RefundStatusSuccess,
		ChannelRefundNo: "MOCK-REF-" + afterSale.ID,
		IdempotencyKey:  "mock:refund:" + afterSale.ID,
		RefundedAt:      &now,
	}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&refund).Error; err != nil {
		return err
	}

	fromStatus := order.Status
	totalRefunded := sumRefundedAmount(tx, order.ID)
	if totalRefunded >= order.TotalAmount {
		order.Status = models.OrderStatusRefunded
		order.PayStatus = models.PayStatusRefunded
	} else {
		order.Status = models.OrderStatusPaid
		order.PayStatus = models.PayStatusPartialRefunded
	}
	order.AfterSaleStatus = models.AfterSaleStatusRefunded
	if err := tx.Save(&order).Error; err != nil {
		return err
	}

	afterSale.Status = models.AfterSaleStatusRefunded
	afterSale.ApprovedAmount = refund.Amount
	afterSale.RefundID = refund.ID
	if err := tx.Save(&afterSale).Error; err != nil {
		return err
	}
	for _, item := range afterSale.Items {
		if err := tx.Model(&models.AfterSaleItem{}).Where("id = ?", item.ID).Update("approved_amount", item.ApplyAmount).Error; err != nil {
			return err
		}
	}

	restockItems := make([]inventory.RestockItem, 0, len(afterSale.Items))
	for _, item := range afterSale.Items {
		restockItems = append(restockItems, inventory.RestockItem{SkuID: item.SkuID, Quantity: item.Quantity})
	}
	if err := s.Inventory.RestockItemsForOrder(tx, order.ID, restockItems); err != nil {
		return err
	}
	for _, item := range afterSale.Items {
		result := tx.Model(&models.OrderItem{}).
			Where("id = ? AND refunded_amount + ? <= payable_amount", item.OrderItemID, item.ApplyAmount).
			Update("refunded_amount", gorm.Expr("refunded_amount + ?", item.ApplyAmount))
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return fmt.Errorf("订单行 %d 可退金额不足", item.OrderItemID)
		}
	}

	entries := []models.AccountingEntry{
		{
			BizType:     "refund",
			BizID:       refund.ID,
			AccountType: "sales_refund",
			Direction:   models.AccountingDirectionDebit,
			Amount:      refund.Amount,
			Currency:    "CNY",
		},
		{
			BizType:     "refund",
			BizID:       refund.ID,
			AccountType: "cash",
			Direction:   models.AccountingDirectionCredit,
			Amount:      refund.Amount,
			Currency:    "CNY",
		},
	}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&entries).Error; err != nil {
		return err
	}
	if err := appendStateLog(tx, order.ID, fromStatus, order.Status, 0, "AFTERSALE_APPROVED", "商家审核通过并模拟退款"); err != nil {
		return err
	}
	return outbox.NewService().Publish(tx, "refund", refund.ID, "RefundSucceeded", map[string]interface{}{
		"refundId":    refund.ID,
		"afterSaleId": afterSale.ID,
		"orderId":     order.ID,
		"amount":      refund.Amount,
	})
}

func (s Service) reject(tx *gorm.DB, order models.Order) error {
	var afterSale models.AfterSaleOrder
	if err := tx.Where("order_id = ? AND status = ?", order.ID, models.AfterSaleStatusApplying).First(&afterSale).Error; err == nil {
		afterSale.Status = models.AfterSaleStatusRejected
		if err := tx.Save(&afterSale).Error; err != nil {
			return err
		}
	}
	fromStatus := order.Status
	order.Status = models.OrderStatusRefundRejected
	order.AfterSaleStatus = models.AfterSaleStatusRejected
	if err := tx.Save(&order).Error; err != nil {
		return err
	}
	if err := appendStateLog(tx, order.ID, fromStatus, models.OrderStatusRefundRejected, 0, "AFTERSALE_REJECTED", "商家拒绝退款申请"); err != nil {
		return err
	}
	return outbox.NewService().Publish(tx, "aftersale", afterSale.ID, "AfterSaleRejected", map[string]interface{}{
		"afterSaleId": afterSale.ID,
		"orderId":     order.ID,
	})
}

func remainingOrderRefundAmount(tx *gorm.DB, order models.Order) int {
	totalRefunded := sumRefundedAmount(tx, order.ID)
	remain := order.TotalAmount - totalRefunded
	if remain < 0 {
		return 0
	}
	return remain
}

func sumRefundedAmount(tx *gorm.DB, orderID string) int {
	var total int64
	_ = tx.Model(&models.RefundOrder{}).
		Where("order_id = ? AND status = ?", orderID, models.RefundStatusSuccess).
		Select("COALESCE(SUM(amount), 0)").
		Scan(&total).Error
	return int(total)
}

func refundableQuantity(item models.OrderItem) int {
	if item.Quantity <= 0 || item.PayableAmount <= 0 {
		return 0
	}
	remaining := item.PayableAmount - item.RefundedAmount
	if remaining <= 0 {
		return 0
	}
	qty := remaining * item.Quantity / item.PayableAmount
	if qty <= 0 {
		return 1
	}
	if qty > item.Quantity {
		return item.Quantity
	}
	return qty
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
