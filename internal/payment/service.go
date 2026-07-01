package payment

import (
	"encoding/json"
	"fmt"
	"time"

	"GoShop/internal/inventory"
	"GoShop/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type MockCallbackRequest struct {
	PaymentOrderID string `json:"paymentOrderId"`
	OrderID        string `json:"orderId"`
	Amount         int    `json:"amount" binding:"required"`
	EventID        string `json:"eventId"`
	ChannelTradeNo string `json:"channelTradeNo"`
	Status         string `json:"status"`
}

type PayResult struct {
	PaymentOrderID string
	AlreadyPaid    bool
	OrderID        string
}

type CreatePaymentResult struct {
	PaymentOrderID string     `json:"paymentOrderId"`
	OrderID        string     `json:"orderId"`
	Amount         int        `json:"amount"`
	Status         int        `json:"status"`
	PayExpireAt    *time.Time `json:"payExpireAt,omitempty"`
}

type Service struct {
	DB        *gorm.DB
	Inventory inventory.Service
}

func NewService(db *gorm.DB) Service {
	return Service{DB: db, Inventory: inventory.NewService(db)}
}

func PaymentOrderID(orderID string) string {
	return "PAY-" + orderID
}

func (s Service) CreateOrGetPaymentOrder(userID uint, orderID string) (CreatePaymentResult, error) {
	var result CreatePaymentResult
	err := s.DB.Transaction(func(tx *gorm.DB) error {
		var order models.Order
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&order, "id = ? AND user_id = ?", orderID, userID).Error; err != nil {
			return err
		}
		if order.Status != models.OrderStatusPendingPayment && order.Status != models.OrderStatusPaid {
			return fmt.Errorf("当前订单状态不可创建支付单")
		}

		payment, err := CreateMockPaymentOrder(tx, order)
		if err != nil {
			return err
		}
		result = CreatePaymentResult{
			PaymentOrderID: payment.ID,
			OrderID:        payment.OrderID,
			Amount:         payment.Amount,
			Status:         payment.Status,
			PayExpireAt:    order.PayExpireAt,
		}
		return nil
	})
	return result, err
}

func (s Service) GetPaymentOrder(userID uint, paymentOrderID string) (models.PaymentOrder, error) {
	var payment models.PaymentOrder
	err := s.DB.Joins("JOIN orders ON orders.id = payment_orders.order_id").
		Where("payment_orders.id = ? AND orders.user_id = ?", paymentOrderID, userID).
		First(&payment).Error
	return payment, err
}

func (s Service) PayMockOrder(userID uint, orderID string) (PayResult, error) {
	var result PayResult
	err := s.DB.Transaction(func(tx *gorm.DB) error {
		var order models.Order
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&order, "id = ? AND user_id = ?", orderID, userID).Error; err != nil {
			return err
		}
		result.OrderID = order.ID
		result.PaymentOrderID = PaymentOrderID(order.ID)

		if order.Status == models.OrderStatusPaid && order.PayStatus == models.PayStatusPaid {
			result.AlreadyPaid = true
			return nil
		}
		if order.Status != models.OrderStatusPendingPayment || order.PayStatus != models.PayStatusUnpaid {
			return fmt.Errorf("当前订单状态不可支付")
		}

		payment, err := CreateMockPaymentOrder(tx, order)
		if err != nil {
			return err
		}
		result.PaymentOrderID = payment.ID

		eventID := "mock-pay:" + payment.ID
		transaction := models.PaymentTransaction{
			PaymentOrderID: payment.ID,
			Channel:        models.PaymentChannelMock,
			ChannelEventID: eventID,
			EventType:      "mock.payment.succeeded",
			RawPayload:     fmt.Sprintf(`{"order_id":"%s","amount":%d}`, order.ID, payment.Amount),
			ProcessStatus:  models.TransactionStatusProcessed,
		}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&transaction).Error; err != nil {
			return err
		}

		return s.markPaid(tx, order, payment, "MOCK-"+order.ID, "ORDER_PAID", "模拟支付成功并入账")
	})
	return result, err
}

