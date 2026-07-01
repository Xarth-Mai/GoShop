package handlers

import (
	"net/http"
	"strconv"

	"GoShop/core"
	"GoShop/models"

	"github.com/gin-gonic/gin"
)

type AddressReq struct {
	ID            uint   `json:"id"`
	ReceiverName  string `json:"receiverName" binding:"required"`
	ReceiverPhone string `json:"receiverPhone" binding:"required"`
	Province      string `json:"province" binding:"required"`
	City          string `json:"city" binding:"required"`
	District      string `json:"district" binding:"required"`
	DetailAddress string `json:"detailAddress" binding:"required"`
	IsDefault     bool   `json:"isDefault"`
}

// GetAddresses 获取当前用户的地址列表
// @Summary 获取收货地址
// @Tags user
// @Produce json
// @Success 200 {array} models.Address
// @Router /api/addresses [get]
func GetAddresses(c *gin.Context) {
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

	var addresses []models.Address
	if err := core.ReplicaDB.Where("user_id = ?", userID).Order("is_default desc, id desc").Find(&addresses).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "获取地址列表失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, addresses)
}

// SaveAddress 创建或修改收货地址
// @Summary 保存收货地址
// @Tags user
// @Accept json
// @Produce json
// @Param body body AddressReq true "地址参数"
// @Success 200 {object} map[string]interface{}
// @Router /api/addresses [post]
func SaveAddress(c *gin.Context) {
	userIDVal, exists := c.Get("userId")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "未登录"})
		return
	}
	userID := userIDVal.(uint)

	var req AddressReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "参数解析失败: " + err.Error()})
		return
	}

	if core.DB == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "数据库未就绪"})
		return
	}

	tx := core.DB.Begin()

	// 如果设置了默认，需要将原先默认的地址重置为非默认
	if req.IsDefault {
		if err := tx.Model(&models.Address{}).Where("user_id = ? AND is_default = ?", userID, true).Update("is_default", false).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"message": "重置默认地址失败"})
			return
		}
	}

	if req.ID > 0 {
		// 编辑地址
		var addr models.Address
		if err := tx.Where("id = ? AND user_id = ?", req.ID, userID).First(&addr).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusNotFound, gin.H{"message": "未找到要修改的地址"})
			return
		}

		addr.ReceiverName = models.EncryptedString(req.ReceiverName)
		addr.ReceiverPhone = models.EncryptedString(req.ReceiverPhone)
		addr.Province = req.Province
		addr.City = req.City
		addr.District = req.District
		addr.DetailAddress = models.EncryptedString(req.DetailAddress)
		addr.IsDefault = req.IsDefault

		if err := tx.Save(&addr).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"message": "更新地址失败: " + err.Error()})
			return
		}
	} else {
		// 新增地址
		addr := models.Address{
			UserID:        userID,
			ReceiverName:  models.EncryptedString(req.ReceiverName),
			ReceiverPhone: models.EncryptedString(req.ReceiverPhone),
			Province:      req.Province,
			City:          req.City,
			District:      req.District,
			DetailAddress: models.EncryptedString(req.DetailAddress),
			IsDefault:     req.IsDefault,
		}
		if err := tx.Create(&addr).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"message": "新增地址失败: " + err.Error()})
			return
		}
	}

	tx.Commit()
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "地址保存成功"})
}

// DeleteAddress 删除地址
// @Summary 删除收货地址
// @Tags user
// @Produce json
// @Param id path int true "地址 ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/addresses/{id} [delete]
func DeleteAddress(c *gin.Context) {
	userIDVal, exists := c.Get("userId")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "未登录"})
		return
	}
	userID := userIDVal.(uint)

	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "无效的地址 ID"})
		return
	}

	if core.DB == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "数据库未就绪"})
		return
	}

	res := core.DB.Where("id = ? AND user_id = ?", id, userID).Delete(&models.Address{})
	if res.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "删除地址失败"})
		return
	}
	if res.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"message": "地址不存在或无权操作"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "删除成功"})
}

// SetDefaultAddress 设置默认地址
// @Summary 设置默认地址
// @Tags user
// @Produce json
// @Param id path int true "地址 ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/addresses/{id}/default [put]
func SetDefaultAddress(c *gin.Context) {
	userIDVal, exists := c.Get("userId")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "未登录"})
		return
	}
	userID := userIDVal.(uint)

	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "无效的地址 ID"})
		return
	}

	if core.DB == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "数据库未就绪"})
		return
	}

	tx := core.DB.Begin()

	// 1. 将该用户所有地址设为非默认
	if err := tx.Model(&models.Address{}).Where("user_id = ?", userID).Update("is_default", false).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"message": "重置地址默认状态失败"})
		return
	}

	// 2. 将指定地址设为默认
	res := tx.Model(&models.Address{}).Where("id = ? AND user_id = ?", id, userID).Update("is_default", true)
	if res.Error != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"message": "设置默认地址失败"})
		return
	}
	if res.RowsAffected == 0 {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{"message": "地址不存在"})
		return
	}

	tx.Commit()
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "设置成功"})
}
