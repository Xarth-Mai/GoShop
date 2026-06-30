package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"GoShop/config"
	"GoShop/core"
	_ "GoShop/docs"
	"GoShop/models"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

type Order struct {
	ID          string    `gorm:"primaryKey;column:id" json:"id"`
	UserID      int       `gorm:"column:user_id" json:"userId"`
	TotalAmount int       `gorm:"column:total_amount" json:"totalAmount"`
	Status      int       `gorm:"column:status" json:"status"` // 1: 待支付, 2: 已支付, 3: 已取消
	CreatedAt   time.Time `gorm:"column:created_at" json:"createdAt"`
	UpdatedAt   time.Time `gorm:"column:updated_at" json:"updatedAt"`
}

type OrderItem struct {
	ID        int       `gorm:"primaryKey;column:id;autoIncrement" json:"id"`
	OrderID   string    `gorm:"column:order_id" json:"orderId"`
	SkuID     int       `gorm:"column:sku_id" json:"skuId"`
	Price     int       `gorm:"column:price" json:"price"`
	Quantity  int       `gorm:"column:quantity" json:"quantity"`
	CreatedAt time.Time `gorm:"column:created_at" json:"createdAt"`
	UpdatedAt time.Time `gorm:"column:updated_at" json:"updatedAt"`
}

type LogItem struct {
	Time string `json:"time"`
	Type string `json:"type"`
	Msg  string `json:"msg"`
}

