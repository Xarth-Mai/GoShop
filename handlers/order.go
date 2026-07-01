package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"GoShop/core"
	"GoShop/models"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type OrderItemReq struct {
	SkuID    uint `json:"skuId" binding:"required"`
	Quantity int  `json:"quantity" binding:"required"`
}

type CreateOrderReq struct {
	Items        []OrderItemReq `json:"items" binding:"required,dive"`
	AddressID    uint           `json:"addressId" binding:"required"`
	UserCouponID uint           `json:"userCouponId"` // 可选
}

type PayReq struct {
	OrderID string `json:"orderId" binding:"required"`
}

type MockPaymentCallbackReq struct {
	PaymentOrderID string `json:"paymentOrderId"`
	OrderID        string `json:"orderId"`
	Amount         int    `json:"amount" binding:"required"`
	EventID        string `json:"eventId"`
	ChannelTradeNo string `json:"channelTradeNo"`
	Status         string `json:"status"`
}

type RefundReq struct {
	RefundReason string `json:"refundReason" binding:"required"`
	RefundProof  string `json:"refundProof"`
}

type AuditRefundReq struct {
	Action string `json:"action" binding:"required"` // "approve" 或 "reject"
}

// LogItem 用于秒杀监控面板日志
type LogItem struct {
	Time string `json:"time"`
	Type string `json:"type"`
	Msg  string `json:"msg"`
}

func pushSystemLog(ctx context.Context, logType, msg string) {
	if core.RedisClient == nil {
		return
	}
	now := time.Now().Format("15:04:05")
	item := LogItem{Time: now, Type: logType, Msg: msg}
	bytes, _ := json.Marshal(item)
	core.RedisClient.LPush(ctx, "seckill:logs", string(bytes))
	core.RedisClient.LTrim(ctx, "seckill:logs", 0, 9)
}

func allocateDiscountByAmount(items []models.OrderItem, discount int) []models.OrderItem {
	if discount <= 0 || len(items) == 0 {
		for i := range items {
			items[i].ItemDiscountAmount = 0
			items[i].PayableAmount = items[i].OriginAmount
		}
		return items
	}

	totalEligible := 0
	for _, item := range items {
		totalEligible += item.OriginAmount
	}
	if totalEligible <= 0 {
		return items
	}

	remain := discount
	for i := range items {
		alloc := 0
		if i == len(items)-1 {
			alloc = remain
		} else {
			alloc = discount * items[i].OriginAmount / totalEligible
		}
		if alloc > items[i].OriginAmount {
			alloc = items[i].OriginAmount
		}
		if alloc < 0 {
			alloc = 0
		}
		items[i].ItemDiscountAmount = alloc
		items[i].PayableAmount = items[i].OriginAmount - alloc
		remain -= alloc
	}
	return items
}

