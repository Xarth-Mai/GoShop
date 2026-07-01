package handlers

import (
	"bufio"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

type caddyRoute struct {
	Name  string
	Paths []string
	Port  string
}

// 解析 Caddyfile.microservices 并返回路由映射
func parseCaddyfile(t *testing.T, filePath string) []caddyRoute {
	file, err := os.Open(filePath)
	if err != nil {
		t.Fatalf("Failed to open Caddyfile: %v", err)
	}
	defer file.Close()

	var routes []caddyRoute
	routeMap := make(map[string]*caddyRoute)

	// 正则匹配 @name path ...
	pathRegex := regexp.MustCompile(`^\s*@(\w+)\s+path\s+(.+)$`)
	// 正则匹配 handle @name {
	handleRegex := regexp.MustCompile(`^\s*handle\s+@(\w+)\s*\{`)
	// 正则匹配 reverse_proxy 127.0.0.1:port 或 :port
	proxyRegex := regexp.MustCompile(`^\s*reverse_proxy\s+(?:127\.0\.0\.1)?(?::)?(\d+)`)

	scanner := bufio.NewScanner(file)
	var currentHandle string

	for scanner.Scan() {
		line := scanner.Text()

		// 1. 匹配 path 定义
		if matches := pathRegex.FindStringSubmatch(line); len(matches) > 0 {
			name := matches[1]
			pathsStr := matches[2]
			paths := strings.Fields(pathsStr)
			if r, exists := routeMap[name]; exists {
				r.Paths = append(r.Paths, paths...)
			} else {
				r := &caddyRoute{Name: name, Paths: paths}
				routeMap[name] = r
				routes = append(routes, *r)
			}
			continue
		}

		// 2. 匹配 handle 块开始
		if matches := handleRegex.FindStringSubmatch(line); len(matches) > 0 {
			currentHandle = matches[1]
			continue
		}

		// 3. 匹配反向代理端口
		if currentHandle != "" {
			if matches := proxyRegex.FindStringSubmatch(line); len(matches) > 0 {
				port := matches[1]
				// 更新 routeMap
				for i := range routes {
					if routes[i].Name == currentHandle {
						routes[i].Port = port
						break
					}
				}
				// 退出 handle 块检测
				currentHandle = ""
			}
		}

		// 如果遇到花括号关闭，清除 currentHandle 状态
		if strings.Contains(line, "}") {
			currentHandle = ""
		}
	}

	if err := scanner.Err(); err != nil {
		t.Fatalf("Error reading Caddyfile: %v", err)
	}

	return routes
}

// 转换 Caddy 匹配路径模式为正则表达式
func caddyPatternToRegex(pattern string) *regexp.Regexp {
	// 把正则保留字转义（除了 *）
	escaped := regexp.QuoteMeta(pattern)
	// 把 \* 替换成 .*
	escaped = strings.ReplaceAll(escaped, `\*`, `.*`)
	return regexp.MustCompile("^" + escaped + "$")
}

// 转换 Gin 路由路径为可以匹配的格式 (把 :param 变成 test_id)
func formatGinPath(path string) string {
	// 匹配 :id 或 :skuId 等，替换成任意单词，比如 "testval"
	paramRegex := regexp.MustCompile(`:\w+`)
	return paramRegex.ReplaceAllString(path, "testval")
}

func TestCaddyRoutesMatchGoServices(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// 解析 Caddyfile.microservices
	// 从当前测试执行目录寻找，应该在 ../deploy/Caddyfile.microservices
	caddyRoutes := parseCaddyfile(t, "../deploy/Caddyfile.microservices")

	if len(caddyRoutes) == 0 {
		t.Fatal("Parsed 0 routes from Caddyfile, parser might be broken")
	}

	// 各个 Go 服务的默认端口和注册路由函数映射
	services := []struct {
		name        string
		defaultPort string
		register    func(*gin.Engine)
	}{
		{"user", "8101", RegisterUserServiceRoutes},
		{"product", "8102", RegisterProductServiceRoutes},
		{"inventory", "8103", RegisterInventoryServiceRoutes},
		{"promotion", "8104", RegisterPromotionServiceRoutes},
		{"order", "8105", RegisterOrderServiceRoutes},
		{"payment", "8106", RegisterPaymentServiceRoutes},
		{"aftersale", "8107", RegisterAfterSaleServiceRoutes},
		{"cart", "8108", RegisterCartServiceRoutes},
	}

	// 端口到 Caddyfile 路由的映射，方便快速查找
	portToCaddyRoute := make(map[string]caddyRoute)
	for _, cr := range caddyRoutes {
		if cr.Port != "" {
			portToCaddyRoute[cr.Port] = cr
		}
	}

	for _, svc := range services {
		t.Run("Service "+svc.name, func(t *testing.T) {
			cr, exists := portToCaddyRoute[svc.defaultPort]
			if !exists {
				t.Fatalf("No Caddyfile route matches default port %s for service %s", svc.defaultPort, svc.name)
			}

			// 获取 Go 服务注册的所有路由
			r := gin.New()
			svc.register(r)
			goRoutes := r.Routes()

			// 检查 Go 服务的每个路由是否能被 Caddyfile 中对应的匹配路径捕获
			for _, route := range goRoutes {
				// 内部微服务调用接口严禁通过 Caddyfile 暴露给外部，在此特判跳过校验
				if strings.HasPrefix(route.Path, "/api/internal") {
					continue
				}

				// 格式化 Gin 的路径占位符
				testPath := formatGinPath(route.Path)

				// 对特殊的旁路逻辑或者秒杀分流路由（/api/seckill 在 order 服务中也有注册，但被 Caddy 转发给 inventory）做特判
				if svc.name == "order" && route.Path == "/api/seckill" {
					// 订单服务中的 /api/seckill 是预留/旁路或历史逻辑，它在 Caddyfile 中确实会被转发到 inventory(8103) 而非 order(8105)
					t.Logf("Skipped checking /api/seckill on order-service (known to be routed to inventory-service)")
					continue
				}

				matched := false
				for _, pattern := range cr.Paths {
					reg := caddyPatternToRegex(pattern)
					if reg.MatchString(testPath) {
						matched = true
						break
					}
				}

				if !matched {
					t.Errorf("Go route %s %s (formatted: %s) on service %s (port %s) has no matching path filter in Caddyfile patterns: %v",
						route.Method, route.Path, testPath, svc.name, svc.defaultPort, cr.Paths)
				}
			}
		})
	}
}
