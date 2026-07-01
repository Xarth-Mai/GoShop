package app

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"GoShop/config"
	"GoShop/core"
	"GoShop/models"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type ServiceOptions struct {
	Name        string
	DefaultPort int
	SeedData    bool
	Background  func()
	Register    func(*gin.Engine)
}

func RunService(opts ServiceOptions) {
	core.InitLogger()
	defer core.Logger.Sync()

	configPath := getenv("GOSHOP_CONFIG", "config.yaml")
	if err := config.InitConfig(configPath); err != nil {
		core.Logger.Fatal("解析配置文件失败", zap.String("service", opts.Name), zap.Error(err))
	}

	cfg := config.GlobalConfig
	gin.SetMode(cfg.Server.Mode)

	if err := core.InitDB(); err != nil {
		core.Logger.Warn("数据库初始化失败，服务将以数据库未就绪模式启动", zap.String("service", opts.Name), zap.Error(err))
	} else if opts.SeedData {
		if err := models.SeedProducts(core.DB); err != nil {
			core.Logger.Warn("种子数据初始化失败", zap.String("service", opts.Name), zap.Error(err))
		}
	}

	if err := core.InitRedis(); err != nil {
		core.Logger.Warn("Redis/Valkey 初始化失败，缓存能力将不可用", zap.String("service", opts.Name), zap.Error(err))
	}

	if err := core.InitNATS(); err != nil {
		core.Logger.Warn("NATS 初始化失败，消息队列将不可用", zap.String("service", opts.Name), zap.Error(err))
	} else if opts.Name == "goshop-scheduler-service" {
		// scheduler 服务自动声明全局消息持久化 Stream
		if err := core.CreateOrUpdateStream("GOSHOP_EVENTS", []string{"goshop.events.>"}); err != nil {
			core.Logger.Error("声明 NATS Stream 失败", zap.Error(err))
		}
	}

	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery(), core.MetricsMiddleware())
	registerHealth(r, opts.Name)
	r.GET("/metrics", core.ExportMetrics)

	if opts.Register != nil {
		opts.Register(r)
	}
	if opts.Background != nil {
		go opts.Background()
	}

	addr := serviceAddr(opts.DefaultPort)
	core.Logger.Info("GoShop service starting", zap.String("service", opts.Name), zap.String("addr", addr))
	if err := r.Run(addr); err != nil {
		core.Logger.Fatal("服务启动失败", zap.String("service", opts.Name), zap.Error(err))
	}
}

func registerHealth(r *gin.Engine, serviceName string) {
	r.GET("/health", func(c *gin.Context) {
		status := "UP"
		statusCode := http.StatusOK

		dbStatus := "OK"
		if core.DB == nil {
			dbStatus = "DISCONNECTED"
			status = "DOWN"
			statusCode = http.StatusInternalServerError
		} else if sqlDB, err := core.DB.DB(); err != nil || sqlDB.Ping() != nil {
			dbStatus = "ERROR"
			status = "DOWN"
			statusCode = http.StatusInternalServerError
		}

		redisStatus := "OK"
		if core.RedisClient == nil || core.RedisClient.Ping(context.Background()).Err() != nil {
			redisStatus = "DISCONNECTED"
		}

		c.JSON(statusCode, gin.H{
			"status":  status,
			"service": serviceName,
			"time":    time.Now().Format(time.RFC3339),
			"components": gin.H{
				"database": dbStatus,
				"redis":    redisStatus,
			},
		})
	})
}

func serviceAddr(defaultPort int) string {
	if port := os.Getenv("PORT"); port != "" {
		return ":" + port
	}
	if port := os.Getenv("GOSHOP_SERVICE_PORT"); port != "" {
		return ":" + port
	}
	if defaultPort <= 0 {
		defaultPort = config.GlobalConfig.Server.Port
	}
	return fmt.Sprintf(":%d", defaultPort)
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	if len(os.Args) > 1 {
		if _, err := os.Stat(os.Args[1]); err == nil {
			return os.Args[1]
		}
	}
	return fallback
}

func EnvInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}