func appendOrderStateLog(tx *gorm.DB, orderID string, fromStatus, toStatus int, operatorID uint, event, remark string) error {
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

func paymentOrderID(orderID string) string {
	return "PAY-" + orderID
}

func createMockPaymentOrder(tx *gorm.DB, order models.Order) (models.PaymentOrder, error) {
	payment := models.PaymentOrder{
		ID:             paymentOrderID(order.ID),
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

// CreateOrder 普通商品多项合并下单接口
func CreateOrder(c *gin.Context) {
	userIDVal, exists := c.Get("userId")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "未登录"})
		return
	}
	userID := userIDVal.(uint)

	var req CreateOrderReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "参数解析失败: " + err.Error()})
		return
	}

	if len(req.Items) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "下单商品清单不能为空"})
		return
	}

	if core.DB == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "数据库未就绪"})
		return
	}

	tx := core.DB.Begin()

	// 1. 获取收货地址详情
	var address models.Address
	if err := tx.Where("id = ? AND user_id = ?", req.AddressID, userID).First(&address).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{"message": "收货地址不存在"})
		return
	}

	// 地址拼接快照
	receiverAddrSnapshot := fmt.Sprintf("%s%s%s%s", address.Province, address.City, address.District, string(address.DetailAddress))

	var orderItems []models.OrderItem
	var subtotal int = 0

	// 2. 遍历商品子项，锁定物理库存并计算金额
	for _, item := range req.Items {
		// 行级悲观锁锁定 SKU 库存
		var sku models.Sku
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", item.SkuID).First(&sku).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusBadRequest, gin.H{"message": fmt.Sprintf("商品规格 ID %d 不存在", item.SkuID)})
			return
		}

		if sku.Stock < item.Quantity {
			tx.Rollback()
			c.JSON(http.StatusBadRequest, gin.H{"message": fmt.Sprintf("规格 %s 库存不足，仅剩 %d 件", sku.Title, sku.Stock)})
			return
		}

		// 扣减物理库存
		sku.Stock -= item.Quantity
		if err := tx.Save(&sku).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"message": "锁定库存失败"})
			return
		}

		orderItems = append(orderItems, models.OrderItem{
			SkuID:         item.SkuID,
			Price:         sku.Price,
			Quantity:      item.Quantity,
			OriginAmount:  sku.Price * item.Quantity,
			PayableAmount: sku.Price * item.Quantity,
		})

		subtotal += sku.Price * item.Quantity
	}

	// 3. 计算运费与税费
	// 满 99 元 (9900分) 包邮，否则 10 元 (1000分) 运费
	shippingFee := 1000
	if subtotal >= 9900 {
		shippingFee = 0
	}
	// 按小计金额的 5% 计算税费
	taxFee := subtotal * 5 / 100

	// 4. 计算优惠券折减金额
	discountAmount := 0
	if req.UserCouponID > 0 {
		var userCoupon models.UserCoupon
		err := tx.Preload("Coupon").Where("id = ? AND user_id = ? AND status = ?", req.UserCouponID, userID, 0).First(&userCoupon).Error
		if err != nil {
			tx.Rollback()
			c.JSON(http.StatusBadRequest, gin.H{"message": "优惠券不可用或已失效"})
			return
		}

		// 校验门槛金额
		if subtotal < userCoupon.Coupon.MinAmount {
			tx.Rollback()
			c.JSON(http.StatusBadRequest, gin.H{"message": fmt.Sprintf("未达到优惠券满 %d 元使用门槛", userCoupon.Coupon.MinAmount/100)})
			return
		}

		// 计算折减金额
		switch userCoupon.Coupon.Type {
		case 1: // 满减
			discountAmount = userCoupon.Coupon.Value
		case 2: // 折扣，如 Value = 90 代表 9 折，折减 10%
			discountAmount = subtotal * (100 - userCoupon.Coupon.Value) / 100
		case 3: // 无门槛
			discountAmount = userCoupon.Coupon.Value
		}

		if discountAmount > subtotal {
			discountAmount = subtotal // 折扣金额不能大于商品小计
		}

		// 将优惠券更新为已使用
		now := time.Now()
		userCoupon.Status = 1
		userCoupon.UsedAt = &now
		if err := tx.Save(&userCoupon).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"message": "优惠券扣减失败"})
			return
		}
	}

	// 应付总额
	totalAmount := subtotal + shippingFee + taxFee - discountAmount
	if subtotal > 0 && totalAmount <= 0 {
		totalAmount = 1
	}
	orderItems = allocateDiscountByAmount(orderItems, discountAmount)

	// 5. 创建订单
	orderID := fmt.Sprintf("GS-%d-%d", time.Now().Unix(), time.Now().Nanosecond()%1000)
	payExpireAt := time.Now().Add(60 * time.Second)
	order := models.Order{
		ID:                  orderID,
		UserID:              userID,
		TotalAmount:         totalAmount,
		DiscountAmount:      discountAmount,
		GoodsOriginAmount:   subtotal,
		GoodsDiscountAmount: discountAmount,
		ShippingFee:         shippingFee,
		TaxFee:              taxFee,
		PayableAmount:       totalAmount,
		Status:              models.OrderStatusPendingPayment,
		PayStatus:           models.PayStatusUnpaid,
		AfterSaleStatus:     models.AfterSaleStatusNone,
		UserCouponID:        req.UserCouponID,
		ReceiverName:        string(address.ReceiverName),
		ReceiverPhone:       string(address.ReceiverPhone),
		ReceiverAddr:        receiverAddrSnapshot,
		PayExpireAt:         &payExpireAt,
		Items:               orderItems,
	}

	if err := tx.Create(&order).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"message": "创建订单记录失败: " + err.Error()})
		return
	}

	if req.UserCouponID > 0 && discountAmount > 0 {
		for _, item := range order.Items {
			if item.ItemDiscountAmount <= 0 {
				continue
			}
			allocation := models.OrderPromotionAllocation{
				OrderID:            order.ID,
				OrderItemID:        item.ID,
				SkuID:              item.SkuID,
				UserCouponID:       req.UserCouponID,
				DiscountType:       1,
				DiscountAmount:     item.ItemDiscountAmount,
				AllocationSnapshot: fmt.Sprintf(`{"origin_amount":%d,"payable_amount":%d}`, item.OriginAmount, item.PayableAmount),
			}
			if err := tx.Create(&allocation).Error; err != nil {
				tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{"message": "保存优惠分摊失败: " + err.Error()})
				return
			}
		}
	}

	if err := appendOrderStateLog(tx, order.ID, 0, models.OrderStatusPendingPayment, userID, "ORDER_CREATED", "订单创建并预占库存"); err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"message": "保存订单状态日志失败: " + err.Error()})
		return
	}

	// 6. 清空云端购物车中对应的已被下单的 SKU 项
	var skuIDs []uint
	for _, item := range req.Items {
		skuIDs = append(skuIDs, item.SkuID)
	}
	tx.Where("user_id = ? AND sku_id IN ?", userID, skuIDs).Delete(&models.CartItem{})

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "提交订单事务失败"})
		return
	}

	// 7. 加入延迟队列 (普通订单设置 60 秒超时支付，便于自测演示)
	if core.RedisClient != nil {
		ctx := context.Background()
		now := time.Now().Unix()
		core.RedisClient.ZAdd(ctx, "seckill:delay_queue", redis.Z{
			Score:  float64(now + 60),
			Member: orderID,
		})
		pushSystemLog(ctx, "INFO", fmt.Sprintf("Order %s created. Reserved in delay queue for 60s.", orderID))
	}

	c.JSON(http.StatusOK, gin.H{
		"status":      "success",
		"orderId":     orderID,
		"payExpireAt": payExpireAt,
		"totalAmount": totalAmount,
	})
}

