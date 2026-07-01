package aftersale

import (
	"fmt"
	"time"

	"GoShop/internal/inventory"
	"GoShop/internal/payment"
	"GoShop/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ApplyRequest struct {
	RefundReason string
	RefundProof  string
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
		if order.Status != models.OrderStatusPaid || order.PayStatus != models.PayStatusPaid {
			return fmt.Errorf("该订单当前状态不支持申请退款")
		}

		afterSaleID := fmt.Sprintf("AS-%s", order.ID)
		afterSale := models.AfterSaleOrder{
			ID:             afterSaleID,
			OrderID:        order.ID,
			UserID:         userID,
			Type:           1,
			Status:         models.AfterSaleStatusApplying,
			Reason:         req.RefundReason,
			ProofURLs:      req.RefundProof,
			ApplyAmount:    order.TotalAmount,
			ApprovedAmount: 0,
		}
		for _, item := range order.Items {
			maxRefundable := item.PayableAmount - item.RefundedAmount
			if maxRefundable < 0 {
				maxRefundable = 0
			}
			afterSale.Items = append(afterSale.Items, models.AfterSaleItem{
				AfterSaleID:         afterSaleID,
				OrderItemID:         item.ID,
				SkuID:               item.SkuID,
				Quantity:            item.Quantity,
				MaxRefundableAmount: maxRefundable,
				ApplyAmount:         maxRefundable,
			})
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
		return appendStateLog(tx, order.ID, fromStatus, models.OrderStatusRefundApplying, userID, "AFTERSALE_APPLIED", req.RefundReason)
	})
}

func (s Service) AuditRefund(orderID string, req AuditRequest) error {
	return s.DB.Transaction(func(tx *gorm.DB) error {
		var order models.Order
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Preload("Items").First(&order, "id = ?", orderID).Error; err != nil {
			return err
		}
		if order.Status != models.OrderStatusRefundApplying {
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
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("order_id = ? AND status = ?", order.ID, models.AfterSaleStatusApplying).First(&afterSale).Error; err != nil && err != gorm.ErrRecordNotFound {
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
		ID:              "REF-" + order.ID,
		PaymentOrderID:  payOrder.ID,
		OrderID:         order.ID,
		AfterSaleID:     afterSale.ID,
		Amount:          order.TotalAmount,
		Reason:          order.RefundReason,
		Status:          models.RefundStatusSuccess,
		ChannelRefundNo: "MOCK-REF-" + order.ID,
		IdempotencyKey:  "mock:refund:" + order.ID,
		RefundedAt:      &now,
	}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&refund).Error; err != nil {
		return err
	}

	fromStatus := order.Status
	order.Status = models.OrderStatusRefunded
	order.PayStatus = models.PayStatusRefunded
	order.AfterSaleStatus = models.AfterSaleStatusRefunded
	if err := tx.Save(&order).Error; err != nil {
		return err
	}

	if afterSale.ID != "" {
		afterSale.Status = models.AfterSaleStatusRefunded
		afterSale.ApprovedAmount = order.TotalAmount
		afterSale.RefundID = refund.ID
		if err := tx.Save(&afterSale).Error; err != nil {
			return err
		}
	}

	if err := s.Inventory.RestockSoldForOrder(tx, order.ID); err != nil {
		return err
	}
	for _, item := range order.Items {
		if err := tx.Model(&models.OrderItem{}).Where("id = ?", item.ID).Update("refunded_amount", item.PayableAmount).Error; err != nil {
			return err
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
	return appendStateLog(tx, order.ID, fromStatus, models.OrderStatusRefunded, 0, "AFTERSALE_APPROVED", "商家审核通过并模拟退款")
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
	return appendStateLog(tx, order.ID, fromStatus, models.OrderStatusRefundRejected, 0, "AFTERSALE_REJECTED", "商家拒绝退款申请")
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
