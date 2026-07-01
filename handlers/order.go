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

type CreatePaymentReq struct {
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
	Type         int             `json:"type"`
	RefundReason string          `json:"refundReason" binding:"required"`
	RefundProof  string          `json:"refundProof"`
	Items        []RefundItemReq `json:"items"`
}

type RefundItemReq struct {
	OrderItemID uint `json:"orderItemId" binding:"required"`
	Quantity    int  `json:"quantity" binding:"required"`
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

const (
	orderDelayQueueKey           = "delay:order_payment_timeout"
	orderDelayProcessingKey      = "delay:order_payment_timeout:processing"
	delayTaskLeaseSeconds        = 30
	delayTaskClaimLimit          = 50
	seckillPendingPaymentSeconds = 15
)

const claimDelayTasksLua = `
local source = KEYS[1]
local processing = KEYS[2]
local now = tonumber(ARGV[1])
local lease_until = tonumber(ARGV[2])
local limit = tonumber(ARGV[3])

local items = redis.call('zrangebyscore', source, '-inf', now, 'LIMIT', 0, limit)
for _, item in ipairs(items) do
  local removed = redis.call('zrem', source, item)
  if removed == 1 then
    redis.call('zadd', processing, lease_until, item)
  end
end
return items
`

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
	core.RedisClient.ZAdd(ctx, orderDelayQueueKey, redis.Z{
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

	for _, key := range []string{orderDelayQueueKey, orderDelayProcessingKey} {
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
// @Summary 创建订单
// @Tags order
// @Accept json
// @Produce json
// @Param body body CreateOrderReq true "下单参数"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /api/orders [post]
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
// @Summary 结算试算
// @Tags order
// @Accept json
// @Produce json
// @Param body body checkout.PreviewRequest true "试算参数"
// @Success 200 {object} checkout.Preview
// @Failure 400 {object} map[string]interface{}
// @Router /api/checkout/preview [post]
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

// CreatePayment 创建或幂等返回当前用户订单的支付单
// @Summary 创建支付单
// @Tags payment
// @Accept json
// @Produce json
// @Param body body CreatePaymentReq true "支付单参数"
// @Success 200 {object} map[string]interface{}
// @Router /api/payments [post]
func CreatePayment(c *gin.Context) {
	userIDVal, exists := c.Get("userId")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "未登录"})
		return
	}
	userID := userIDVal.(uint)

	var req CreatePaymentReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "参数错误"})
		return
	}
	if core.DB == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "数据库未就绪"})
		return
	}

	result, err := paymentsvc.NewService(core.DB).CreateOrGetPaymentOrder(userID, req.OrderID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"message": "订单不存在"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

// GetPayment 查询当前用户可访问的支付单
// @Summary 查询支付单
// @Tags payment
// @Produce json
// @Param id path string true "支付单 ID"
// @Success 200 {object} models.PaymentOrder
// @Router /api/payments/{id} [get]
func GetPayment(c *gin.Context) {
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

	payment, err := paymentsvc.NewService(core.ReplicaDB).GetPaymentOrder(userID, c.Param("id"))
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"message": "支付单不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"message": "查询支付单失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, payment)
}

// PayOrder 支付订单接口
// @Summary 模拟支付订单
// @Tags payment
// @Accept json
// @Produce json
// @Param body body PayReq true "支付参数"
// @Success 200 {object} map[string]interface{}
// @Router /api/pay [post]
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
// @Summary 模拟支付回调
// @Tags payment
// @Accept json
// @Produce json
// @Param body body MockPaymentCallbackReq true "回调参数"
// @Success 200 {object} map[string]interface{}
// @Router /api/payments/callback/mock [post]
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
// @Summary 查询订单列表
// @Tags order
// @Produce json
// @Param status query int false "订单状态"
// @Success 200 {array} models.Order
// @Router /api/orders [get]
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

	query := core.ReplicaDB.Preload("Items").Where("user_id = ?", userID)
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
	ordersvc.EnrichOrdersItemSKUs(core.ReplicaDB, orders)

	c.JSON(http.StatusOK, orders)
}