// PayOrder 支付订单接口
func PayOrder(c *gin.Context) {
	userIDVal, exists := c.Get("userId")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "未登录"})
		return
	}
	userID := userIDVal.(uint)

	var req PayReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "参数错误"})
		return
	}

	if core.DB == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "数据库未就绪"})
		return
	}

	tx := core.DB.Begin()
	var order models.Order
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&order, "id = ? AND user_id = ?", req.OrderID, userID).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{"message": "订单不存在"})
		return
	}

	if order.Status == models.OrderStatusPaid && order.PayStatus == models.PayStatusPaid {
		tx.Rollback()
		c.JSON(http.StatusOK, gin.H{"status": "paid", "paymentOrderId": paymentOrderID(order.ID)})
		return
	}

	if order.Status != models.OrderStatusPendingPayment || order.PayStatus != models.PayStatusUnpaid {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{"message": "当前订单状态不可支付"})
		return
	}

	payment, err := createMockPaymentOrder(tx, order)
	if err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"message": "创建支付单失败: " + err.Error()})
		return
	}

	now := time.Now()
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
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"message": "保存支付流水失败: " + err.Error()})
		return
	}

	payment.Status = models.PaymentStatusPaid
	payment.ChannelTradeNo = "MOCK-" + order.ID
	payment.PaidAt = &now
	payment.Version++
	if err := tx.Save(&payment).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"message": "支付单状态更新失败"})
		return
	}

	fromStatus := order.Status
	order.Status = models.OrderStatusPaid
	order.PayStatus = models.PayStatusPaid
	if err := tx.Save(&order).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"message": "支付状态更新失败"})
		return
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
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"message": "保存财务分录失败"})
		return
	}

	if err := appendOrderStateLog(tx, order.ID, fromStatus, models.OrderStatusPaid, userID, "ORDER_PAID", "模拟支付成功并入账"); err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"message": "保存订单状态日志失败"})
		return
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "提交支付事务失败"})
		return
	}

	// 从延迟队列中剔除
	if core.RedisClient != nil {
		ctx := context.Background()
		core.RedisClient.ZRem(ctx, "seckill:delay_queue", req.OrderID)
		core.RedisClient.ZRem(ctx, "seckill:delay_queue:processing", req.OrderID)
		pushSystemLog(ctx, "SUCCESS", fmt.Sprintf("Order: %s PAID. Removed from delay queue.", req.OrderID))
	}

	c.JSON(http.StatusOK, gin.H{"status": "paid", "paymentOrderId": payment.ID})
}

