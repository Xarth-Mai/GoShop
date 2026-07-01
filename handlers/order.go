package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"GoShop/core"
	aftersalesvc "GoShop/internal/aftersale"
	checkoutsvc "GoShop/internal/checkout"
	ordersvc "GoShop/internal/order"
	paymentsvc "GoShop/internal/payment"
	"GoShop/models"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
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

type DelayTask struct {
	TaskID    string `json:"taskId"`
	Type      string `json:"type"`
	OrderID   string `json:"orderId"`
	ExecuteAt int64  `json:"executeAt"`
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

func enqueueOrderPaymentTimeout(orderID string, executeAt time.Time) {
	if core.RedisClient == nil {
		return
	}
	ctx := context.Background()
	task := DelayTask{
		TaskID:    "TASK-" + orderID,
		Type:      "ORDER_PAYMENT_TIMEOUT",
		OrderID:   orderID,
		ExecuteAt: executeAt.Unix(),
	}
	bytes, _ := json.Marshal(task)
	core.RedisClient.ZAdd(ctx, "delay:order_payment_timeout", redis.Z{
		Score:  float64(task.ExecuteAt),
		Member: string(bytes),
	})
	pushSystemLog(ctx, "INFO", fmt.Sprintf("Order %s created. Reserved in delay queue until %s.", orderID, executeAt.Format(time.RFC3339)))
}

func removeOrderDelayTasks(orderID string) {
	if core.RedisClient == nil {
		return
	}
	ctx := context.Background()
	core.RedisClient.ZRem(ctx, "seckill:delay_queue", orderID)
	core.RedisClient.ZRem(ctx, "seckill:delay_queue:processing", orderID)

	for _, key := range []string{"delay:order_payment_timeout", "delay:order_payment_timeout:processing"} {
		items, _ := core.RedisClient.ZRange(ctx, key, 0, -1).Result()
		for _, item := range items {
			if delayTaskOrderID(item) == orderID {
				core.RedisClient.ZRem(ctx, key, item)
			}
		}
	}
}

func delayTaskOrderID(member string) string {
	var task DelayTask
	if err := json.Unmarshal([]byte(member), &task); err == nil && task.OrderID != "" {
		return task.OrderID
	}
	return member
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

	serviceReq := checkoutsvc.PreviewRequest{
		AddressID:    req.AddressID,
		UserCouponID: req.UserCouponID,
	}
	for _, item := range req.Items {
		serviceReq.Items = append(serviceReq.Items, checkoutsvc.ItemReq{SkuID: item.SkuID, Quantity: item.Quantity})
	}
	result, err := ordersvc.NewService(core.DB).CreateOrder(userID, serviceReq)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "创建订单失败: " + err.Error()})
		return
	}
	enqueueOrderPaymentTimeout(result.OrderID, result.PayExpireAt)

	c.JSON(http.StatusOK, gin.H{
		"status":      "success",
		"orderId":     result.OrderID,
		"payExpireAt": result.PayExpireAt,
		"totalAmount": result.TotalAmount,
	})
}

