package aftersale

import (
	"fmt"
	"time"

	"GoShop/core"
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
	DB *gorm.DB
}

func NewService(db *gorm.DB) Service {
	return Service{DB: db}
}

type orderRefundSource struct {
	OrderID         string            `json:"orderId"`
	UserID          uint              `json:"userId"`
	TotalAmount     int               `json:"totalAmount"`
	Status          int               `json:"status"`
	PayStatus       int               `json:"payStatus"`
	AfterSaleStatus int               `json:"afterSaleStatus"`
	Items           []orderRefundItem `json:"items"`
}

type orderRefundItem struct {
	OrderItemID      uint `json:"orderItemId"`
	SkuID            uint `json:"skuId"`
	Quantity         int  `json:"quantity"`
	PayableAmount    int  `json:"payableAmount"`
	RefundedAmount   int  `json:"refundedAmount"`
	RefundableQty    int  `json:"refundableQuantity"`
	RefundableAmount int  `json:"refundableAmount"`
}

func (s Service) ApplyRefund(userID uint, orderID string, req ApplyRequest) error {
	return s.DB.Transaction(func(tx *gorm.DB) error {
		order, err := loadOrderRefundSource(tx, userID, orderID)
		if err != nil {
			return err
		}
		if order.Status != models.OrderStatusPaid || (order.PayStatus != models.PayStatusPaid && order.PayStatus != models.PayStatusPartialRefunded) {
			return fmt.Errorf("该订单当前状态不支持申请退款")
		}
		var applyingCount int64
		if err := tx.Model(&models.AfterSaleOrder{}).
			Where("order_id = ? AND status = ?", order.OrderID, models.AfterSaleStatusApplying).
			Count(&applyingCount).Error; err != nil {
			return err
		}
		if applyingCount > 0 {
			return fmt.Errorf("该订单已有售后申请处理中")
		}

		afterSaleID := fmt.Sprintf("AS-%s-%d", order.OrderID, time.Now().UnixNano()%1000000)
		afterSale := models.AfterSaleOrder{
			ID:             afterSaleID,
			OrderID:        order.OrderID,
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

		itemsByID := make(map[uint]orderRefundItem, len(order.Items))
		for _, item := range order.Items {
			itemsByID[item.OrderItemID] = item
		}

		fullOrderRefund := len(req.Items) == 0
		if fullOrderRefund {
			afterSale.ApplyAmount = remainingOrderRefundAmount(tx, order)
			if afterSale.ApplyAmount <= 0 {
				return fmt.Errorf("该订单已无可退金额")
			}
			for _, item := range order.Items {
				maxRefundable := item.RefundableAmount
				if maxRefundable < 0 {
					maxRefundable = 0
				}
				if maxRefundable == 0 {
					continue
				}
				afterSale.Items = append(afterSale.Items, models.AfterSaleItem{
					AfterSaleID:         afterSaleID,
					OrderItemID:         item.OrderItemID,
					SkuID:               item.SkuID,
					Quantity:            item.RefundableQty,
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
				maxRefundable := item.RefundableAmount
				if maxRefundable <= 0 {
					return fmt.Errorf("订单行 %d 已无可退金额", item.OrderItemID)
				}
				maxQuantity := item.RefundableQty
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
					OrderItemID:         item.OrderItemID,
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

		if err := markOrderRefundApplying(tx, order.OrderID, userID, req.RefundReason, req.RefundProof); err != nil {
			return err
		}
		return outbox.NewService().Publish(tx, "aftersale", afterSale.ID, "AfterSaleApplied", map[string]interface{}{
			"afterSaleId": afterSale.ID,
			"orderId":     order.OrderID,
			"userId":      userID,
			"applyAmount": afterSale.ApplyAmount,
		})
	})
}

func (s Service) AuditRefund(orderID string, req AuditRequest) error {
	return s.DB.Transaction(func(tx *gorm.DB) error {
		switch req.Action {
		case "approve":
			return s.approve(tx, orderID)
		case "reject":
			return s.reject(tx, orderID)
		default:
			return fmt.Errorf("无效的操作指令")
		}
	})
}

func (s Service) approve(tx *gorm.DB, orderID string) error {
	var afterSale models.AfterSaleOrder
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Preload("Items").Where("order_id = ? AND status = ?", orderID, models.AfterSaleStatusApplying).First(&afterSale).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			var done models.AfterSaleOrder
			if doneErr := tx.Where("order_id = ? AND status = ?", orderID, models.AfterSaleStatusRefunded).First(&done).Error; doneErr == nil {
				return nil
			}
			return nil
		}
		return err
	}

	order, err := loadOrderRefundSource(tx, afterSale.UserID, orderID)
	if err != nil {
		return err
	}

	now := time.Now()
	refund := models.RefundOrder{
		ID:              "REF-" + afterSale.ID,
		PaymentOrderID:  payment.PaymentOrderID(order.OrderID),
		OrderID:         order.OrderID,
		AfterSaleID:     afterSale.ID,
		Amount:          afterSale.ApplyAmount,
		Reason:          afterSale.Reason,
		Status:          models.RefundStatusSuccess,
		ChannelRefundNo: "MOCK-REF-" + afterSale.ID,
		IdempotencyKey:  "mock:refund:" + afterSale.ID,
		RefundedAt:      &now,
	}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&refund).Error; err != nil {
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

	restockItems := make([]map[string]interface{}, 0, len(afterSale.Items))
	completeItems := make([]map[string]interface{}, 0, len(afterSale.Items))
	for _, item := range afterSale.Items {
		restockItems = append(restockItems, map[string]interface{}{"skuId": item.SkuID, "quantity": item.Quantity})
		completeItems = append(completeItems, map[string]interface{}{"orderItemId": item.OrderItemID, "amount": item.ApplyAmount})
	}
	if err := restockInventoryItems(tx, order.OrderID, restockItems); err != nil {
		return err
	}
	if err := completeOrderRefund(tx, order.OrderID, completeItems, "商家审核通过并模拟退款"); err != nil {
		return err
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
	return outbox.NewService().Publish(tx, "refund", refund.ID, "RefundSucceeded", map[string]interface{}{
		"refundId":    refund.ID,
		"afterSaleId": afterSale.ID,
		"orderId":     order.OrderID,
		"amount":      refund.Amount,
	})
}

func (s Service) reject(tx *gorm.DB, orderID string) error {
	var afterSale models.AfterSaleOrder
	if err := tx.Where("order_id = ? AND status = ?", orderID, models.AfterSaleStatusApplying).First(&afterSale).Error; err == nil {
		afterSale.Status = models.AfterSaleStatusRejected
		if err := tx.Save(&afterSale).Error; err != nil {
			return err
		}
	} else if err != gorm.ErrRecordNotFound {
		return err
	}
	if err := rejectOrderRefund(tx, orderID); err != nil {
		return err
	}
	return outbox.NewService().Publish(tx, "aftersale", afterSale.ID, "AfterSaleRejected", map[string]interface{}{
		"afterSaleId": afterSale.ID,
		"orderId":     orderID,
	})
}

func remainingOrderRefundAmount(tx *gorm.DB, order orderRefundSource) int {
	totalRefunded := sumRefundedAmount(tx, order.OrderID)
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

func loadOrderRefundSource(tx *gorm.DB, userID uint, orderID string) (orderRefundSource, error) {
	var source orderRefundSource
	err := core.CallInternalService(tx, 8105, "GET", fmt.Sprintf("/api/internal/orders/%s/refund-source?userId=%d", orderID, userID), nil, &source)
	return source, err
}

func markOrderRefundApplying(tx *gorm.DB, orderID string, userID uint, reason, proof string) error {
	return core.CallInternalService(tx, 8105, "POST", fmt.Sprintf("/api/internal/orders/%s/refund-apply", orderID), map[string]interface{}{
		"userId": userID,
		"reason": reason,
		"proof":  proof,
	}, nil)
}

func completeOrderRefund(tx *gorm.DB, orderID string, items []map[string]interface{}, remark string) error {
	return core.CallInternalService(tx, 8105, "POST", fmt.Sprintf("/api/internal/orders/%s/refund-complete", orderID), map[string]interface{}{
		"items":  items,
		"remark": remark,
	}, nil)
}

func rejectOrderRefund(tx *gorm.DB, orderID string) error {
	return core.CallInternalService(tx, 8105, "POST", fmt.Sprintf("/api/internal/orders/%s/refund-reject", orderID), nil, nil)
}

func restockInventoryItems(tx *gorm.DB, orderID string, items []map[string]interface{}) error {
	return core.CallInternalService(tx, 8103, "POST", "/api/internal/inventory/restock", map[string]interface{}{
		"orderId": orderID,
		"items":   items,
	}, nil)
}