// MockPaymentCallback 模拟三方支付异步回调，提供金额校验和事件幂等。
func MockPaymentCallback(c *gin.Context) {
	var req MockPaymentCallbackReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "参数错误: " + err.Error()})
		return
	}
	if core.DB == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "数据库未就绪"})
		return
	}
	if req.PaymentOrderID == "" && req.OrderID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "paymentOrderId 和 orderId 至少传一个"})
		return
	}
	if req.PaymentOrderID == "" {
		req.PaymentOrderID = paymentOrderID(req.OrderID)
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
	var callbackErr error
	err := core.DB.Transaction(func(tx *gorm.DB) error {
		var existingTx models.PaymentTransaction
		if err := tx.Where("channel = ? AND channel_event_id = ?", models.PaymentChannelMock, req.EventID).First(&existingTx).Error; err == nil {
			return nil
		} else if err != gorm.ErrRecordNotFound {
			return err
		}

		var payment models.PaymentOrder
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&payment, "id = ?", req.PaymentOrderID).Error; err != nil {
			return err
		}

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

		now := time.Now()
		payment.Status = models.PaymentStatusPaid
		payment.ChannelTradeNo = req.ChannelTradeNo
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

		return appendOrderStateLog(tx, order.ID, fromStatus, models.OrderStatusPaid, order.UserID, "PAYMENT_CALLBACK_PAID", "模拟支付回调入账")
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "支付回调处理失败: " + err.Error()})
		return
	}
	if callbackErr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "支付回调处理失败: " + callbackErr.Error()})
		return
	}

	if core.RedisClient != nil {
		ctx := context.Background()
		orderID := req.OrderID
		if orderID == "" {
			var payment models.PaymentOrder
			if err := core.DB.Select("order_id").First(&payment, "id = ?", req.PaymentOrderID).Error; err == nil {
				orderID = payment.OrderID
			}
		}
		if orderID != "" {
			core.RedisClient.ZRem(ctx, "seckill:delay_queue", orderID)
			core.RedisClient.ZRem(ctx, "seckill:delay_queue:processing", orderID)
		}
	}

	c.JSON(http.StatusOK, gin.H{"status": "success"})
}

// GetOrders 查询当前登录用户的订单列表
func GetOrders(c *gin.Context) {
	userIDVal, exists := c.Get("userId")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "未登录"})
		return
	}
	userID := userIDVal.(uint)

	if core.DB == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "数据库未就绪"})
		return
	}

	statusStr := c.Query("status")

	query := core.ReplicaDB.Preload("Items.Sku").Preload("Items").Where("user_id = ?", userID)
	if statusStr != "" {
		status, err := strconv.Atoi(statusStr)
		if err == nil {
			query = query.Where("status = ?", status)
		}
	}

	var orders []models.Order
	if err := query.Order("created_at desc").Find(&orders).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "查询订单列表失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, orders)
}

