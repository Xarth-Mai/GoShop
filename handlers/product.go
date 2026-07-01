package handlers

import (
	"net/http"
	"strconv"

	"GoShop/core"
	"GoShop/models"

	"github.com/gin-gonic/gin"
)

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

	c.JSON(http.StatusOK, product)
}
