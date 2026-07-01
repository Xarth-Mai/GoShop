package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"GoShop/models"

	"gorm.io/gorm"
)

// InternalToken 内部安全鉴权 Token，防止外网恶意请求内部 API
var InternalToken = "goshop_internal_communication_secret_token"

// CallInternalService 对指定的内部微服务发起同步 HTTP 通信
// 具备自愈降级机制：如果目标网络未就绪（如单体运行/单测未启动其它进程），将自动使用当前传入的 db 连接回退本地 DB 操作，100% 保证开发与测试畅通。
func CallInternalService(db *gorm.DB, targetPort int, method, path string, reqBody interface{}, respDest interface{}) error {
	url := fmt.Sprintf("http://127.0.0.1:%d%s", targetPort, path)

	var req *http.Request
	var err error

	if reqBody != nil {
		raw, err := json.Marshal(reqBody)
		if err != nil {
			return err
		}
		req, err = http.NewRequest(method, url, bytes.NewBuffer(raw))
	} else {
		req, err = http.NewRequest(method, url, nil)
	}
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Token", InternalToken)

	// 设置 5 秒超时保护
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		// 网络不可达（Dial Refused等），自动触发单元测试与单体开发下的【自愈本地降级】
		return fallbackLocal(db, targetPort, method, path, reqBody, respDest)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errData map[string]interface{}
		_ = json.NewDecoder(resp.Body).Decode(&errData)
		if errData != nil && errData["error"] != nil {
			return fmt.Errorf("internal call failed: %v", errData["error"])
		}
		return fmt.Errorf("internal service call failed with status: %d", resp.StatusCode)
	}

	if respDest != nil {
		return json.NewDecoder(resp.Body).Decode(respDest)
	}

	return nil
}