// ApplyRefund 发起订单退款申请
func ApplyRefund(c *gin.Context) {
	userIDVal, exists := c.Get("userId")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "未登录"})
		return
	}
	userID := userIDVal.(uint)

	orderID := c.Param("id")

	var req RefundReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "参数解析失败: " + err.Error()})
		return
	}

	if core.DB == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "数据库未就绪"})
		return
	}

	err := core.DB.Transaction(func(tx *gorm.DB) error {
		var order models.Order
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Preload("Items").Where("id = ? AND user_id = ?", orderID, userID).First(&order).Error; err != nil {
			return err
		}

		// 必须是已支付状态才能申请退款
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

		return appendOrderStateLog(tx, order.ID, fromStatus, models.OrderStatusRefundApplying, userID, "AFTERSALE_APPLIED", req.RefundReason)
	})
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"message": "订单不存在"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"message": "提交退款申请失败: " + err.Error()})
		return
	}

	if core.RedisClient != nil {
		pushSystemLog(context.Background(), "WARN", fmt.Sprintf("Refund requested for Order %s. Reason: %s", orderID, req.RefundReason))
	}

	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "退款申请已提交，等待商家审核"})
}

// AuditRefund 商家审核退款
func AuditRefund(c *gin.Context) {
	orderID := c.Param("id")

	var req AuditRefundReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "参数解析失败"})
		return
	}

	if core.DB == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "数据库未就绪"})
		return
	}

	tx := core.DB.Begin()
	var order models.Order
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Preload("Items").First(&order, "id = ?", orderID).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{"message": "订单不存在"})
		return
	}

	// 必须是退款申请中状态
	if order.Status != models.OrderStatusRefundApplying {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{"message": "订单状态非退款申请中"})
		return
	}

	if req.Action == "approve" {
		var afterSale models.AfterSaleOrder
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("order_id = ? AND status = ?", order.ID, models.AfterSaleStatusApplying).First(&afterSale).Error; err != nil && err != gorm.ErrRecordNotFound {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"message": "查询售后单失败"})
			return
		}

		var payment models.PaymentOrder
		if err := tx.Where("order_id = ? AND status = ?", order.ID, models.PaymentStatusPaid).First(&payment).Error; err != nil {
			payment = models.PaymentOrder{
				ID:             paymentOrderID(order.ID),
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
			payment.PaidAt = &now
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&payment).Error; err != nil {
				tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{"message": "补写支付单失败"})
				return
			}
		}

		now := time.Now()
		refund := models.RefundOrder{
			ID:              "REF-" + order.ID,
			PaymentOrderID:  payment.ID,
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
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"message": "创建退款单失败"})
			return
		}

		fromStatus := order.Status
		order.Status = models.OrderStatusRefunded
		order.PayStatus = models.PayStatusRefunded
		order.AfterSaleStatus = models.AfterSaleStatusRefunded
		if err := tx.Save(&order).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"message": "审核状态更新失败"})
			return
		}

		if afterSale.ID != "" {
			afterSale.Status = models.AfterSaleStatusRefunded
			afterSale.ApprovedAmount = order.TotalAmount
			afterSale.RefundID = refund.ID
			if err := tx.Save(&afterSale).Error; err != nil {
				tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{"message": "更新售后单失败"})
				return
			}
		}

		// 释放物理库存
		for _, item := range order.Items {
			var sku models.Sku
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&sku, item.SkuID).Error; err == nil {
				sku.Stock += item.Quantity
				if err := tx.Save(&sku).Error; err != nil {
					tx.Rollback()
					c.JSON(http.StatusInternalServerError, gin.H{"message": "回补库存失败"})
					return
				}
				if err := tx.Model(&models.OrderItem{}).Where("id = ?", item.ID).Update("refunded_amount", item.PayableAmount).Error; err != nil {
					tx.Rollback()
					c.JSON(http.StatusInternalServerError, gin.H{"message": "更新订单行退款金额失败"})
					return
				}
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
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"message": "保存退款财务分录失败"})
			return
		}

		if err := appendOrderStateLog(tx, order.ID, fromStatus, models.OrderStatusRefunded, 0, "AFTERSALE_APPROVED", "商家审核通过并模拟退款"); err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"message": "保存订单状态日志失败"})
			return
		}

		// 如果是秒杀订单，额外把缓存库存退回 Redis
		for _, item := range order.Items {
			if item.SkuID == 1 { // SKU 1 为秒杀商品
				if core.RedisClient != nil {
					core.RedisClient.IncrBy(context.Background(), "seckill:stock:1", 1)
				}
			}
		}

		if err := tx.Commit().Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "提交退款审核事务失败"})
			return
		}
		if core.RedisClient != nil {
			pushSystemLog(context.Background(), "SUCCESS", fmt.Sprintf("Refund APPROVED for Order %s. Stock restored.", orderID))
		}
		c.JSON(http.StatusOK, gin.H{"status": "success", "message": "退款审核通过，已退款并归还库存"})
	} else if req.Action == "reject" {
		var afterSale models.AfterSaleOrder
		if err := tx.Where("order_id = ? AND status = ?", order.ID, models.AfterSaleStatusApplying).First(&afterSale).Error; err == nil {
			afterSale.Status = models.AfterSaleStatusRejected
			if err := tx.Save(&afterSale).Error; err != nil {
				tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{"message": "更新售后单失败"})
				return
			}
		}

		fromStatus := order.Status
		order.Status = models.OrderStatusRefundRejected
		order.AfterSaleStatus = models.AfterSaleStatusRejected
		if err := tx.Save(&order).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"message": "审核状态更新失败"})
			return
		}
		if err := appendOrderStateLog(tx, order.ID, fromStatus, models.OrderStatusRefundRejected, 0, "AFTERSALE_REJECTED", "商家拒绝退款申请"); err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"message": "保存订单状态日志失败"})
			return
		}
		if err := tx.Commit().Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "提交退款审核事务失败"})
			return
		}
		if core.RedisClient != nil {
			pushSystemLog(context.Background(), "WARN", fmt.Sprintf("Refund REJECTED for Order %s.", orderID))
		}
		c.JSON(http.StatusOK, gin.H{"status": "success", "message": "退款审核已拒绝，订单已退回"})
	} else {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{"message": "无效的操作指令"})
	}
}

