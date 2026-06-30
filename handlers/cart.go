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

// GetCart 获取当前用户的云端购物车
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
	if err := core.ReplicaDB.Preload("Sku").Where("user_id = ?", userID).Find(&dbItems).Error; err != nil {
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
		// 查询关联的 SPU 详情
		var spu models.Spu
		if err := core.ReplicaDB.Where("id = ?", item.Sku.SpuID).First(&spu).Error; err != nil {
			continue
		}
		responseList = append(responseList, ResponseCartItem{
			SkuID:    item.SkuID,
			SpuID:    item.Sku.SpuID,
			SpuName:  spu.Name,
			SkuName:  item.Sku.Title,
			Price:    item.Sku.Price,
			Quantity: item.Quantity,
			Image:    spu.MainImage,
		})
	}

	c.JSON(http.StatusOK, responseList)
}

// AddOrUpdateCart 添加或更新云端购物车数量
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

	// 校验 SKU 是否存在
	var sku models.Sku
	if err := core.ReplicaDB.Where("id = ?", req.SkuID).First(&sku).Error; err != nil {
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
