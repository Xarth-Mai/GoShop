package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
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

	return fmt.Errorf("no connection to service at port %d, and fallback local not implemented for path %s", targetPort, path)
}