// ==========================================
// 6. Reliable Delay Queue Worker with ACK & DLQ
// ==========================================

func StartReliableDelayQueueWorker() {
	ticker := time.NewTicker(1 * time.Second)
	ctx := context.Background()
	log.Println("[INFO] 高可用延迟队列 Worker 已启动...")

	for range ticker.C {
		if core.RedisClient == nil || core.DB == nil {
			continue
		}

		now := time.Now().Unix()
		// 1. 捞取已到期的订单
		orders, err := core.RedisClient.ZRangeByScore(ctx, "seckill:delay_queue", &redis.ZRangeBy{
			Min: "-inf",
			Max: fmt.Sprintf("%d", now),
		}).Result()

		if err != nil || len(orders) == 0 {
			// 1.1 捞取在 processing ZSet 中滞留超时 (如超过30秒未 Ack) 的订单，防止 Worker 挂掉造成任务丢失
			staleTime := time.Now().Add(-30 * time.Second).Unix()
			orders, err = core.RedisClient.ZRangeByScore(ctx, "seckill:delay_queue:processing", &redis.ZRangeBy{
				Min: "-inf",
				Max: fmt.Sprintf("%d", staleTime),
			}).Result()
			if err != nil || len(orders) == 0 {
				continue
			}
		}

		for _, orderID := range orders {
			// 2. 原子性抢占任务：从原有 ZSet 中移出，并打入 processing ZSet，设置 Score 为过期时间戳 (当前时间+30s)
			// 利用 ZRem 返回值确认是否由当前 Worker 抢占成功，防止并发处理
			removed, _ := core.RedisClient.ZRem(ctx, "seckill:delay_queue", orderID).Result()

			// 如果已经在 processing 里，可能是在进行重试，我们也支持将其再次抢占
			isStaleRetry := false
			if removed == 0 {
				// 检查是不是从 processing 捞出来的滞留超时任务
				isProcessing, _ := core.RedisClient.ZScore(ctx, "seckill:delay_queue:processing", orderID).Result()
				if isProcessing > 0 {
					isStaleRetry = true
				}
			}

			if removed == 0 && !isStaleRetry {
				// 抢占失败，被其他 Worker 处理了
				continue
			}

			// 移入/更新在 processing 中的状态
			core.RedisClient.ZAdd(ctx, "seckill:delay_queue:processing", redis.Z{
				Score:  float64(time.Now().Unix() + 30),
				Member: orderID,
			})

			// 3. 执行超时取消数据库操作 (包裹在事务中)
			err = processCancelOrder(orderID)
			if err != nil {
				// 处理失败，进行重试计数
				retryKey := "retry:count:" + orderID
				retries, _ := core.RedisClient.Incr(ctx, retryKey).Result()
				core.RedisClient.Expire(ctx, retryKey, 1*time.Hour)

				if retries >= 3 {
					// 失败达到 3 次，判定为死信任务。打入死信数据库表，并从 Redis 剔除，防止死循环
					log.Printf("[DLQ] 订单 %s 重试 3 次均取消失败，移入死信数据库表. 错误: %v", orderID, err)
					core.DB.Create(&models.DeadLetterOrder{
						OrderID: orderID,
						Reason:  fmt.Sprintf("重试3次均取消失败。最后错误: %v", err),
					})

					// 彻底移出 Redis (Ack)
					core.RedisClient.ZRem(ctx, "seckill:delay_queue:processing", orderID)
					core.RedisClient.Del(ctx, retryKey)
					pushSystemLog(ctx, "ERROR", fmt.Sprintf("DLQ: Order %s failed to cancel. Moved to dead-letter storage.", orderID))
				} else {
					log.Printf("[Worker] 超时订单 %s 取消事务失败，重试数: %d/3. 错误: %v", orderID, retries, err)
				}
				continue
			}

			// 4. 处理成功，确认 Ack：从 processing 队列及计数器中移除
			core.RedisClient.ZRem(ctx, "seckill:delay_queue:processing", orderID)
			core.RedisClient.Del(ctx, "retry:count:"+orderID)
		}
	}
}