func (s Service) HandleMockCallback(req MockCallbackRequest) (string, error) {
	if req.PaymentOrderID == "" && req.OrderID == "" {
		return "", fmt.Errorf("paymentOrderId 和 orderId 至少传一个")
	}
	if req.PaymentOrderID == "" {
		req.PaymentOrderID = PaymentOrderID(req.OrderID)
	}
	if req.EventID == "" {
		req.EventID = "mock-callback:" + req.PaymentOrderID
	}
	if req.ChannelTradeNo == "" {
		req.ChannelTradeNo = "MOCK-CB-" + req.PaymentOrderID
	}
	if req.Status == "" {
		req.Status = "paid"
	}

	raw, _ := json.Marshal(req)
	var orderID string
	var callbackErr error
	err := s.DB.Transaction(func(tx *gorm.DB) error {
		var existingTx models.PaymentTransaction
		if err := tx.Where("channel = ? AND channel_event_id = ?", models.PaymentChannelMock, req.EventID).First(&existingTx).Error; err == nil {
			var payment models.PaymentOrder
			if err := tx.Select("order_id").First(&payment, "id = ?", req.PaymentOrderID).Error; err == nil {
				orderID = payment.OrderID
			}
			return nil
		} else if err != gorm.ErrRecordNotFound {
			return err
		}

		var payment models.PaymentOrder
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&payment, "id = ?", req.PaymentOrderID).Error; err != nil {
			return err
		}
		orderID = payment.OrderID

		processStatus := models.TransactionStatusProcessed
		errorMessage := ""
		if req.Amount != payment.Amount {
			processStatus = models.TransactionStatusFailed
			errorMessage = "callback amount mismatch"
		}
		transaction := models.PaymentTransaction{
			PaymentOrderID: payment.ID,
			Channel:        models.PaymentChannelMock,
			ChannelEventID: req.EventID,
			EventType:      "mock.payment.callback",
			RawPayload:     string(raw),
			ProcessStatus:  processStatus,
			ErrorMessage:   errorMessage,
		}
		if err := tx.Create(&transaction).Error; err != nil {
			return err
		}
		if processStatus == models.TransactionStatusFailed {
			callbackErr = fmt.Errorf("%s", errorMessage)
			return nil
		}
		if req.Status != "paid" {
			return fmt.Errorf("unsupported mock callback status: %s", req.Status)
		}
		if payment.Status == models.PaymentStatusPaid {
			return nil
		}

		var order models.Order
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&order, "id = ? AND user_id = ?", payment.OrderID, payment.UserID).Error; err != nil {
			return err
		}
		if order.Status != models.OrderStatusPendingPayment || order.PayStatus != models.PayStatusUnpaid {
			return nil
		}

		return s.markPaid(tx, order, payment, req.ChannelTradeNo, "PAYMENT_CALLBACK_PAID", "模拟支付回调入账")
	})
	if err != nil {
		return orderID, err
	}
	if callbackErr != nil {
		return orderID, callbackErr
	}
	return orderID, nil
}

func CreateMockPaymentOrder(tx *gorm.DB, order models.Order) (models.PaymentOrder, error) {
	payment := models.PaymentOrder{
		ID:             PaymentOrderID(order.ID),
		OrderID:        order.ID,
		UserID:         order.UserID,
		Channel:        models.PaymentChannelMock,
		Amount:         order.TotalAmount,
		Currency:       "CNY",
		Status:         models.PaymentStatusCreated,
		IdempotencyKey: "mock:create:" + order.ID,
	}

	var existing models.PaymentOrder
	err := tx.Where("id = ?", payment.ID).First(&existing).Error
	if err == nil {
		return existing, nil
	}
	if err != gorm.ErrRecordNotFound {
		return payment, err
	}
	return payment, tx.Create(&payment).Error
}

func (s Service) markPaid(tx *gorm.DB, order models.Order, payment models.PaymentOrder, channelTradeNo, event, remark string) error {
	now := time.Now()
	payment.Status = models.PaymentStatusPaid
	payment.ChannelTradeNo = channelTradeNo
	payment.PaidAt = &now
	payment.Version++
	if err := tx.Save(&payment).Error; err != nil {
		return err
	}

	fromStatus := order.Status
	order.Status = models.OrderStatusPaid
	order.PayStatus = models.PayStatusPaid
	if err := tx.Save(&order).Error; err != nil {
		return err
	}
	if err := s.Inventory.ConfirmOrderReservations(tx, order.ID); err != nil {
		return err
	}

	entries := []models.AccountingEntry{
		{
			BizType:     "payment",
			BizID:       payment.ID,
			AccountType: "cash",
			Direction:   models.AccountingDirectionDebit,
			Amount:      payment.Amount,
			Currency:    payment.Currency,
		},
		{
			BizType:     "payment",
			BizID:       payment.ID,
			AccountType: "sales_revenue",
			Direction:   models.AccountingDirectionCredit,
			Amount:      payment.Amount,
			Currency:    payment.Currency,
		},
	}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&entries).Error; err != nil {
		return err
	}

	return tx.Create(&models.OrderStateLog{
		OrderID:      order.ID,
		FromStatus:   fromStatus,
		ToStatus:     models.OrderStatusPaid,
		OperatorType: 1,
		OperatorID:   order.UserID,
		Event:        event,
		Remark:       remark,
	}).Error
}