type PayRequest struct {
	OrderID string `json:"orderId" binding:"required"`
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

func pushLog(ctx context.Context, logType, msg string) {
	if core.RedisClient == nil {
		return
	}
	now := time.Now().Format("15:04:05")
	item := LogItem{Time: now, Type: logType, Msg: msg}
	bytes, _ := json.Marshal(item)
	core.RedisClient.LPush(ctx, "seckill:logs", string(bytes))
	core.RedisClient.LTrim(ctx, "seckill:logs", 0, 9)
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

func startDelayQueueWorker() {
	ticker := time.NewTicker(1 * time.Second)
	ctx := context.Background()
	log.Println("[INFO] 延迟队列 Worker 已启动...")

	for range ticker.C {
		if core.RedisClient == nil || core.DB == nil {
			continue
		}

		now := time.Now().Unix()
		// 获取超时的订单号 (Score <= now)
		orders, err := core.RedisClient.ZRangeByScore(ctx, "seckill:delay_queue", &redis.ZRangeBy{
			Min: "-inf",
			Max: fmt.Sprintf("%d", now),
		}).Result()

		if err != nil || len(orders) == 0 {
			continue
		}

		for _, orderID := range orders {
			// 使用事务在数据库中取消待支付订单
			tx := core.DB.Begin()
			var order Order
			if err := tx.First(&order, "id = ?", orderID).Error; err == nil {
				if order.Status == 1 { // 只有处于待支付状态才回收库存并取消订单
					if err := tx.Model(&order).Update("status", 3).Error; err == nil {
						tx.Commit()

						// 回退 Redis 中的缓存库存 (增加 1)
						core.RedisClient.IncrBy(ctx, "seckill:stock:1", 1)
						pushLog(ctx, "WARN", fmt.Sprintf("Delay worker: Order %s EXPIRED. Cancelled and rolled back stock.", orderID))
					} else {
						tx.Rollback()
					}
				} else {
					tx.Commit()
				}
			} else {
				tx.Rollback()
			}

			// 从 ZSet 延迟队列中移除任务
			core.RedisClient.ZRem(ctx, "seckill:delay_queue", orderID)
		}
	}
}

// @title           GoShop API
// @version         1.0
// @description     一个基于 Go 语言构建的轻量级、高性能通用电商后端系统。
// @host            localhost:3233
// @BasePath        /
func main() {
	// 1. 初始化配置
	configPath := "config.yaml"
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		log.Fatalf("配置文件 %s 不存在，请先复制 config.example.yaml 为 config.yaml 并根据本地环境调整配置", configPath)
	}

	err := config.InitConfig(configPath)
	if err != nil {
		log.Fatalf("解析配置文件失败: %v", err)
	}

	cfg := config.GlobalConfig

	// 设置 Gin 运行模式
	gin.SetMode(cfg.Server.Mode)

	// 2. 初始化数据库 (PostgreSQL)
	log.Println("正在连接 PostgreSQL 数据库...")
	err = core.InitDB()
	if err != nil {
		log.Printf("[警告] 连接 PostgreSQL 数据库失败: %v", err)
		log.Println("[警告] 系统将处于数据库未就绪状态运行。")
	} else {
		log.Println("PostgreSQL 数据库连接成功 (读写分离就绪).")
		// 数据库自动迁移
		err = core.DB.AutoMigrate(&models.Category{}, &models.Spu{}, &models.Sku{}, &Order{}, &OrderItem{})
		if err != nil {
			log.Printf("[错误] 数据库自动迁移失败: %v", err)
		} else {
			log.Println("数据库迁移/更新成功.")
			// 测试数据种子化
			if err := models.SeedProducts(core.DB); err != nil {
				log.Printf("[错误] 数据种子初始化失败: %v", err)
			} else {
				log.Println("数据库初始商品数据对齐完成.")
			}
		}
	}

	// 3. 初始化 Redis (Valkey)
	log.Println("正在连接 Redis/Valkey...")
	err = core.InitRedis()
	if err != nil {
		log.Printf("[警告] 连接 Redis/Valkey 失败: %v", err)
		log.Println("[警告] 系统将处于缓存/队列未就绪状态运行。")
	} else {
		log.Println("Redis/Valkey 连接成功.")
	}

	// 4. 初始化 Gin 引擎
	r := gin.Default()

	// 启动延迟队列后台 Worker
	go startDelayQueueWorker()

	// 5. 静态资源托管
	r.Static("/assets", "./web/dist/assets")
	r.StaticFile("/favicon.ico", "./web/dist/favicon.ico")
	r.StaticFile("/", "./web/dist/index.html")

	// 5.1 Swagger API 文档
	r.GET("/swagger", func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, "/swagger/index.html")
	})
	r.GET("/swagger/*any", func(c *gin.Context) {
		path := c.Param("any")
		if path == "" || path == "/" {
			c.Redirect(http.StatusMovedPermanently, "/swagger/index.html")
			return
		}
		ginSwagger.WrapHandler(swaggerFiles.Handler)(c)
	})

	// SPA Routing Fallback
	r.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.JSON(http.StatusNotFound, gin.H{"message": "API not found"})
			return
		}
		c.File("./web/dist/index.html")
	})

	// 6. 注册基础路由与健康检查
	r.GET("/health", func(c *gin.Context) {
		dbStatus := "OK"
		if core.DB == nil {
			dbStatus = "DISCONNECTED"
		} else {
			sqlDB, err := core.DB.DB()
			if err != nil || sqlDB.Ping() != nil {
				dbStatus = "ERROR"
			}
		}

		redisStatus := "OK"
		if core.RedisClient == nil || core.RedisClient.Ping(context.Background()).Err() != nil {
			redisStatus = "DISCONNECTED"
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "UP",
			"components": gin.H{
				"database": dbStatus,
				"redis":    redisStatus,
			},
		})
	})

	// 7. API 对接路由
	// @Summary      Get monitoring metrics and logs
	// @Description  Retrieve Valkey pre-decrement stock metrics, lock count, and real-time logs
	// @Tags         metrics
	// @Produce      json
	// @Success      200  {object}  map[string]interface{}
	// @Router       /api/metrics [get]
	r.GET("/api/metrics", func(c *gin.Context) {
		ctx := context.Background()

		// 1. 获取库存
		stockStr, err := core.RedisClient.Get(ctx, "seckill:stock:1").Result()
		if err != nil {
			core.RedisClient.Set(ctx, "seckill:stock:1", 87, 0)
			stockStr = "87"
			pushLog(ctx, "INFO", "Valkey stock cache initialized. Stock: 87")
		}
		stock, _ := strconv.Atoi(stockStr)

		// 2. 获取锁定库存数
		lockStock, _ := core.RedisClient.ZCard(ctx, "seckill:delay_queue").Result()

		// 3. 获取已支付订单数与销售额
		var ordersPaid int64 = 0
		var totalRevenueCent int64 = 0

		if core.DB != nil {
			core.DB.Model(&Order{}).Where("status = ?", 2).Count(&ordersPaid)
			core.DB.Model(&Order{}).Where("status = ?", 2).Select("COALESCE(SUM(total_amount), 0)").Row().Scan(&totalRevenueCent)
		}

		revenueVal := float64(totalRevenueCent) / 100.0
		revenueStr := formatCurrency(revenueVal)

		// 4. 获取日志
		var logs []LogItem
		logStrs, _ := core.RedisClient.LRange(ctx, "seckill:logs", 0, -1).Result()
		for _, s := range logStrs {
			var item LogItem
			if err := json.Unmarshal([]byte(s), &item); err == nil {
				logs = append(logs, item)
			}
		}

		// 5. 获取待支付订单列表 (status = 1)
		var pendingOrders []Order
		if core.DB != nil {
			core.DB.Where("status = ?", 1).Order("created_at desc").Find(&pendingOrders)
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
	})

	// @Summary      Get all categories
	// @Description  Retrieve multi-level product categories list
	// @Tags         products
	// @Produce      json
	// @Success      200  {array}   models.Category
	// @Router       /api/categories [get]
	r.GET("/api/categories", func(c *gin.Context) {
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
	})

	// @Summary      Get products list
	// @Description  Retrieve products list with category filtering, paging, and sorting
	// @Tags         products
	// @Produce      json
	// @Param        categoryId  query  int  false  "Category ID filter"
	// @Param        keyword     query  string  false  "Keyword search"
	// @Param        page        query  int  false  "Page number (default 1)"
	// @Param        pageSize    query  int  false  "Page size (default 10)"
	// @Success      200  {array}   models.Spu
	// @Router       /api/products [get]
	r.GET("/api/products", func(c *gin.Context) {
		if core.DB == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "数据库未就绪"})
			return
		}
		categoryIdStr := c.Query("categoryId")
		keyword := c.Query("keyword")
		pageStr := c.DefaultQuery("page", "1")
		pageSizeStr := c.DefaultQuery("pageSize", "10")

		page, _ := strconv.Atoi(pageStr)
		pageSize, _ := strconv.Atoi(pageSizeStr)
		if page < 1 { page = 1 }
		if pageSize < 1 { pageSize = 10 }

		query := core.ReplicaDB.Model(&models.Spu{}).Where("status = ?", 1)

		if categoryIdStr != "" {
			categoryId, err := strconv.Atoi(categoryIdStr)
			if err == nil && categoryId > 0 {
				query = query.Where("category_id = ?", categoryId)
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
	})

	// @Summary      Get product detail
	// @Description  Retrieve single product details and its associated SKUs list
	// @Tags         products
	// @Produce      json
	// @Param        id   path  int  true  "Product SPU ID"
	// @Success      200  {object}  models.Spu
	// @Router       /api/products/{id} [get]
	r.GET("/api/products/:id", func(c *gin.Context) {
		if core.DB == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "数据库未就绪"})
			return
		}
		idStr := c.Param("id")
		id, err := strconv.Atoi(idStr)
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
	})

	// @Summary      Perform a high-concurrency seckill order creation
	// @Description  Deduct Valkey cache stock and create an order in GORM PostgreSQL with status=1 (pending payment)
	// @Tags         seckill
	// @Produce      json
	// @Success      200  {object}  map[string]interface{}
	// @Failure      400  {object}  map[string]interface{}
	// @Router       /api/seckill [post]
	r.POST("/api/seckill", func(c *gin.Context) {
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
			core.RedisClient.Set(ctx, "seckill:stock:1", 87, 0)
			res, _ = core.RedisClient.Eval(ctx, seckillLua, []string{"seckill:stock:1"}, 1).Result()
			statusVal, _ = res.(int64)
		}

		if statusVal == 0 {
			pushLog(ctx, "ERROR", "Seckill failed. Valkey cache stock is 0.")
			c.JSON(http.StatusBadRequest, gin.H{"message": "库存不足"})
			return
		}

		// 2. 获取剩余库存
		leftStock, _ := core.RedisClient.Get(ctx, "seckill:stock:1").Result()

		// 3. 生成订单并写入数据库 (status=1, 待支付)
		orderID := fmt.Sprintf("GS-%d-%d", time.Now().Unix(), time.Now().Nanosecond()%1000)

		if core.DB != nil {
			order := Order{
				ID:          orderID,
				UserID:      1,
				TotalAmount: 39900, // 399.00元
				Status:      1,
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			}
			if err := core.DB.Create(&order).Error; err == nil {
				orderItem := OrderItem{
					OrderID:   orderID,
					SkuID:     1,
					Price:     39900,
					Quantity:  1,
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				}
				core.DB.Create(&orderItem)
			}
		}

		// 4. 加入延迟队列 (15秒后超时，为演示起见，方便查看)
		now := time.Now().Unix()
		core.RedisClient.ZAdd(ctx, "seckill:delay_queue", redis.Z{
			Score:  float64(now + 15),
			Member: orderID,
		})

		pushLog(ctx, "INFO", fmt.Sprintf("Valkey Lua pre-decrement SUCCESS. Stock left: %s. Order %s created. Please pay in 15 seconds.", leftStock, orderID))

		c.JSON(http.StatusOK, gin.H{
			"status":  "success",
			"orderId": orderID,
		})
	})

	// @Summary      Pay a seckill order
	// @Description  Confirm payment for a pending order, remove from delay queue, and mark as paid
	// @Tags         pay
	// @Accept       json
	// @Produce      json
	// @Param        request  body  PayRequest  true  "Order ID to pay"
	// @Success      200  {object}  map[string]interface{}
	// @Failure      400  {object}  map[string]interface{}
	// @Router       /api/pay [post]
	r.POST("/api/pay", func(c *gin.Context) {
		var req PayRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": "参数错误"})
			return
		}

		ctx := context.Background()
		if core.DB == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "数据库未就绪"})
			return
		}

		tx := core.DB.Begin()
		var order Order
		if err := tx.First(&order, "id = ?", req.OrderID).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusNotFound, gin.H{"message": "订单未找到"})
			return
		}

		if order.Status != 1 {
			tx.Rollback()
			c.JSON(http.StatusBadRequest, gin.H{"message": "订单状态不正确，无法支付"})
			return
		}

		// 更新订单状态为已支付 (2)
		if err := tx.Model(&order).Update("status", 2).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"message": "更新订单失败"})
			return
		}

		tx.Commit()

		// 立刻从延迟队列中移除任务
		core.RedisClient.ZRem(ctx, "seckill:delay_queue", req.OrderID)

		pushLog(ctx, "SUCCESS", fmt.Sprintf("Order: %s PAID. Saved to DB.", req.OrderID))

		c.JSON(http.StatusOK, gin.H{"status": "paid"})
	})

	// @Summary      Reset the mock system state
	// @Description  Clear redis logs, delay queue, set stock back to 87, and delete mock database orders
	// @Tags         system
	// @Produce      json
	// @Success      200  {object}  map[string]interface{}
	// @Router       /api/reset [post]
	r.POST("/api/reset", func(c *gin.Context) {
		ctx := context.Background()

		// 1. 重置 Redis
		core.RedisClient.Set(ctx, "seckill:stock:1", 87, 0)
		core.RedisClient.Del(ctx, "seckill:delay_queue")
		core.RedisClient.Del(ctx, "seckill:logs")

		// 2. 清空数据库中本测试生成的订单 (以 GS- 开头)
		if core.DB != nil {
			core.DB.Where("order_id LIKE ?", "GS-%").Delete(&OrderItem{})
			core.DB.Where("id LIKE ?", "GS-%").Delete(&Order{})
		}

		pushLog(ctx, "INFO", "System state reset. Valkey cache set: stock=87, locks=0, logs cleared.")

		c.JSON(http.StatusOK, gin.H{"status": "reset"})
	})

	// 启动 HTTP 服务
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	log.Printf("GoShop 服务已启动，监听端口 %s ...", addr)
	if err := r.Run(addr); err != nil {
		log.Fatalf("启动服务失败: %v", err)
	}
}