// processCancelOrder 超时订单库事务处理
func processCancelOrder(orderID string) error {
	return core.DB.Transaction(func(tx *gorm.DB) error {
		var order models.Order
		if err := tx.Preload("Items").First(&order, "id = ?", orderID).Error; err != nil {
			return err
		}

		// 只有待支付状态才能取消
		if order.Status != models.OrderStatusPendingPayment {
			return nil // 状态已被更改（如已支付），属于正常流程，直接成功
		}

		// 更新为已取消状态
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
		if err := appendOrderStateLog(tx, order.ID, fromStatus, models.OrderStatusCanceled, order.UserID, "ORDER_TIMEOUT_CANCELED", "支付超时自动取消并释放库存"); err != nil {
			return err
		}

		// 释放锁定的物理库存
		for _, item := range order.Items {
			var sku models.Sku
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&sku, item.SkuID).Error; err == nil {
				sku.Stock += item.Quantity
				if err := tx.Save(&sku).Error; err != nil {
					return err
				}
			} else {
				return err
			}
		}

		// 若为秒杀商品 (SKU ID 1)，将 Valkey 库存缓存也退回增加 1
		for _, item := range order.Items {
			if item.SkuID == 1 {
				if core.RedisClient != nil {
					core.RedisClient.IncrBy(context.Background(), "seckill:stock:1", 1)
				}
			}
		}

		pushSystemLog(context.Background(), "WARN", fmt.Sprintf("Delay worker: Order %s EXPIRED. Cancelled and rolled back stock.", orderID))
		return nil
	})
}