// GetOrderDetail 查询当前登录用户的订单详情聚合信息
// @Summary 查询订单详情
// @Tags order
// @Produce json
// @Param id path string true "订单 ID"
// @Success 200 {object} ordersvc.Detail
// @Router /api/orders/{id} [get]
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
// @Summary 申请售后退款
// @Tags aftersale
// @Accept json
// @Produce json
// @Param id path string true "订单 ID"
// @Param body body RefundReq true "退款参数"
// @Success 200 {object} map[string]interface{}
// @Router /api/orders/{id}/refund [post]
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

	applyItems := make([]aftersalesvc.ApplyItem, 0, len(req.Items))
	for _, item := range req.Items {
		applyItems = append(applyItems, aftersalesvc.ApplyItem{OrderItemID: item.OrderItemID, Quantity: item.Quantity})
	}
	err := aftersalesvc.NewService(core.DB).ApplyRefund(userID, orderID, aftersalesvc.ApplyRequest{
		Type:         req.Type,
		RefundReason: req.RefundReason,
		RefundProof:  req.RefundProof,
		Items:        applyItems,
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
// @Summary 审核售后退款
// @Tags aftersale
// @Accept json
// @Produce json
// @Param id path string true "订单 ID"
// @Param body body AuditRefundReq true "审核参数"
// @Success 200 {object} map[string]interface{}
// @Router /api/admin/orders/{id}/refund/audit [post]
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
	lastSweep := time.Now()

	for range ticker.C {
		if core.RedisClient == nil || core.DB == nil {
			continue
		}

		processDelayQueue(ctx, orderDelayQueueKey, orderDelayProcessingKey)
		if time.Since(lastSweep) >= time.Minute {
			sweepExpiredPendingOrders(ctx)
			lastSweep = time.Now()
		}
	}
}

// processCancelOrder 超时订单库事务处理
func processCancelOrder(orderID string) error {
	var resp struct {
		OrderID  string `json:"orderId"`
		Status   int    `json:"status"`
		Canceled bool   `json:"canceled"`
	}
	if err := core.CallInternalService(core.DB, 8105, "POST", fmt.Sprintf("/api/internal/orders/%s/cancel-pending", orderID), map[string]interface{}{
		"reason": "支付超时自动取消并释放库存",
	}, &resp); err != nil {
		return err
	}
	if strings.HasPrefix(orderID, "SK-") && resp.Canceled && core.RedisClient != nil {
		core.RedisClient.IncrBy(context.Background(), "seckill:stock:1", 1)
	}
	pushSystemLog(context.Background(), "WARN", fmt.Sprintf("Delay worker: Order %s EXPIRED. Cancelled and released reservations.", orderID))
	return nil
}

func processDelayQueue(ctx context.Context, sourceKey, processingKey string) {
	now := time.Now().Unix()
	members, err := claimDelayTasks(ctx, sourceKey, processingKey, now)
	if err != nil || len(members) == 0 {
		members, err = claimDelayTasks(ctx, processingKey, processingKey, now)
		if err != nil || len(members) == 0 {
			return
		}
	}

	for _, member := range members {
		orderID := delayTaskOrderID(member)

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

func claimDelayTasks(ctx context.Context, sourceKey, processingKey string, now int64) ([]string, error) {
	leaseUntil := now + delayTaskLeaseSeconds
	res, err := core.RedisClient.Eval(ctx, claimDelayTasksLua, []string{sourceKey, processingKey}, now, leaseUntil, delayTaskClaimLimit).Result()
	if err != nil {
		return nil, err
	}
	switch items := res.(type) {
	case []interface{}:
		members := make([]string, 0, len(items))
		for _, item := range items {
			if member, ok := item.(string); ok {
				members = append(members, member)
			}
		}
		return members, nil
	case []string:
		return items, nil
	default:
		return nil, nil
	}
}

func sweepExpiredPendingOrders(ctx context.Context) {
	var orderIDs []string
	if err := core.CallInternalService(core.DB, 8105, "GET", "/api/internal/orders/expired-pending?limit=100", nil, &orderIDs); err != nil {
		log.Printf("[Worker] 超时订单兜底扫描失败: %v", err)
		return
	}
	for _, orderID := range orderIDs {
		if err := processCancelOrder(orderID); err != nil {
			log.Printf("[Worker] 超时订单 %s 兜底取消失败: %v", orderID, err)
		} else {
			pushSystemLog(ctx, "WARN", fmt.Sprintf("Sweep: Order %s expired and was cancelled.", orderID))
		}
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
// @Summary 秒杀下单
// @Tags order
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/seckill [post]
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

	// 3. 生成订单并交给订单服务写入数据库
	orderID := fmt.Sprintf("SK-%d-%d", time.Now().Unix(), time.Now().Nanosecond()%1000)
	payExpireAt := time.Now().Add(seckillPendingPaymentSeconds * time.Second)
	if err := core.CallInternalService(core.DB, 8105, "POST", "/api/internal/orders/seckill-create", map[string]interface{}{
		"orderId":     orderID,
		"userId":      userID,
		"skuId":       1,
		"price":       39900,
		"quantity":    1,
		"payExpireAt": payExpireAt,
	}, nil); err != nil {
		core.RedisClient.IncrBy(ctx, "seckill:stock:1", 1)
		c.JSON(http.StatusInternalServerError, gin.H{"message": "秒杀订单创建失败: " + err.Error()})
		return
	}

	// 4. 加入通用延迟队列 (秒杀订单限短时间内超时支付，以匹配前端倒计时)
	enqueueOrderPaymentTimeout(orderID, payExpireAt)

	pushSystemLog(ctx, "INFO", fmt.Sprintf("User %d Valkey Lua pre-decrement SUCCESS. Stock left: %s. Order %s. Pay in 15s.", userID, leftStock, orderID))

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"orderId": orderID,
	})
}

// GetMetrics 看板专属指标与日志同步 API
func GetMetrics(c *gin.Context) {
	ctx := c.Request.Context()

	// 获取秒杀商品缓存库存数
	var stock int
	if core.RedisClient != nil {
		stockStr, err := core.RedisClient.Get(ctx, "seckill:stock:1").Result()
		if err != nil {
			core.RedisClient.Set(ctx, "seckill:stock:1", 87, 0)
			stockStr = "87"
		}
		stock, _ = strconv.Atoi(stockStr)
	}

	// 延迟锁库存计数
	var lockStock int64 = 0
	if core.RedisClient != nil {
		oldDelayCount, _ := core.RedisClient.ZCard(ctx, "seckill:delay_queue").Result()
		oldProcessingCount, _ := core.RedisClient.ZCard(ctx, "seckill:delay_queue:processing").Result()
		newDelayCount, _ := core.RedisClient.ZCard(ctx, "delay:order_payment_timeout").Result()
		newProcessingCount, _ := core.RedisClient.ZCard(ctx, "delay:order_payment_timeout:processing").Result()
		lockStock = oldDelayCount + oldProcessingCount + newDelayCount + newProcessingCount
	}

	// 已支付订单总额及销售额
	var ordersPaid int64 = 0
	var totalRevenueCent int64 = 0

	if core.DB != nil {
		core.DB.Model(&models.Order{}).Where("status = ?", models.OrderStatusPaid).Count(&ordersPaid)
		core.DB.Model(&models.Order{}).Where("status = ?", models.OrderStatusPaid).Select("COALESCE(SUM(total_amount), 0)").Row().Scan(&totalRevenueCent)
	}

	revenueVal := float64(totalRevenueCent) / 100.0
	revenueStr := formatCurrency(revenueVal)

	// 获取日志记录
	var logs []LogItem
	if core.RedisClient != nil {
		logStrs, _ := core.RedisClient.LRange(ctx, "seckill:logs", 0, -1).Result()
		for _, s := range logStrs {
			var item LogItem
			if err := json.Unmarshal([]byte(s), &item); err == nil {
				logs = append(logs, item)
			}
		}
	}

	// 获取待支付订单 (status = 10)
	var pendingOrders []models.Order
	if core.DB != nil {
		core.DB.Where("status = ?", models.OrderStatusPendingPayment).Order("created_at desc").Find(&pendingOrders)
	}

	c.JSON(http.StatusOK, gin.H{
		"metrics": gin.H{
			"seckillStock": stock,
			"lockStock":    lockStock,
			"ordersPaid":   ordersPaid,
			"revenue":      revenueStr,
		},
		"logs":          logs,
		"pendingOrders": pendingOrders,
	})
}

func formatCurrency(val float64) string {
	s := fmt.Sprintf("%.2f", val)
	parts := strings.Split(s, ".")
	integer := parts[0]
	fraction := parts[1]

	var result []string
	for i, c := range integer {
		if i > 0 && (len(integer)-i)%3 == 0 {
			result = append(result, ",")
		}
		result = append(result, string(c))
	}
	return strings.Join(result, "") + "." + fraction
}
