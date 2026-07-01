package handlers

import (
	"encoding/json"
	"fmt"

	"GoShop/core"
	"GoShop/internal/promotion"
	"GoShop/models"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

type NATSOutboxEvent struct {
	EventID       string `json:"event_id"`
	AggregateType string `json:"aggregate_type"`
	AggregateID   string `json:"aggregate_id"`
	EventType     string `json:"event_type"`
	Payload       string `json:"payload"`
}

type PaymentSucceededPayload struct {
	OrderID        string `json:"orderId"`
	UserID         uint   `json:"userId"`
	UserCouponID   uint   `json:"userCouponId"`
	PaymentOrderID string `json:"paymentOrderId"`
	Amount         int    `json:"amount"`
	Channel        string `json:"channel"`
	ChannelTradeNo string `json:"channelTradeNo"`
}

// RegisterOrderServiceSubscribers 注册订单微服务的 NATS 消费者
func RegisterOrderServiceSubscribers() {
	if core.NATSConn == nil {
		return
	}

	subject := "goshop.events.payment.paymentsucceeded"
	queueGroup := "goshop-order-service-group"
	consumerName := "order-payment-handler"

	_, err := core.RegisterSubscriber(subject, queueGroup, consumerName, func(msgData []byte) error {
		var outer NATSOutboxEvent
		if err := json.Unmarshal(msgData, &outer); err != nil {
			return err
		}

		var payload PaymentSucceededPayload
		if err := json.Unmarshal([]byte(outer.Payload), &payload); err != nil {
			return err
		}

		core.Logger.Info("订单服务收到支付成功事件，开始幂等落库", zap.String("order_id", payload.OrderID))

		// 开启订单本地事务，由 Inbox 幂等去重
		return core.DB.Transaction(func(tx *gorm.DB) error {
			return core.ProcessWithInbox(tx, outer.EventID, consumerName, func(dbTx *gorm.DB) error {
				var order models.Order
				if err := dbTx.Where("id = ?", payload.OrderID).First(&order).Error; err != nil {
					return err
				}

				fromStatus := order.Status
				order.Status = models.OrderStatusPaid
				order.PayStatus = models.PayStatusPaid
				if err := dbTx.Save(&order).Error; err != nil {
					return err
				}

				return dbTx.Create(&models.OrderStateLog{
					OrderID:      order.ID,
					FromStatus:   fromStatus,
					ToStatus:     models.OrderStatusPaid,
					OperatorType: 1, // System/User
					OperatorID:   order.UserID,
					Event:        "PaymentSucceeded",
					Remark:       fmt.Sprintf("订单通过 NATS MQ 异步确认支付成功，流水号: %s", payload.ChannelTradeNo),
				}).Error
			})
		})
	})

	if err != nil {
		core.Logger.Error("订单服务注册 NATS 订阅失败", zap.Error(err))
	} else {
		core.Logger.Info("订单服务成功注册 NATS 支付事件订阅")
	}
}

// RegisterInventoryServiceSubscribers 注册库存微服务的 NATS 消费者
func RegisterInventoryServiceSubscribers() {
	if core.NATSConn == nil {
		return
	}

	subject := "goshop.events.payment.paymentsucceeded"
	queueGroup := "goshop-inventory-service-group"
	consumerName := "inventory-payment-handler"

	_, err := core.RegisterSubscriber(subject, queueGroup, consumerName, func(msgData []byte) error {
		var outer NATSOutboxEvent
		if err := json.Unmarshal(msgData, &outer); err != nil {
			return err
		}

		var payload PaymentSucceededPayload
		if err := json.Unmarshal([]byte(outer.Payload), &payload); err != nil {
			return err
		}

		core.Logger.Info("库存服务收到支付成功事件，确认预占库存", zap.String("order_id", payload.OrderID))

		return core.DB.Transaction(func(tx *gorm.DB) error {
			return core.ProcessWithInbox(tx, outer.EventID, consumerName, func(dbTx *gorm.DB) error {
				var reservations []models.InventoryReservation
				if err := dbTx.Where("order_id = ? AND status = ?", payload.OrderID, models.ReservationStatusReserved).Find(&reservations).Error; err != nil {
					return err
				}

				for _, res := range reservations {
					res.Status = models.ReservationStatusConfirmed
					if err := dbTx.Save(&res).Error; err != nil {
						return err
					}

					// 记录库存变动明细日志
					journal := models.InventoryJournal{
						SkuID:         res.SkuID,
						OrderID:       res.OrderID,
						ReservationID: res.ID,
						ChangeType:    "confirm",
						Quantity:      -res.Quantity,
					}
					if err := dbTx.Create(&journal).Error; err != nil {
						return err
					}
				}
				return nil
			})
		})
	})

	if err != nil {
		core.Logger.Error("库存服务注册 NATS 订阅失败", zap.Error(err))
	} else {
		core.Logger.Info("库存服务成功注册 NATS 支付事件订阅")
	}
}

// RegisterPromotionServiceSubscribers 注册营销/优惠券微服务的 NATS 消费者
func RegisterPromotionServiceSubscribers() {
	if core.NATSConn == nil {
		return
	}

	subject := "goshop.events.payment.paymentsucceeded"
	queueGroup := "goshop-promotion-service-group"
	consumerName := "promotion-payment-handler"

	_, err := core.RegisterSubscriber(subject, queueGroup, consumerName, func(msgData []byte) error {
		var outer NATSOutboxEvent
		if err := json.Unmarshal(msgData, &outer); err != nil {
			return err
		}

		var payload PaymentSucceededPayload
		if err := json.Unmarshal([]byte(outer.Payload), &payload); err != nil {
			return err
		}

		core.Logger.Info("营销服务收到支付成功事件，核销优惠券", zap.String("order_id", payload.OrderID))

		return core.DB.Transaction(func(tx *gorm.DB) error {
			return core.ProcessWithInbox(tx, outer.EventID, consumerName, func(dbTx *gorm.DB) error {
				if payload.UserCouponID == 0 {
					return nil // 没有使用优惠券，幂等退出
				}

				// 将优惠券状态置为已核销
				return promotion.NewService(dbTx).ConfirmCouponUsed(dbTx, payload.UserID, payload.UserCouponID, payload.OrderID)
			})
		})
	})

	if err != nil {
		core.Logger.Error("营销服务注册 NATS 订阅失败", zap.Error(err))
	} else {
		core.Logger.Info("营销服务成功注册 NATS 支付事件订阅")
	}
}