// PreviewCheckout 后端统一结算试算接口
func PreviewCheckout(c *gin.Context) {
	userIDVal, exists := c.Get("userId")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "未登录"})
		return
	}
	userID := userIDVal.(uint)

	var req checkoutsvc.PreviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "参数解析失败: " + err.Error()})
		return
	}
	if core.DB == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "数据库未就绪"})
		return
	}
	preview, err := checkoutsvc.NewService(core.ReplicaDB).Calculate(userID, req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "结算试算失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, preview)
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

	result, err := paymentsvc.NewService(core.DB).PayMockOrder(userID, req.OrderID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"message": "订单不存在"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	removeOrderDelayTasks(req.OrderID)
	pushSystemLog(context.Background(), "SUCCESS", fmt.Sprintf("Order: %s PAID. Removed from delay queue.", req.OrderID))

	c.JSON(http.StatusOK, gin.H{"status": "paid", "paymentOrderId": result.PaymentOrderID})
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

	orderID, err := paymentsvc.NewService(core.DB).HandleMockCallback(paymentsvc.MockCallbackRequest(req))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "支付回调处理失败: " + err.Error()})
		return
	}
	if orderID != "" {
		removeOrderDelayTasks(orderID)
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

// GetOrderDetail 查询当前登录用户的订单详情聚合信息
func GetOrderDetail(c *gin.Context) {
	userIDVal, exists := c.Get("userId")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "未登录"})
		return
	}
	userID := userIDVal.(uint)
	orderID := c.Param("id")

	if core.DB == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "数据库未就绪"})
		return
	}

	detail, err := ordersvc.NewService(core.ReplicaDB).GetOrderDetail(userID, orderID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"message": "订单不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"message": "查询订单详情失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, detail)
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

	err := aftersalesvc.NewService(core.DB).ApplyRefund(userID, orderID, aftersalesvc.ApplyRequest{
		RefundReason: req.RefundReason,
		RefundProof:  req.RefundProof,
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

	if err := aftersalesvc.NewService(core.DB).AuditRefund(orderID, aftersalesvc.AuditRequest{Action: req.Action}); err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"message": "订单不存在"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	if req.Action == "approve" {
		if core.RedisClient != nil {
			core.RedisClient.IncrBy(context.Background(), "seckill:stock:1", 1)
			pushSystemLog(context.Background(), "SUCCESS", fmt.Sprintf("Refund APPROVED for Order %s. Stock restored.", orderID))
		}
		c.JSON(http.StatusOK, gin.H{"status": "success", "message": "退款审核通过，已退款并归还库存"})
	} else if req.Action == "reject" {
		if core.RedisClient != nil {
			pushSystemLog(context.Background(), "WARN", fmt.Sprintf("Refund REJECTED for Order %s.", orderID))
		}
		c.JSON(http.StatusOK, gin.H{"status": "success", "message": "退款审核已拒绝，订单已退回"})
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

		processDelayQueue(ctx, "delay:order_payment_timeout", "delay:order_payment_timeout:processing")
		processDelayQueue(ctx, "seckill:delay_queue", "seckill:delay_queue:processing")
	}
}

// processCancelOrder 超时订单库事务处理
func processCancelOrder(orderID string) error {
	if err := ordersvc.NewService(core.DB).CancelPendingOrder(orderID, "支付超时自动取消并释放库存"); err != nil {
		return err
	}
	if strings.HasPrefix(orderID, "SK-") && core.RedisClient != nil {
		var order models.Order
		if err := core.DB.Select("status").First(&order, "id = ?", orderID).Error; err == nil && order.Status == models.OrderStatusCanceled {
			core.RedisClient.IncrBy(context.Background(), "seckill:stock:1", 1)
		}
	}
	pushSystemLog(context.Background(), "WARN", fmt.Sprintf("Delay worker: Order %s EXPIRED. Cancelled and released reservations.", orderID))
	return nil
}

func processDelayQueue(ctx context.Context, sourceKey, processingKey string) {
	now := time.Now().Unix()
	members, err := core.RedisClient.ZRangeByScore(ctx, sourceKey, &redis.ZRangeBy{
		Min: "-inf",
		Max: fmt.Sprintf("%d", now),
	}).Result()
	fromProcessing := false
	if err != nil || len(members) == 0 {
		staleTime := time.Now().Add(-30 * time.Second).Unix()
		members, err = core.RedisClient.ZRangeByScore(ctx, processingKey, &redis.ZRangeBy{
			Min: "-inf",
			Max: fmt.Sprintf("%d", staleTime),
		}).Result()
		fromProcessing = true
		if err != nil || len(members) == 0 {
			return
		}
	}

	for _, member := range members {
		orderID := delayTaskOrderID(member)
		removed := int64(0)
		if !fromProcessing {
			removed, _ = core.RedisClient.ZRem(ctx, sourceKey, member).Result()
		}
		if removed == 0 && !fromProcessing {
			continue
		}

		core.RedisClient.ZAdd(ctx, processingKey, redis.Z{
			Score:  float64(time.Now().Unix() + 30),
			Member: member,
		})

		if err := processCancelOrder(orderID); err != nil {
			retryKey := "retry:count:" + orderID
			retries, _ := core.RedisClient.Incr(ctx, retryKey).Result()
			core.RedisClient.Expire(ctx, retryKey, 1*time.Hour)
			if retries >= 3 {
				log.Printf("[DLQ] 订单 %s 重试 3 次均取消失败，移入死信数据库表. 错误: %v", orderID, err)
				core.DB.Create(&models.DeadLetterOrder{
					OrderID: orderID,
					Reason:  fmt.Sprintf("重试3次均取消失败。最后错误: %v", err),
				})
				core.RedisClient.ZRem(ctx, processingKey, member)
				core.RedisClient.Del(ctx, retryKey)
				pushSystemLog(ctx, "ERROR", fmt.Sprintf("DLQ: Order %s failed to cancel. Moved to dead-letter storage.", orderID))
			} else {
				log.Printf("[Worker] 超时订单 %s 取消事务失败，重试数: %d/3. 错误: %v", orderID, retries, err)
			}
			continue
		}

		core.RedisClient.ZRem(ctx, processingKey, member)
		core.RedisClient.Del(ctx, "retry:count:"+orderID)
	}
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

	// 4. 加入通用延迟队列 (秒杀订单限 15 秒内超时支付，以匹配前端倒计时)
	enqueueOrderPaymentTimeout(orderID, time.Now().Add(15*time.Second))

	pushSystemLog(ctx, "INFO", fmt.Sprintf("User %d Valkey Lua pre-decrement SUCCESS. Stock left: %s. Order %s. Pay in 15s.", userID, leftStock, orderID))

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"orderId": orderID,
	})
}
