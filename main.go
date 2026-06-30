package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	"GoShop/config"
	"GoShop/core"

	"github.com/gin-gonic/gin"
)

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

	// 5. 注册基础路由与健康检查
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

	// 首页欢迎信息
	r.GET("/", func(c *gin.Context) {
		c.String(http.StatusOK, "Welcome to GoShop API Server! 🛒\n")
	})

	// 启动 HTTP 服务
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	log.Printf("GoShop 服务已启动，监听端口 %s ...", addr)
	if err := r.Run(addr); err != nil {
		log.Fatalf("启动服务失败: %v", err)
	}
}
