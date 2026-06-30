package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"GoShop/config"
	"GoShop/core"
	_ "GoShop/docs"
	"GoShop/handlers"
	"GoShop/models"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"go.uber.org/zap"
)

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

// @title           GoShop API
// @version         1.0
// @description     一个基于 Go 语言构建的轻量级、高性能通用电商后端系统。
// @host            localhost:3233
// @BasePath        /
func main() {
	// 1. 初始化结构化日志 Zap
	core.InitLogger()
	defer core.Logger.Sync()

	// 2. 初始化配置
	configPath := "config.yaml"
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		core.Logger.Fatal("配置文件不存在，请复制 config.example.yaml 为 config.yaml 并进行调整", zap.String("path", configPath))
	}

	err := config.InitConfig(configPath)
	if err != nil {
		core.Logger.Fatal("解析配置文件失败", zap.Error(err))
	}

	cfg := config.GlobalConfig

	// 设置 Gin 运行模式
	gin.SetMode(cfg.Server.Mode)

	// 3. 初始化数据库 (PostgreSQL)
	core.Logger.Info("正在连接 PostgreSQL 数据库并执行读写分离配置...")
	err = core.InitDB()
	if err != nil {
		core.Logger.Warn("连接 PostgreSQL 数据库失败，系统将以数据库未就绪模式继续启动", zap.Error(err))
	} else {
		core.Logger.Info("PostgreSQL 数据库主从集群连接成功，且 AutoMigrate 表结构迁移对齐完成.")
		// 种子化测试数据
		if err := models.SeedProducts(core.DB); err != nil {
			core.Logger.Error("数据库初始数据种子化失败", zap.Error(err))
		} else {
			core.Logger.Info("数据库初始卡券、地址、用户及商品数据对齐完成.")
		}
	}

	// 4. 初始化 Redis (Valkey)
	core.Logger.Info("正在连接 Redis/Valkey 高速缓存与延迟队列...")
	err = core.InitRedis()
	if err != nil {
		core.Logger.Warn("连接 Redis/Valkey 失败，缓存与延迟队列将不可用", zap.Error(err))
	} else {
		core.Logger.Info("Redis/Valkey 缓存层初始化成功.")
	}

	// 5. 初始化 Gin 引擎与全局中间件
	r := gin.Default()

	// 接入全局可观测性 Prometheus 拦截器，统计访问次数与延迟
	r.Use(core.MetricsMiddleware())

	// 启动高可用延迟队列后台可靠 Worker
	go handlers.StartReliableDelayQueueWorker()

	// 6. 静态资源与前端合一托管
	r.Static("/assets", "./web/dist/assets")
	r.StaticFile("/favicon.ico", "./web/dist/favicon.ico")
	r.StaticFile("/", "./web/dist/index.html")

	// Swagger API 文档挂载
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

	// SPA 路由兜底，确保前端刷新路由不丢失
	r.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.JSON(http.StatusNotFound, gin.H{"message": "API not found"})
			return
		}
		c.File("./web/dist/index.html")
	})

	// 7. 系统自愈监控接口
	r.GET("/health", func(c *gin.Context) {
		dbStatus := "OK"
		if core.DB == nil {
			dbStatus = "DISCONNECTED"
			// 尝试重连自愈
			core.Logger.Warn("检测到数据库失联，尝试断线重连自愈...")
			for i := 1; i <= 3; i++ {
				if err := core.InitDB(); err == nil {
					dbStatus = "OK (RESTORED)"
					core.Logger.Info("数据库断线重连自愈成功!")
					break
				}
				time.Sleep(1 * time.Second)
			}
		} else {
			sqlDB, err := core.DB.DB()
			if err != nil || sqlDB.Ping() != nil {
				dbStatus = "ERROR"
			}
		}

		redisStatus := "OK"
		if core.RedisClient == nil || core.RedisClient.Ping(context.Background()).Err() != nil {
			redisStatus = "DISCONNECTED"
			// 尝试重连自愈
			core.Logger.Warn("检测到 Redis/Valkey 失联，尝试重连自愈...")
			for i := 1; i <= 3; i++ {
				if err := core.InitRedis(); err == nil {
					redisStatus = "OK (RESTORED)"
					core.Logger.Info("Redis/Valkey 重连自愈成功!")
					break
				}
				time.Sleep(1 * time.Second)
			}
		}

		status := "UP"
		statusCode := http.StatusOK
		if dbStatus != "OK" && dbStatus != "OK (RESTORED)" {
			status = "DOWN"
			statusCode = http.StatusInternalServerError
			// 模拟发送邮件通知管理员
			core.Logger.Error("[ALERT] 报警触发：系统核心数据库组件故障，且重连自愈失败！通知已发送。")
		}

		c.JSON(statusCode, gin.H{
			"status": status,
			"components": gin.H{
				"database": dbStatus,
				"redis":    redisStatus,
			},
		})
	})

	// 8. 看板专属指标与日志同步 API
	r.GET("/api/metrics", func(c *gin.Context) {
		ctx := context.Background()

		// 获取秒杀商品缓存库存数
		stockStr, err := core.RedisClient.Get(ctx, "seckill:stock:1").Result()
		if err != nil {
			core.RedisClient.Set(ctx, "seckill:stock:1", 87, 0)
			stockStr = "87"
		}
		stock, _ := strconv.Atoi(stockStr)

		// 延迟锁库存计数
		lockStock, _ := core.RedisClient.ZCard(ctx, "seckill:delay_queue").Result()

		// 已支付订单总额及销售额
		var ordersPaid int64 = 0
		var totalRevenueCent int64 = 0

		if core.DB != nil {
			core.DB.Model(&models.Order{}).Where("status = ?", 2).Count(&ordersPaid)
			core.DB.Model(&models.Order{}).Where("status = ?", 2).Select("COALESCE(SUM(total_amount), 0)").Row().Scan(&totalRevenueCent)
		}

		revenueVal := float64(totalRevenueCent) / 100.0
		revenueStr := formatCurrency(revenueVal)

		// 获取日志记录
		var logs []handlers.LogItem
		logStrs, _ := core.RedisClient.LRange(ctx, "seckill:logs", 0, -1).Result()
		for _, s := range logStrs {
			var item handlers.LogItem
			if err := json.Unmarshal([]byte(s), &item); err == nil {
				logs = append(logs, item)
			}
		}

		// 获取待支付订单 (status = 1)
		var pendingOrders []models.Order
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

	// 9. 公开商品展示接口 (用于商品浏览，支持分类、模糊过滤等)
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
		if page < 1 {
			page = 1
		}
		if pageSize < 1 {
			pageSize = 10
		}

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

	// 10. 系统一键重置接口 (清除 Redis 记录和测试数据库订单)
	r.POST("/api/reset", func(c *gin.Context) {
		ctx := context.Background()

		// 重置 Redis 缓存
		core.RedisClient.Set(ctx, "seckill:stock:1", 87, 0)
		core.RedisClient.Del(ctx, "seckill:delay_queue")
		core.RedisClient.Del(ctx, "seckill:delay_queue:processing")
		core.RedisClient.Del(ctx, "seckill:logs")

		// 删除测试生成的普通与秒杀演示订单
		if core.DB != nil {
			core.DB.Where("order_id LIKE ? OR order_id LIKE ?", "GS-%", "SK-%").Delete(&models.OrderItem{})
			core.DB.Where("id LIKE ? OR id LIKE ?", "GS-%", "SK-%").Delete(&models.Order{})
			core.DB.Where("order_id LIKE ? OR order_id LIKE ?", "GS-%", "SK-%").Delete(&models.DeadLetterOrder{})
			// 恢复所有的卡券和购物车
			core.DB.Exec("UPDATE user_coupons SET status = 0, used_at = NULL")
		}

		c.JSON(http.StatusOK, gin.H{"status": "reset"})
	})

	// 11. 注册核心重构业务路由与拦截保护中间件
	handlers.RegisterRoutes(r)

	// 启动 HTTP 服务
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	core.Logger.Info("GoShop 后端业务服务已就绪，正在启动监听...", zap.String("address", addr))
	if err := r.Run(addr); err != nil {
		core.Logger.Fatal("GoShop 服务启动失败", zap.Error(err))
	}
}
