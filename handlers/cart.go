package handlers

import (
	"net/http"
	"strconv"

	"GoShop/core"
	"GoShop/models"

	"github.com/gin-gonic/gin"
)

type CartSaveReq struct {
	SkuID    uint `json:"skuId" binding:"required"`
	Quantity int  `json:"quantity" binding:"required"`
}

type SyncItem struct {
	SkuID    uint `json:"skuId" binding:"required"`
	Quantity int  `json:"quantity" binding:"required"`
}

type CartSyncReq struct {
	Items []SyncItem `json:"items" binding:"required"`
}

type cartProductSummary struct {
	SkuID   uint   `json:"skuId"`
	SpuID   uint   `json:"spuId"`
	SpuName string `json:"spuName"`
	SkuName string `json:"skuName"`
	Price   int    `json:"price"`
	Image   string `json:"image"`
}

func fetchCartProductSummary(skuID uint) (cartProductSummary, error) {
	var summary cartProductSummary
	err := core.CallInternalService(
		core.DB,
		8102,
		http.MethodGet,
		"/api/internal/products/"+strconv.Itoa(int(skuID))+"/cart-summary",
		nil,
		&summary,
	)
	return summary, err
}

// GetCart 获取当前用户的云端购物车
// @Summary 获取购物车
// @Tags cart
// @Produce json
// @Success 200 {array} map[string]interface{}
// @Router /api/cart [get]
func GetCart(c *gin.Context) {
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

	var dbItems []models.CartItem
	if err := core.ReplicaDB.Where("user_id = ?", userID).Find(&dbItems).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "获取购物车失败: " + err.Error()})
		return
	}

	// 组装成前端熟悉的 CartItem 格式
	type ResponseCartItem struct {
		SkuID    uint   `json:"skuId"`
		SpuID    uint   `json:"spuId"`
		SpuName  string `json:"spuName"`
		SkuName  string `json:"skuName"`
		Price    int    `json:"price"`
		Quantity int    `json:"quantity"`
		Image    string `json:"image"`
	}

	var responseList []ResponseCartItem
	for _, item := range dbItems {
		summary, err := fetchCartProductSummary(item.SkuID)
		if err != nil {
			continue
		}
		responseList = append(responseList, ResponseCartItem{
			SkuID:    item.SkuID,
			SpuID:    summary.SpuID,
			SpuName:  summary.SpuName,
			SkuName:  summary.SkuName,
			Price:    summary.Price,
			Quantity: item.Quantity,
			Image:    summary.Image,
		})
	}

	c.JSON(http.StatusOK, responseList)
}

// AddOrUpdateCart 添加或更新云端购物车数量
// @Summary 添加或更新购物车
// @Tags cart
// @Accept json
// @Produce json
// @Param body body CartSaveReq true "购物车项"
// @Success 200 {object} map[string]interface{}
// @Router /api/cart [post]
func AddOrUpdateCart(c *gin.Context) {
	userIDVal, exists := c.Get("userId")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "未登录"})
		return
	}
	userID := userIDVal.(uint)

	var req CartSaveReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "参数错误"})
		return
	}

	if core.DB == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "数据库未就绪"})
		return
	}

	if _, err := fetchCartProductSummary(req.SkuID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "商品规格不存在"})
		return
	}

	var cartItem models.CartItem
	err := core.DB.Where("user_id = ? AND sku_id = ?", userID, req.SkuID).First(&cartItem).Error
	if err == nil {
		// 已存在则覆盖更新数量
		cartItem.Quantity = req.Quantity
		if cartItem.Quantity < 1 {
			cartItem.Quantity = 1
		}
		core.DB.Save(&cartItem)
	} else {
		// 不存在则创建
		newQty := req.Quantity
		if newQty < 1 {
			newQty = 1
		}
		cartItem = models.CartItem{
			UserID:   userID,
			SkuID:    req.SkuID,
			Quantity: newQty,
		}
		if err := core.DB.Create(&cartItem).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "加入购物车失败: " + err.Error()})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "云端购物车更新成功"})
}

// RemoveFromCart 移除购物车中某项
// @Summary 删除购物车项
// @Tags cart
// @Produce json
// @Param skuId path int true "SKU ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/cart/{skuId} [delete]
func RemoveFromCart(c *gin.Context) {
	userIDVal, exists := c.Get("userId")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "未登录"})
		return
	}
	userID := userIDVal.(uint)

	skuIdStr := c.Param("skuId")
	skuID, err := strconv.Atoi(skuIdStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "无效的商品规格 ID"})
		return
	}

	if core.DB == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "数据库未就绪"})
		return
	}

	core.DB.Where("user_id = ? AND sku_id = ?", userID, skuID).Delete(&models.CartItem{})
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "删除成功"})
}

// SyncCart 批量同步本地购物车至云端
// @Summary 同步购物车
// @Tags cart
// @Accept json
// @Produce json
// @Param body body CartSyncReq true "同步项"
// @Success 200 {object} map[string]interface{}
// @Router /api/cart/sync [post]
func SyncCart(c *gin.Context) {
	userIDVal, exists := c.Get("userId")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "未登录"})
		return
	}
	userID := userIDVal.(uint)

	var req CartSyncReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "参数错误"})
		return
	}

	if core.DB == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "数据库未就绪"})
		return
	}

	for _, item := range req.Items {
		if _, err := fetchCartProductSummary(item.SkuID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": "商品规格不存在"})
			return
		}
	}

	tx := core.DB.Begin()
	for _, item := range req.Items {
		var cartItem models.CartItem
		err := tx.Where("user_id = ? AND sku_id = ?", userID, item.SkuID).First(&cartItem).Error
		if err == nil {
			// 若存在，合并累加数量
			cartItem.Quantity += item.Quantity
			tx.Save(&cartItem)
		} else {
			// 若不存在，新建
			tx.Create(&models.CartItem{
				UserID:   userID,
				SkuID:    item.SkuID,
				Quantity: item.Quantity,
			})
		}
	}
	tx.Commit()

	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "云端同步合并完成"})
}