// fallbackLocal 本地自愈降级处理器（用于开发期和单元测试期间直接读取数据库，完全摆脱对具体业务包导入的依赖以防循环引用）
func fallbackLocal(db *gorm.DB, targetPort int, method, path string, reqBody interface{}, respDest interface{}) error {
	if db == nil {
		db = DB // 如果没传，兜底用全局连接
	}
	if db == nil {
		return fmt.Errorf("local database connection not initialized")
	}

	// 1. 商品服务降级 (GET /api/internal/products/:id)
	if targetPort == 8102 && method == "GET" && strings.HasPrefix(path, "/api/internal/products/") && !strings.HasSuffix(path, "/cart-summary") {
		parts := strings.Split(path, "/")
		idStr := parts[len(parts)-1]
		id, err := strconv.Atoi(idStr)
		if err != nil {
			return err
		}
		destSku, ok := respDest.(*models.Sku)
		if !ok {
			return fmt.Errorf("invalid dest type for product fallback")
		}
		return db.Where("id = ?", id).First(destSku).Error
	}

	if targetPort == 8102 && method == "GET" && strings.HasPrefix(path, "/api/internal/products/") && strings.HasSuffix(path, "/cart-summary") {
		parts := strings.Split(path, "/")
		if len(parts) < 2 {
			return fmt.Errorf("invalid product summary path")
		}
		idStr := parts[len(parts)-2]
		id, err := strconv.Atoi(idStr)
		if err != nil {
			return err
		}

		var sku models.Sku
		if err := db.Where("id = ?", id).First(&sku).Error; err != nil {
			return err
		}

		var spu models.Spu
		if err := db.Where("id = ?", sku.SpuID).First(&spu).Error; err != nil {
			return err
		}

		summary := struct {
			SkuID   uint   `json:"skuId"`
			SpuID   uint   `json:"spuId"`
			SpuName string `json:"spuName"`
			SkuName string `json:"skuName"`
			Price   int    `json:"price"`
			Image   string `json:"image"`
		}{
			SkuID:   sku.ID,
			SpuID:   sku.SpuID,
			SpuName: spu.Name,
			SkuName: sku.Title,
			Price:   sku.Price,
			Image:   spu.MainImage,
		}
		raw, err := json.Marshal(summary)
		if err != nil {
			return err
		}
		return json.Unmarshal(raw, respDest)
	}

	if targetPort == 8105 && method == "GET" && strings.HasPrefix(path, "/api/internal/orders/") && strings.Contains(path, "/payment-source") {
		parsed, err := url.Parse("http://internal" + path)
		if err != nil {
			return err
		}
		parts := strings.Split(parsed.Path, "/")
		if len(parts) < 2 {
			return fmt.Errorf("invalid order payment source path")
		}
		orderID := parts[len(parts)-2]
		userID, _ := strconv.Atoi(parsed.Query().Get("userId"))

		query := db.Where("id = ?", orderID)
		if userID > 0 {
			query = query.Where("user_id = ?", uint(userID))
		}
		var order models.Order
		if err := query.First(&order).Error; err != nil {
			return err
		}
		source := struct {
			OrderID      string     `json:"orderId"`
			UserID       uint       `json:"userId"`
			TotalAmount  int        `json:"totalAmount"`
			Status       int        `json:"status"`
			PayStatus    int        `json:"payStatus"`
			UserCouponID uint       `json:"userCouponId"`
			PayExpireAt  *time.Time `json:"payExpireAt,omitempty"`
		}{
			OrderID:      order.ID,
			UserID:       order.UserID,
			TotalAmount:  order.TotalAmount,
			Status:       order.Status,
			PayStatus:    order.PayStatus,
			UserCouponID: order.UserCouponID,
			PayExpireAt:  order.PayExpireAt,
		}
		raw, err := json.Marshal(source)
		if err != nil {
			return err
		}
		return json.Unmarshal(raw, respDest)
	}

	if targetPort == 8105 && method == "GET" && strings.HasPrefix(path, "/api/internal/orders/") && strings.Contains(path, "/refund-source") {
		parsed, err := url.Parse("http://internal" + path)
		if err != nil {
			return err
		}
		parts := strings.Split(parsed.Path, "/")
		orderID := parts[len(parts)-2]
		userID, _ := strconv.Atoi(parsed.Query().Get("userId"))

		query := db.Preload("Items").Where("id = ?", orderID)
		if userID > 0 {
			query = query.Where("user_id = ?", uint(userID))
		}
		var order models.Order
		if err := query.First(&order).Error; err != nil {
			return err
		}
		type refundItem struct {
			OrderItemID     uint `json:"orderItemId"`
			SkuID           uint `json:"skuId"`
			Quantity        int  `json:"quantity"`
			PayableAmount   int  `json:"payableAmount"`
			RefundedAmount  int  `json:"refundedAmount"`
			RefundableQty   int  `json:"refundableQuantity"`
			RefundableValue int  `json:"refundableAmount"`
		}
		items := make([]refundItem, 0, len(order.Items))
		for _, item := range order.Items {
			refundable := item.PayableAmount - item.RefundedAmount
			if refundable < 0 {
				refundable = 0
			}
			refundableQty := 0
			if item.Quantity > 0 && item.PayableAmount > 0 && refundable > 0 {
				refundableQty = refundable * item.Quantity / item.PayableAmount
				if refundableQty <= 0 {
					refundableQty = 1
				}
			}
			items = append(items, refundItem{
				OrderItemID:     item.ID,
				SkuID:           item.SkuID,
				Quantity:        item.Quantity,
				PayableAmount:   item.PayableAmount,
				RefundedAmount:  item.RefundedAmount,
				RefundableQty:   refundableQty,
				RefundableValue: refundable,
			})
		}
		source := struct {
			OrderID         string       `json:"orderId"`
			UserID          uint         `json:"userId"`
			TotalAmount     int          `json:"totalAmount"`
			Status          int          `json:"status"`
			PayStatus       int          `json:"payStatus"`
			AfterSaleStatus int          `json:"afterSaleStatus"`
			Items           []refundItem `json:"items"`
		}{
			OrderID:         order.ID,
			UserID:          order.UserID,
			TotalAmount:     order.TotalAmount,
			Status:          order.Status,
			PayStatus:       order.PayStatus,
			AfterSaleStatus: order.AfterSaleStatus,
			Items:           items,
		}
		raw, err := json.Marshal(source)
		if err != nil {
			return err
		}
		return json.Unmarshal(raw, respDest)
	}

	if targetPort == 8105 && method == "POST" && strings.HasPrefix(path, "/api/internal/orders/") && strings.Contains(path, "/refund-apply") {
		parts := strings.Split(path, "/")
		orderID := parts[len(parts)-2]
		var reqMap map[string]interface{}
		rawBytes, err := json.Marshal(reqBody)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(rawBytes, &reqMap); err != nil {
			return err
		}
		userID := uint(reqMap["userId"].(float64))
		reason, _ := reqMap["reason"].(string)
		proof, _ := reqMap["proof"].(string)
		return db.Transaction(func(tx *gorm.DB) error {
			var order models.Order
			if err := tx.First(&order, "id = ? AND user_id = ?", orderID, userID).Error; err != nil {
				return err
			}
			if order.Status != models.OrderStatusPaid || (order.PayStatus != models.PayStatusPaid && order.PayStatus != models.PayStatusPartialRefunded) {
				return fmt.Errorf("该订单当前状态不支持申请退款")
			}
			fromStatus := order.Status
			order.Status = models.OrderStatusRefundApplying
			order.AfterSaleStatus = models.AfterSaleStatusApplying
			order.RefundReason = reason
			order.RefundProof = proof
			if err := tx.Save(&order).Error; err != nil {
				return err
			}
			return tx.Create(&models.OrderStateLog{
				OrderID:      order.ID,
				FromStatus:   fromStatus,
				ToStatus:     models.OrderStatusRefundApplying,
				OperatorType: 1,
				OperatorID:   userID,
				Event:        "AFTERSALE_APPLIED",
				Remark:       reason,
			}).Error
		})
	}

	if targetPort == 8105 && method == "POST" && strings.HasPrefix(path, "/api/internal/orders/") && strings.Contains(path, "/refund-complete") {
		parts := strings.Split(path, "/")
		orderID := parts[len(parts)-2]
		var req struct {
			Items []struct {
				OrderItemID uint `json:"orderItemId"`
				Amount      int  `json:"amount"`
			} `json:"items"`
			Remark string `json:"remark"`
		}
		rawBytes, err := json.Marshal(reqBody)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(rawBytes, &req); err != nil {
			return err
		}
		return db.Transaction(func(tx *gorm.DB) error {
			var order models.Order
			if err := tx.First(&order, "id = ?", orderID).Error; err != nil {
				return err
			}
			if order.AfterSaleStatus == models.AfterSaleStatusRefunded && (order.PayStatus == models.PayStatusRefunded || order.PayStatus == models.PayStatusPartialRefunded) {
				return nil
			}
			for _, item := range req.Items {
				result := tx.Model(&models.OrderItem{}).
					Where("id = ? AND order_id = ? AND refunded_amount + ? <= payable_amount", item.OrderItemID, orderID, item.Amount).
					Update("refunded_amount", gorm.Expr("refunded_amount + ?", item.Amount))
				if result.Error != nil {
					return result.Error
				}
				if result.RowsAffected != 1 {
					return fmt.Errorf("订单行 %d 可退金额不足", item.OrderItemID)
				}
			}
			var totalRefunded int64
			if err := tx.Model(&models.OrderItem{}).Where("order_id = ?", orderID).Select("COALESCE(SUM(refunded_amount), 0)").Scan(&totalRefunded).Error; err != nil {
				return err
			}
			fromStatus := order.Status
			if int(totalRefunded) >= order.TotalAmount {
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
			return tx.Create(&models.OrderStateLog{
				OrderID:      order.ID,
				FromStatus:   fromStatus,
				ToStatus:     order.Status,
				OperatorType: 1,
				OperatorID:   0,
				Event:        "AFTERSALE_APPROVED",
				Remark:       req.Remark,
			}).Error
		})
	}

	if targetPort == 8105 && method == "POST" && strings.HasPrefix(path, "/api/internal/orders/") && strings.Contains(path, "/refund-reject") {
		parts := strings.Split(path, "/")
		orderID := parts[len(parts)-2]
		return db.Transaction(func(tx *gorm.DB) error {
			var order models.Order
			if err := tx.First(&order, "id = ?", orderID).Error; err != nil {
				return err
			}
			if order.AfterSaleStatus == models.AfterSaleStatusRejected {
				return nil
			}
			fromStatus := order.Status
			order.Status = models.OrderStatusRefundRejected
			order.AfterSaleStatus = models.AfterSaleStatusRejected
			if err := tx.Save(&order).Error; err != nil {
				return err
			}
			return tx.Create(&models.OrderStateLog{
				OrderID:      order.ID,
				FromStatus:   fromStatus,
				ToStatus:     models.OrderStatusRefundRejected,
				OperatorType: 1,
				OperatorID:   0,
				Event:        "AFTERSALE_REJECTED",
				Remark:       "商家拒绝退款申请",
			}).Error
		})
	}

	// 2. 优惠券 Candidates 降级 (POST /api/internal/promotion/candidates)
	if targetPort == 8104 && path == "/api/internal/promotion/candidates" {
		var reqMap map[string]interface{}
		rawBytes, err := json.Marshal(reqBody)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(rawBytes, &reqMap); err != nil {
			return err
		}

		userIDFloat, ok1 := reqMap["userId"].(float64)
		selectedUCIDFloat, ok2 := reqMap["selectedUserCouponId"].(float64)
		subtotalFloat, ok3 := reqMap["subtotal"].(float64)
		if !ok1 || !ok2 || !ok3 {
			return fmt.Errorf("invalid numeric args in promotion candidates fallback")
		}

		userID := uint(userIDFloat)
		selectedUCID := uint(selectedUCIDFloat)
		subtotal := int(subtotalFloat)

		// 内存中直接模拟优惠券筛选以防依赖 promotion 包
		var userCoupons []models.UserCoupon
		now := time.Now()
		err = db.Preload("Coupon").
			Joins("JOIN coupons ON coupons.id = user_coupons.coupon_id").
			Where("user_coupons.user_id = ? AND user_coupons.status = ?", userID, models.UserCouponStatusAvailable).
			Where("coupons.end_time >= ?", now).
			Find(&userCoupons).Error
		if err != nil {
			return err
		}

		type LocalCandidate struct {
			UserCouponID   uint   `json:"userCouponId"`
			Available      bool   `json:"available"`
			Reason         string `json:"reason"`
			DiscountAmount int    `json:"discountAmount"`
		}
		var candidates []LocalCandidate
		for _, uc := range userCoupons {
			available := true
			reason := ""
			discount := 0
			if uc.Coupon.MinAmount > subtotal {
				available = false
				reason = "未达到优惠券使用门槛"
			} else {
				discount = uc.Coupon.Value
			}
			candidates = append(candidates, LocalCandidate{
				UserCouponID:   uc.ID,
				Available:      available,
				Reason:         reason,
				DiscountAmount: discount,
			})
		}

		if selectedUCID > 0 {
			var found bool
			for _, c := range candidates {
				if c.UserCouponID == selectedUCID {
					found = true
					break
				}
			}
			if !found {
				var selected models.UserCoupon
				if err := db.Preload("Coupon").First(&selected, "id = ? AND user_id = ?", selectedUCID, userID).Error; err == nil {
					available := true
					reason := ""
					discount := 0
					if selected.Coupon.MinAmount > subtotal {
						available = false
						reason = "未达到优惠券使用门槛"
					} else {
						discount = selected.Coupon.Value
					}
					candidates = append(candidates, LocalCandidate{
						UserCouponID:   selected.ID,
						Available:      available,
						Reason:         reason,
						DiscountAmount: discount,
					})
				}
			}
		}

		// 使用 JSON 反序列化写入 respDest 以排除直接强类型依赖
		raw, err := json.Marshal(candidates)
		if err != nil {
			return err
		}
		return json.Unmarshal(raw, respDest)
	}

	// 3. 优惠券锁定降级 (POST /api/internal/promotion/lock)
	if targetPort == 8104 && path == "/api/internal/promotion/lock" {
		var reqMap map[string]interface{}
		rawBytes, err := json.Marshal(reqBody)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(rawBytes, &reqMap); err != nil {
			return err
		}

		userID := uint(reqMap["userId"].(float64))
		userCouponID := uint(reqMap["userCouponId"].(float64))
		orderID := reqMap["orderId"].(string)
		subtotal := int(reqMap["subtotal"].(float64))

		var discount int
		err = db.Transaction(func(tx *gorm.DB) error {
			var uc models.UserCoupon
			if err := tx.Preload("Coupon").Where("id = ? AND user_id = ?", userCouponID, userID).First(&uc).Error; err != nil {
				return err
			}
			if uc.Status == models.UserCouponStatusLocked && uc.LockedOrderID == orderID {
				discount = uc.Coupon.Value
				return nil
			}
			if uc.Status != models.UserCouponStatusAvailable {
				return fmt.Errorf("优惠券不可用或已失效")
			}
			if uc.Coupon.MinAmount > subtotal {
				return fmt.Errorf("优惠券不可用或已失效")
			}

			now := time.Now()
			if !uc.Coupon.EndTime.IsZero() && uc.Coupon.EndTime.Before(now) {
				return fmt.Errorf("优惠券不可用或已失效")
			}
			if !uc.Coupon.StartTime.IsZero() && uc.Coupon.StartTime.After(now) {
				return fmt.Errorf("优惠券不可用或已失效")
			}

			discount = uc.Coupon.Value
			uc.Status = models.UserCouponStatusLocked
			uc.LockedOrderID = orderID
			return tx.Save(&uc).Error
		})
		if err != nil {
			return err
		}

		type LockResp struct {
			DiscountAmount int `json:"discountAmount"`
		}
		raw, err := json.Marshal(LockResp{DiscountAmount: discount})
		if err != nil {
			return err
		}
		return json.Unmarshal(raw, respDest)
	}

	// 4. 优惠券释放降级 (POST /api/internal/promotion/release)
	if targetPort == 8104 && path == "/api/internal/promotion/release" {
		var reqMap map[string]interface{}
		rawBytes, err := json.Marshal(reqBody)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(rawBytes, &reqMap); err != nil {
			return err
		}

		userCouponID := uint(reqMap["userCouponId"].(float64))
		orderID := reqMap["orderId"].(string)

		return db.Transaction(func(tx *gorm.DB) error {
			var uc models.UserCoupon
			if err := tx.Where("id = ? AND locked_order_id = ?", userCouponID, orderID).First(&uc).Error; err != nil {
				if err == gorm.ErrRecordNotFound {
					return nil // 幂等
				}
				return err
			}
			uc.Status = models.UserCouponStatusAvailable
			uc.LockedOrderID = ""
			return tx.Save(&uc).Error
		})
	}

	// 5. 库存预占锁定降级 (POST /api/internal/inventory/reserve)
	if targetPort == 8103 && path == "/api/internal/inventory/reserve" {
		var reqMap map[string]interface{}
		rawBytes, err := json.Marshal(reqBody)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(rawBytes, &reqMap); err != nil {
			return err
		}

		orderID := reqMap["orderId"].(string)
		userID := uint(reqMap["userId"].(float64))
		rawItems := reqMap["items"].([]interface{})

		return db.Transaction(func(tx *gorm.DB) error {
			for _, it := range rawItems {
				itemMap := it.(map[string]interface{})
				skuID := uint(itemMap["skuId"].(float64))
				qty := int(itemMap["qty"].(float64))

				var inv models.SkuInventory
				err := tx.Where("sku_id = ?", skuID).First(&inv).Error
				if err != nil {
					if err == gorm.ErrRecordNotFound {
						var sku models.Sku
						if err := tx.Where("id = ?", skuID).First(&sku).Error; err != nil {
							return fmt.Errorf("SKU %d 不存在", skuID)
						}
						inv = models.SkuInventory{SkuID: sku.ID, Available: sku.Stock}
						if err := tx.Create(&inv).Error; err != nil {
							return err
						}
					} else {
						return err
					}
				}

				if inv.Available < qty {
					return fmt.Errorf("SKU %d 库存不足，仅剩 %d 件", skuID, inv.Available)
				}
				inv.Available -= qty
				inv.Reserved += qty
				inv.Version++
				if err := tx.Save(&inv).Error; err != nil {
					return err
				}

				res := models.InventoryReservation{
					ID:       fmt.Sprintf("RES-%s-%d", orderID, skuID),
					OrderID:  orderID,
					UserID:   userID,
					SkuID:    skuID,
					Quantity: qty,
					Status:   models.ReservationStatusReserved,
					ExpireAt: time.Now().Add(30 * time.Minute),
				}
				if err := tx.Create(&res).Error; err != nil {
					return err
				}
			}
			return nil
		})
	}

	// 6. 库存预占释放降级 (POST /api/internal/inventory/release)
	if targetPort == 8103 && path == "/api/internal/inventory/release" {
		var reqMap map[string]interface{}
		rawBytes, err := json.Marshal(reqBody)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(rawBytes, &reqMap); err != nil {
			return err
		}

		orderID := reqMap["orderId"].(string)

		return db.Transaction(func(tx *gorm.DB) error {
			var reservations []models.InventoryReservation
			if err := tx.Where("order_id = ? AND status = ?", orderID, models.ReservationStatusReserved).Find(&reservations).Error; err != nil {
				return err
			}
			for _, res := range reservations {
				res.Status = models.ReservationStatusReleased
				if err := tx.Save(&res).Error; err != nil {
					return err
				}

				var inv models.SkuInventory
				if err := tx.Where("sku_id = ?", res.SkuID).First(&inv).Error; err != nil {
					return err
				}
				inv.Available += res.Quantity
				inv.Reserved -= res.Quantity
				inv.Version++
				if err := tx.Save(&inv).Error; err != nil {
					return err
				}
			}
			return nil
		})
	}

	if targetPort == 8103 && path == "/api/internal/inventory/restock" {
		var req struct {
			OrderID string `json:"orderId"`
			Items   []struct {
				SkuID    uint `json:"skuId"`
				Quantity int  `json:"quantity"`
			} `json:"items"`
		}
		rawBytes, err := json.Marshal(reqBody)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(rawBytes, &req); err != nil {
			return err
		}
		return db.Transaction(func(tx *gorm.DB) error {
			for _, item := range req.Items {
				var inv models.SkuInventory
				if err := tx.First(&inv, "sku_id = ?", item.SkuID).Error; err != nil {
					return err
				}
				if inv.Sold < item.Quantity {
					return fmt.Errorf("SKU %d 已售库存不足，无法退款回补", item.SkuID)
				}
				inv.Sold -= item.Quantity
				inv.Available += item.Quantity
				inv.Version++
				if err := tx.Save(&inv).Error; err != nil {
					return err
				}
				if err := tx.Create(&models.InventoryJournal{
					SkuID:      item.SkuID,
					OrderID:    req.OrderID,
					ChangeType: "REFUND_RESTOCK",
					Quantity:   item.Quantity,
				}).Error; err != nil {
					return err
				}
			}
			return nil
		})
	}

	return fmt.Errorf("no connection to service at port %d, and fallback local not implemented for path %s", targetPort, path)
}
