package core

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
)

var (
	// HttpRequestsTotal 累计请求计数
	HttpRequestsTotal int64
	// TotalDurationMs 累计耗时毫秒
	TotalDurationMs int64
	// ActiveConnections 当前连接数
	ActiveConnections int64
)

// MetricsMiddleware Prometheus 指标统计拦截器
func MetricsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		atomic.AddInt64(&ActiveConnections, 1)
		defer atomic.AddInt64(&ActiveConnections, -1)

		start := time.Now()
		c.Next()
		duration := time.Since(start).Milliseconds()

		atomic.AddInt64(&HttpRequestsTotal, 1)
		atomic.AddInt64(&TotalDurationMs, duration)
	}
}

// ExportMetrics 导出 Prometheus 规范格式的监控指标
func ExportMetrics(c *gin.Context) {
	ctx := context.Background()

	// 1. 获取延迟队列堆积数量
	var queueLength int64 = 0
	if RedisClient != nil {
		l1, _ := RedisClient.ZCard(ctx, "seckill:delay_queue").Result()
		l2, _ := RedisClient.ZCard(ctx, "seckill:delay_queue:processing").Result()
		queueLength = l1 + l2
	}

	// 2. 计算平均耗时 (秒)
	avgDurationSec := 0.0
	reqs := atomic.LoadInt64(&HttpRequestsTotal)
	if reqs > 0 {
		avgDurationSec = float64(atomic.LoadInt64(&TotalDurationMs)) / float64(reqs) / 1000.0
	}

	// 3. 构建符合 Prometheus 格式的报文
	res := ""
	res += "# HELP goshop_http_requests_total Total number of HTTP requests handled.\n"
	res += "# TYPE goshop_http_requests_total counter\n"
	res += fmt.Sprintf("goshop_http_requests_total %d\n\n", reqs)

	res += "# HELP goshop_http_request_duration_seconds Average HTTP request duration in seconds.\n"
	res += "# TYPE goshop_http_request_duration_seconds gauge\n"
	res += fmt.Sprintf("goshop_http_request_duration_seconds %.6f\n\n", avgDurationSec)

	res += "# HELP goshop_delay_queue_length Number of active items in the delay queue.\n"
	res += "# TYPE goshop_delay_queue_length gauge\n"
	res += fmt.Sprintf("goshop_delay_queue_length %d\n\n", queueLength)

	res += "# HELP goshop_current_connections Number of active connections currently handled.\n"
	res += "# TYPE goshop_current_connections gauge\n"
	res += fmt.Sprintf("goshop_current_connections %d\n", atomic.LoadInt64(&ActiveConnections))

	c.Header("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	c.String(http.StatusOK, res)
}
