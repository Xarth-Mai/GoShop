package handlers

import (
	"fmt"
	"net/http"
	"strconv"

	"GoShop/core"
	"GoShop/models"

	"github.com/gin-gonic/gin"
)

// GetCategories 获取商品分类列表
// @Summary 获取商品分类
// @Tags product
// @Produce json
// @Success 200 {array} models.Category
// @Router /api/categories [get]
func GetCategories(c *gin.Context) {
	var categories []models.Category
	if core.DB == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "数据库未就绪"})
		return
	}
	if err := core.ReplicaDB.Order("sort_order asc").Find(&categories).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, categories)
}

// GetProducts 分页查询商品
// @Summary 分页查询商品
// @Tags product
// @Produce json
// @Param categoryId query int false "分类 ID"
// @Param keyword query string false "搜索关键词"
// @Param page query int false "页码"
// @Param pageSize query int false "每页数量"
// @Success 200 {object} map[string]interface{}
// @Router /api/products [get]
func GetProducts(c *gin.Context) {
	if core.DB == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "数据库未就绪"})
		return
	}
	categoryIDStr := c.Query("categoryId")
	keyword := c.Query("keyword")
	pageStr := c.DefaultQuery("page", "1")
	pageSizeStr := c.DefaultQuery("pageSize", "10")

	page, _ := strconv.Atoi(pageStr)
	pageSize, _ := strconv.Atoi(pageSizeStr)
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}

	query := core.ReplicaDB.Model(&models.Spu{}).Where("status = ?", 1)
	if categoryIDStr != "" {
		categoryID, err := strconv.Atoi(categoryIDStr)
		if err == nil && categoryID > 0 {
			query = query.Where("category_id = ?", categoryID)
		}
	}
	if keyword != "" {
		query = query.Where("name LIKE ? OR subtitle LIKE ?", "%"+keyword+"%", "%"+keyword+"%")
	}

	var total int64
	query.Count(&total)

	var products []models.Spu
	offset := (page - 1) * pageSize
	if err := query.Offset(offset).Limit(pageSize).Find(&products).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"total":    total,
		"page":     page,
		"pageSize": pageSize,
		"data":     products,
	})
}

// GetProduct 查询商品详情
// @Summary 查询商品详情
// @Tags product
// @Produce json
// @Param id path int true "SPU ID"
// @Success 200 {object} models.Spu
// @Failure 404 {object} map[string]interface{}
// @Router /api/products/{id} [get]
func GetProduct(c *gin.Context) {
	if core.DB == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "数据库未就绪"})
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "无效的商品ID"})
		return
	}

	var product models.Spu
	if err := core.ReplicaDB.Preload("Skus").First(&product, "id = ? AND status = ?", id, 1).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "商品未找到"})
		return
	}

	for i := range product.Skus {
		var stockResp struct {
			Available int `json:"available"`
		}
		if err := core.CallInternalService(core.ReplicaDB, 8103, "GET", fmt.Sprintf("/api/internal/inventory/skus/%d", product.Skus[i].ID), nil, &stockResp); err == nil {
			product.Skus[i].Stock = stockResp.Available
		}
	}

	c.JSON(http.StatusOK, product)
}
