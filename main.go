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

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

type Order struct {
	ID          string    `gorm:"primaryKey;column:id"`
	UserID      int       `gorm:"column:user_id"`
	TotalAmount int       `gorm:"column:total_amount"`
	Status      int       `gorm:"column:status"` // 1: 待支付, 2: 已支付, 3: 已取消
	CreatedAt   time.Time `gorm:"column:created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at"`
}

type OrderItem struct {
	ID        int       `gorm:"primaryKey;column:id;autoIncrement"`
	OrderID   string    `gorm:"column:order_id"`
	SkuID     int       `gorm:"column:sku_id"`
	Price     int       `gorm:"column:price"`
	Quantity  int       `gorm:"column:quantity"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

type LogItem struct {
	Time string `json:"time"`
	Type string `json:"type"`
	Msg  string `json:"msg"`
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

	// 5. 静态资源托管
	r.Static("/assets", "./web/dist/assets")
	r.StaticFile("/favicon.ico", "./web/dist/favicon.ico")
	r.StaticFile("/", "./web/dist/index.html")

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
	// 获取监控指标与日志
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

		c.JSON(http.StatusOK, gin.H{
			"metrics": gin.H{
				"seckillStock": stock,
				"lockStock":    lockStock,
				"ordersPaid":   ordersPaid,
				"revenue":      revenueStr,
			},
			"logs": logs,
		})
	})

	// 模拟秒杀下单
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

		// 4. 加入延迟队列
		now := time.Now().Unix()
		core.RedisClient.ZAdd(ctx, "seckill:delay_queue", redis.Z{
			Score:  float64(now + 15),
			Member: orderID,
		})

		pushLog(ctx, "INFO", fmt.Sprintf("Valkey Lua pre-decrement SUCCESS. Stock left: %s. Order created: %s, pushed to delay queue.", leftStock, orderID))

		// 5. 模拟 3 秒后支付成功
		go func(oid string) {
			time.Sleep(3 * time.Second)
			if core.DB != nil {
				var order Order
				if err := core.DB.First(&order, "id = ?", oid).Error; err == nil {
					if order.Status == 1 {
						core.DB.Model(&order).Update("status", 2)
						core.RedisClient.ZRem(context.Background(), "seckill:delay_queue", oid)
						pushLog(context.Background(), "SUCCESS", fmt.Sprintf("Order: %s PAID. Moved to metrics. Saved to DB.", oid))
					}
				}
			}
		}(orderID)

		c.JSON(http.StatusOK, gin.H{
			"status":  "success",
			"orderId": orderID,
		})
	})

	// 重置库存与清空数据
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
