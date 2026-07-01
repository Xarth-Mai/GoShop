# GoShop Agent Guide

本文件面向在本仓库内工作的自动化代码代理。开始修改前请先阅读 `README.md`、相关目录下的代码，以及本文件中的约定。

## 项目概览

GoShop 是一个 Go + Gin 的电商后端项目，支持两种运行形态：

- 默认单体运行时：`main.go` 启动完整后端、静态前端托管和后台 worker，默认监听 `:3233`。
- 多进程微服务过渡运行时：`cmd/goshop-*-service` 下的服务分别监听 `8101` 到 `8109`，通过 `deploy/Caddyfile.microservices` 统一网关转发。

核心基础设施包括 PostgreSQL、Valkey/Redis、NATS JetStream、GORM、Swagger，以及 `web` 下的 Vue 3 前端。

## 目录速览

- `main.go`：单体运行时入口。
- `cmd/`：各微服务进程入口及对应测试。
- `internal/`：领域服务实现，优先在这里放业务逻辑。
- `handlers/`：Gin 路由、HTTP handler、服务注册和订阅入口。
- `models/`：GORM 模型、种子数据及模型相关测试。
- `core/`：数据库、缓存、NATS、鉴权、中间件、内部服务调用等基础能力。
- `config/`：配置加载逻辑。
- `docs/`：Swagger 产物、微服务文档和测试交接提示。
- `deploy/`：Caddy 配置、构建和冒烟脚本。
- `web/`：Vue 3 前端应用，构建产物位于 `web/dist`。

## 常用命令

后端测试：

```bash
GOCACHE=/tmp/goshop-go-build go test ./...
```

单体启动：

```bash
GOSHOP_CONFIG=config.yaml go run main.go
```

前端构建：

```bash
cd web
bun run build
```

构建微服务二进制：

```bash
./deploy/build.sh
```

微服务冒烟：

```bash
./deploy/smoke_run.sh start
./deploy/smoke_run.sh status
./deploy/smoke_run.sh stop
```

Swagger 重新生成：

```bash
GOCACHE=/tmp/goshop-go-build go run github.com/swaggo/swag/cmd/swag init -g main.go -o docs
```

## 修改原则

- 优先保持现有架构：不要把微服务过渡形态改成另一套框架，也不要绕过 `internal/` 中已有领域服务。
- HTTP handler 应尽量薄，业务规则放在对应 `internal/<domain>` 服务中。
- 涉及跨服务调用时，检查 `core.CallInternalService` 和 `handlers/internal_handlers.go`，保留本地 fallback 和幂等语义。
- 涉及订单、支付、售后、库存时，同时考虑数据库事务、Valkey 库存、Outbox/Inbox 和 NATS JetStream 的一致性。
- 修改公开 API 时，同步检查 Swagger、`README.md`、`docs/microservices.md` 和 Caddy 路由。
- 不要把 `/api/internal/*` 作为公开 API 暴露到 Swagger。
- 不要提交本地敏感配置；`config.yaml` 是本地运行配置，示例配置应写入 `config.example.yaml`。
- 不要随意删除 `web/dist`，单体运行时依赖它托管前端静态资源。

## 测试与验证建议

小范围 Go 逻辑修改至少运行相关包测试；跨领域或路由修改运行：

```bash
GOCACHE=/tmp/goshop-go-build go test ./...
```

前端或静态资源相关修改运行：

```bash
cd web
bun run build
```

涉及微服务拆分、Caddy 路由、内部 RPC、NATS 或后台 worker 时，优先参考 `docs/agent_test_prompts.md` 中的微服务冒烟和契约测试提示。

## 前端约定

- 前端位于 `web/src`，技术栈为 Vue 3 + Pinia + Vue Router + Vanilla CSS。
- 保持现有视觉方向和 `Design.md` 中的暖乳白、珊瑚色、克制运营后台风格。
- 不要新增营销落地页来替代实际应用界面。
- 修改 UI 后至少运行 `bun run build`，必要时补充浏览器验证。

## 交付说明

完成任务时说明：

- 修改了哪些文件和行为。
- 执行过哪些验证命令。
- 如果没有运行某项关键验证，说明原因。
