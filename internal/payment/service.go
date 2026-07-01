package payment

import (
	"encoding/json"
	"fmt"
	"time"

	"GoShop/core"
	"GoShop/internal/outbox"
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

type orderPaymentSource struct {
	OrderID      string     `json:"orderId"`
	UserID       uint       `json:"userId"`
	TotalAmount  int        `json:"totalAmount"`
	Status       int        `json:"status"`
	PayStatus    int        `json:"payStatus"`
	UserCouponID uint       `json:"userCouponId"`
	PayExpireAt  *time.Time `json:"payExpireAt,omitempty"`
}

type Service struct {
	DB *gorm.DB
}

func NewService(db *gorm.DB) Service {
	return Service{DB: db}
}

func PaymentOrderID(orderID string) string {
	return "PAY-" + orderID
}

func (s Service) CreateOrGetPaymentOrder(userID uint, orderID string) (CreatePaymentResult, error) {
	var result CreatePaymentResult
	err := s.DB.Transaction(func(tx *gorm.DB) error {
		source, err := loadOrderPaymentSource(tx, userID, orderID)
		if err != nil {
			return err
		}
		if source.Status != models.OrderStatusPendingPayment && source.Status != models.OrderStatusPaid {
			return fmt.Errorf("当前订单状态不可创建支付单")
		}

		payment, err := CreateMockPaymentOrderFromSource(tx, source)
		if err != nil {
			return err
		}
		result = CreatePaymentResult{
			PaymentOrderID: payment.ID,
			OrderID:        payment.OrderID,
			Amount:         payment.Amount,
			Status:         payment.Status,
			PayExpireAt:    source.PayExpireAt,
		}
		return nil
	})
	return result, err
}

func (s Service) GetPaymentOrder(userID uint, paymentOrderID string) (models.PaymentOrder, error) {
	var payment models.PaymentOrder
	err := s.DB.Where("id = ? AND user_id = ?", paymentOrderID, userID).First(&payment).Error
	return payment, err
}

func (s Service) PayMockOrder(userID uint, orderID string) (PayResult, error) {
	result := PayResult{OrderID: orderID, PaymentOrderID: PaymentOrderID(orderID)}
	paymentResult, err := s.CreateOrGetPaymentOrder(userID, orderID)
	if err != nil {
		return result, err
	}
	result.PaymentOrderID = paymentResult.PaymentOrderID

	if paymentResult.Status == models.PaymentStatusPaid {
		result.AlreadyPaid = true
		return result, nil
	}

	_, err = s.HandleMockCallback(MockCallbackRequest{
		PaymentOrderID: paymentResult.PaymentOrderID,
		OrderID:        orderID,
		Amount:         paymentResult.Amount,
		EventID:        "mock-pay:" + paymentResult.PaymentOrderID,
		ChannelTradeNo: "MOCK-" + orderID,
		Status:         "paid",
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

		source, err := loadOrderPaymentSource(tx, payment.UserID, payment.OrderID)
		if err != nil {
			return err
		}
		if source.Status != models.OrderStatusPendingPayment || source.PayStatus != models.PayStatusUnpaid {
			return nil
		}

		return s.markPaid(tx, source, payment, req.ChannelTradeNo, "PAYMENT_CALLBACK_PAID", "模拟支付回调入账")
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
	return CreateMockPaymentOrderFromSource(tx, orderPaymentSource{
		OrderID:      order.ID,
		UserID:       order.UserID,
		TotalAmount:  order.TotalAmount,
		Status:       order.Status,
		PayStatus:    order.PayStatus,
		UserCouponID: order.UserCouponID,
		PayExpireAt:  order.PayExpireAt,
	})
}

func CreateMockPaymentOrderFromSource(tx *gorm.DB, source orderPaymentSource) (models.PaymentOrder, error) {
	payment := models.PaymentOrder{
		ID:             PaymentOrderID(source.OrderID),
		OrderID:        source.OrderID,
		UserID:         source.UserID,
		Channel:        models.PaymentChannelMock,
		Amount:         source.TotalAmount,
		Currency:       "CNY",
		Status:         models.PaymentStatusCreated,
		IdempotencyKey: "mock:create:" + source.OrderID,
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

func (s Service) markPaid(tx *gorm.DB, source orderPaymentSource, payment models.PaymentOrder, channelTradeNo, event, remark string) error {
	now := time.Now()
	payment.Status = models.PaymentStatusPaid
	payment.ChannelTradeNo = channelTradeNo
	payment.PaidAt = &now
	payment.Version++
	if err := tx.Save(&payment).Error; err != nil {
		return err
	}

	if err := markPaidInSharedMonolithDB(tx, source, payment, event, remark); err != nil {
		return err
	}
	return outbox.NewService().Publish(tx, "payment", payment.ID, "PaymentSucceeded", map[string]interface{}{
		"paymentOrderId": payment.ID,
		"orderId":        source.OrderID,
		"userId":         source.UserID,
		"userCouponId":   source.UserCouponID,
		"amount":         payment.Amount,
		"channel":        payment.Channel,
		"channelTradeNo": payment.ChannelTradeNo,
	})
}

func loadOrderPaymentSource(tx *gorm.DB, userID uint, orderID string) (orderPaymentSource, error) {
	var source orderPaymentSource
	err := core.CallInternalService(
		tx,
		8105,
		"GET",
		fmt.Sprintf("/api/internal/orders/%s/payment-source?userId=%d", orderID, userID),
		nil,
		&source,
	)
	return source, err
}

func markPaidInSharedMonolithDB(tx *gorm.DB, source orderPaymentSource, payment models.PaymentOrder, event, remark string) error {
	if !tx.Migrator().HasTable(&models.Order{}) {
		return nil
	}

	var order models.Order
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&order, "id = ? AND user_id = ?", source.OrderID, source.UserID).Error; err != nil {
		return err
	}
	if order.Status != models.OrderStatusPendingPayment || order.PayStatus != models.PayStatusUnpaid {
		return nil
	}

	fromStatus := order.Status
	order.Status = models.OrderStatusPaid
	order.PayStatus = models.PayStatusPaid
	if err := tx.Save(&order).Error; err != nil {
		return err
	}

	if tx.Migrator().HasTable(&models.InventoryReservation{}) {
		if err := confirmSharedOrderReservations(tx, order.ID); err != nil {
			return err
		}
	}
	if tx.Migrator().HasTable(&models.UserCoupon{}) {
		if err := confirmSharedCoupon(tx, order.UserID, order.UserCouponID, order.ID); err != nil {
			return err
		}
	}
	if tx.Migrator().HasTable(&models.AccountingEntry{}) {
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
	}

	if tx.Migrator().HasTable(&models.OrderStateLog{}) {
		if err := tx.Create(&models.OrderStateLog{
			OrderID:      order.ID,
			FromStatus:   fromStatus,
			ToStatus:     models.OrderStatusPaid,
			OperatorType: 1,
			OperatorID:   order.UserID,
			Event:        event,
			Remark:       remark,
		}).Error; err != nil {
			return err
		}
	}
	return nil
}

func confirmSharedOrderReservations(tx *gorm.DB, orderID string) error {
	var reservations []models.InventoryReservation
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("order_id = ? AND status = ?", orderID, models.ReservationStatusReserved).
		Find(&reservations).Error; err != nil {
		return err
	}
	for _, reservation := range reservations {
		var inv models.SkuInventory
		if err := tx.Where("sku_id = ?", reservation.SkuID).First(&inv).Error; err != nil {
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
		journal := models.InventoryJournal{
			SkuID:         reservation.SkuID,
			OrderID:       orderID,
			ReservationID: reservation.ID,
			ChangeType:    "CONFIRM",
			Quantity:      reservation.Quantity,
		}
		if tx.Migrator().HasTable(&models.InventoryJournal{}) {
			if err := tx.Create(&journal).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func confirmSharedCoupon(tx *gorm.DB, userID, userCouponID uint, orderID string) error {
	if userCouponID == 0 {
		return nil
	}
	var userCoupon models.UserCoupon
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&userCoupon, "id = ? AND user_id = ?", userCouponID, userID).Error; err != nil {
		return err
	}
	if userCoupon.Status == models.UserCouponStatusUsed {
		return nil
	}
	if userCoupon.Status != models.UserCouponStatusLocked || userCoupon.LockedOrderID != orderID {
		return fmt.Errorf("优惠券未被当前订单锁定")
	}

	now := time.Now()
	return tx.Model(&models.UserCoupon{}).
		Where("id = ? AND user_id = ?", userCouponID, userID).
		Updates(map[string]interface{}{
			"status":          models.UserCouponStatusUsed,
			"used_at":         &now,
			"locked_order_id": "",
			"locked_at":       nil,
			"updated_at":      now,
		}).Error
}