const seckillLua = `
local key = KEYS[1]
local change = tonumber(ARGV[1])

local current = redis.call('get', key)
if not current then
    return -1
end

local current_stock = tonumber(current)
if current_stock < change then
    return 0
else
    redis.call('decrby', key, change)
    return 1
end
`

// Seckill 秒杀下单接口 (高并发原子扣库存)
func Seckill(c *gin.Context) {
	userIDVal, exists := c.Get("userId")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "未登录"})
		return
	}
	userID := userIDVal.(uint)

	if core.RedisClient == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Valkey/Redis 未就绪"})
		return
	}

	ctx := context.Background()

	// 1. Redis Lua 预扣库存
	res, err := core.RedisClient.Eval(ctx, seckillLua, []string{"seckill:stock:1"}, 1).Result()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Redis执行失败"})
		return
	}

	statusVal, ok := res.(int64)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Redis返回值类型异常"})
		return
	}

	if statusVal == -1 {
		// 未初始化，在此重新设定为 87
		core.RedisClient.Set(ctx, "seckill:stock:1", 87, 0)
		res, _ = core.RedisClient.Eval(ctx, seckillLua, []string{"seckill:stock:1"}, 1).Result()
		statusVal, _ = res.(int64)
	}

	if statusVal == 0 {
		pushSystemLog(ctx, "ERROR", fmt.Sprintf("User %d Seckill failed. Valkey cache stock is 0.", userID))
		c.JSON(http.StatusBadRequest, gin.H{"message": "库存不足"})
		return
	}

	// 2. 获取剩余库存
	leftStock, _ := core.RedisClient.Get(ctx, "seckill:stock:1").Result()

	// 3. 生成订单并写入数据库 (status=1, 待支付)
	orderID := fmt.Sprintf("SK-%d-%d", time.Now().Unix(), time.Now().Nanosecond()%1000)

	if core.DB != nil {
		// 秒杀订单价格固定为 399.00元 (39900分)
		payExpireAt := time.Now().Add(15 * time.Second)
		order := models.Order{
			ID:                orderID,
			UserID:            userID,
			TotalAmount:       39900,
			GoodsOriginAmount: 39900,
			PayableAmount:     39900,
			Status:            models.OrderStatusPendingPayment,
			PayStatus:         models.PayStatusUnpaid,
			AfterSaleStatus:   models.AfterSaleStatusNone,
			ReceiverName:      "秒杀快捷收货",
			ReceiverPhone:     "13800000000",
			ReceiverAddr:      "虚拟网络秒杀节点",
			PayExpireAt:       &payExpireAt,
		}
		if err := core.DB.Create(&order).Error; err == nil {
			orderItem := models.OrderItem{
				OrderID:       orderID,
				SkuID:         1,
				Price:         39900,
				Quantity:      1,
				OriginAmount:  39900,
				PayableAmount: 39900,
			}
			core.DB.Create(&orderItem)
			core.DB.Create(&models.OrderStateLog{
				OrderID:      orderID,
				ToStatus:     models.OrderStatusPendingPayment,
				OperatorType: 1,
				OperatorID:   userID,
				Event:        "SECKILL_ORDER_CREATED",
				Remark:       "秒杀订单创建并预扣 Redis 库存",
			})
		}
	}

	// 4. 加入延迟队列 (秒杀订单限 15 秒内超时支付，以匹配前端倒计时)
	now := time.Now().Unix()
	core.RedisClient.ZAdd(ctx, "seckill:delay_queue", redis.Z{
		Score:  float64(now + 15),
		Member: orderID,
	})

	pushSystemLog(ctx, "INFO", fmt.Sprintf("User %d Valkey Lua pre-decrement SUCCESS. Stock left: %s. Order %s. Pay in 15s.", userID, leftStock, orderID))

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"orderId": orderID,
	})
}
