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
		if err := tx.Set("gorm:query_option", "FOR UPDATE").Where("id = ?", item.SkuID).First(&sku).Error; err != nil {
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
			SkuID:    item.SkuID,
			Price:    sku.Price,
			Quantity: item.Quantity,
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
	if totalAmount < 0 {
		totalAmount = 0
	}

	// 5. 创建订单
	orderID := fmt.Sprintf("GS-%d-%d", time.Now().Unix(), time.Now().Nanosecond()%1000)
	order := models.Order{
		ID:             orderID,
		UserID:         userID,
		TotalAmount:    totalAmount,
		DiscountAmount: discountAmount,
		ShippingFee:    shippingFee,
		TaxFee:         taxFee,
		Status:         1, // 待支付
		ReceiverName:   string(address.ReceiverName),
		ReceiverPhone:  string(address.ReceiverPhone),
		ReceiverAddr:   receiverAddrSnapshot,
		Items:          orderItems,
	}

	if err := tx.Create(&order).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"message": "创建订单记录失败: " + err.Error()})
		return
	}

	// 6. 清空云端购物车中对应的已被下单的 SKU 项
	var skuIDs []uint
	for _, item := range req.Items {
		skuIDs = append(skuIDs, item.SkuID)
	}
	tx.Where("user_id = ? AND sku_id IN ?", userID, skuIDs).Delete(&models.CartItem{})

	tx.Commit()

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
	if err := tx.First(&order, "id = ? AND user_id = ?", req.OrderID, userID).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{"message": "订单不存在"})
		return
	}

	if order.Status != 1 {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{"message": "当前订单状态不可支付"})
		return
	}

	// 更新为已支付 (2)
	order.Status = 2
	if err := tx.Save(&order).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"message": "支付状态更新失败"})
		return
	}

	tx.Commit()

	// 从延迟队列中剔除
	if core.RedisClient != nil {
		ctx := context.Background()
		core.RedisClient.ZRem(ctx, "seckill:delay_queue", req.OrderID)
		core.RedisClient.ZRem(ctx, "seckill:delay_queue:processing", req.OrderID)
		pushSystemLog(ctx, "SUCCESS", fmt.Sprintf("Order: %s PAID. Removed from delay queue.", req.OrderID))
	}

	c.JSON(http.StatusOK, gin.H{"status": "paid"})
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

	var order models.Order
	if err := core.DB.Where("id = ? AND user_id = ?", orderID, userID).First(&order).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "订单不存在"})
		return
	}

	// 必须是已支付状态才能申请退款 (2)
	if order.Status != 2 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "该订单当前状态不支持申请退款"})
		return
	}

	order.Status = 4 // 申请退款中
	order.RefundReason = req.RefundReason
	order.RefundProof = req.RefundProof

	if err := core.DB.Save(&order).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "提交退款申请失败: " + err.Error()})
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
	if err := tx.Preload("Items").First(&order, "id = ?", orderID).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{"message": "订单不存在"})
		return
	}

	// 必须是退款申请中状态 (4)
	if order.Status != 4 {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{"message": "订单状态非退款申请中"})
		return
	}

	if req.Action == "approve" {
		order.Status = 5 // 已退款
		if err := tx.Save(&order).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"message": "审核状态更新失败"})
			return
		}

		// 释放物理库存
		for _, item := range order.Items {
			var sku models.Sku
			if err := tx.Set("gorm:query_option", "FOR UPDATE").First(&sku, item.SkuID).Error; err == nil {
				sku.Stock += item.Quantity
				tx.Save(&sku)
			}
		}

		// 如果是秒杀订单，额外把缓存库存退回 Redis
		for _, item := range order.Items {
			if item.SkuID == 1 { // SKU 1 为秒杀商品
				if core.RedisClient != nil {
					core.RedisClient.IncrBy(context.Background(), "seckill:stock:1", 1)
				}
			}
		}

		tx.Commit()
		if core.RedisClient != nil {
			pushSystemLog(context.Background(), "SUCCESS", fmt.Sprintf("Refund APPROVED for Order %s. Stock restored.", orderID))
		}
		c.JSON(http.StatusOK, gin.H{"status": "success", "message": "退款审核通过，已退款并归还库存"})
	} else if req.Action == "reject" {
		order.Status = 6 // 退款被拒绝
		if err := tx.Save(&order).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"message": "审核状态更新失败"})
			return
		}
		tx.Commit()
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

		// 只有待支付状态 (1) 才能取消
		if order.Status != 1 {
			return nil // 状态已被更改（如已支付），属于正常流程，直接成功
		}

		// 更新为已取消状态 (3)
		order.Status = 3
		if err := tx.Save(&order).Error; err != nil {
			return err
		}

		// 释放锁定的物理库存
		for _, item := range order.Items {
			var sku models.Sku
			if err := tx.Set("gorm:query_option", "FOR UPDATE").First(&sku, item.SkuID).Error; err == nil {
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
		order := models.Order{
			ID:            orderID,
			UserID:        userID,
			TotalAmount:   39900,
			Status:        1,
			ReceiverName:  "秒杀快捷收货",
			ReceiverPhone: "13800000000",
			ReceiverAddr:  "虚拟网络秒杀节点",
		}
		if err := core.DB.Create(&order).Error; err == nil {
			orderItem := models.OrderItem{
				OrderID:  orderID,
				SkuID:    1,
				Price:    39900,
				Quantity: 1,
			}
			core.DB.Create(&orderItem)
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
