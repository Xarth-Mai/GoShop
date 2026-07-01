# GoShop 🛒

> 一个基于 Go 语言构建的轻量级、高性能通用电商后端系统。

本项目旨在提供一个高并发、高可用的电商基础架构。系统支持两种运行形态：

1. **模块化单体运行时 (Monolith Runtime)**：默认推荐，单进程运行并监听 `:3233` 端口，内置完整的前端静态文件托管（位于 `web/dist`）与后台延迟任务 Worker。
2. **多进程微服务过渡运行时 (Transitional Multi-Process Microservice Runtime)**：单仓库多进程（Mono-repo, Multi-process）服务形态。9 个核心服务作为独立二进制进程启动，分别运行在 `8101` ~ `8109` 独立端口。由 Caddy 网关（Caddyfile 监听 `:8080`）实现统一的路由分发与前端静态文件代理。在该形态下，所有异步后台 Worker（Outbox 事件发布、订单支付超时释放等）统一由 `goshop-scheduler-service` 调度器服务独立承载，其余 API 服务均为纯粹无状态的 HTTP 服务。

当前重点攻克了交易一致性、库存预占、支付回调幂等、售后退款和轻量级事件发布。

**在线演示：** [https://shop.lzzz.ink](https://shop.lzzz.ink)

**API文档：** [https://shop.lzzz.ink/swagger/index.html](https://shop.lzzz.ink/swagger/index.html)

---

## 💡 核心技术亮点 (Technical Highlights)

### 1. 内存级原子预扣库存（杜绝超卖漏洞）

传统的数据库行级锁（`SELECT FOR UPDATE`）在大流量冲击下极易导致数据库连接池爆满、响应剧烈延迟。

* **设计实现：** 本项目将热点商品的 SKU 库存同步至高性能的 **Valkey (Redis)** 缓存中。
* **核心机制：** 下单链路放弃操作关系型数据库，完全由 Go 驱动 **Lua 脚本**在 Valkey 内部实现“判断库存 -> 扣减库存”的原子化操作。
* **务实价值：** 在内存层将高并发请求转化为无锁的单线程串行执行，将扣库存的 QPS 提升了两个数量级，且从根源上完全杜绝了电商经典的“超卖”现象。

### 2. 轻量级延迟队列（保障订单最终一致性）

用户下单后“占位”库存，若长时间不支付会导致死锁库存、影响实际商品销售。

* **设计实现：** 引入轻量级异步任务队列，利用 Valkey 的有序集合（ZSet）作为底座。
* **核心机制：** 订单创建成功后，立刻投递一个异步超时检查任务：普通订单 30 分钟、秒杀订单 15 秒。消费者进程在后台平稳消耗队列，在超时触发时校验订单状态。
* **务实价值：** 免去了轮询数据库导致的 I/O 损耗。如果用户未支付，系统将自动修改订单状态并安全回滚 Valkey 与 PostgreSQL 中的物理库存，确保业务闭环。

### 3. 交易闭环与 Outbox 事件总线

* **设计实现：** 普通订单采用“价格试算 -> 优惠券锁定 -> 库存预占 -> 支付单 -> mock 回调 -> 库存确认 -> 售后退款”的统一状态机。
* **核心机制：** 订单、支付、售后在本地事务内写入 `outbox_events`，调度服务将待发送事件发布到 NATS JetStream `goshop.events.<aggregate>.<event>` 主题。
* **务实价值：** 单体内保持强事务一致性，微服务形态下通过 NATS JetStream 与 `inbox_events` 幂等消费完成跨库最终一致性对账。

### 4. 基于 PostgreSQL 的高性能分层商品模型

通用电商的痛点在于多规格商品（颜色、尺寸等）繁杂的检索与高频变动。

* **设计实现：** 在关系型数据库中采用严格的 SPU (标准产品单元) 与 SKU (库存量单位) 1:N 物理分层设计。
* **核心机制：** 深度结合 GORM 关联查询，并在外层通过二级缓存（本地缓存 + 分布式缓存）对静态商品详情做多级加速。
* **务实价值：** 结构设计符合企业级规范，既保证了商品规格灵活拓展的便利性，又规避了频繁多表联查产生的性能包袱。

### 5. JWT 无状态鉴权与工业级安全实践

* **设计实现：** 全站采用 JWT (JSON Web Token) 实现用户的无状态分布式认证，配合自研 Gin 中间件进行精细化的路由权限拦截与令牌自动续签（Refresh Token）机制。
* **核心机制：** 在网关/路由层拦截非法数据，敏感字段（如卡密、核心凭证）在 PostgreSQL 内部全部采取高级对称加密（AES）存储，避免数据库意外泄露导致的生产安全事故。

### 6. 现代化后台管理界面（高效的运营支撑与库存监控）

* **设计实现：** 配套设计自适应、精美的后台管理系统，实现商品 SPU/SKU 管理、订单发货、库存阈值警报以及销售数据可视化。
* **核心机制：** 基于 RBAC 的动态菜单权限控制，实时展示秒杀/高并发下的 Valkey 库存余量与延迟队列消费状态，提升系统可维护性。

---

## 🛠️ 技术栈选型

* **后端核心:** Go + Gin (轻量极速的 Web 框架，生产环境路由性能卓越)
* **后台前端:** Vue 3 + Pinia + Vue Router + Vanilla CSS (严格遵循 Anthropic 暖乳白极简设计规范，实现多页面商品选购、购物车及延迟队列秒杀支付链路，深度集成实时技术引擎监控看板)
* **持久层:** PostgreSQL 14+ + GORM (利用 PG 强大的关系型事务特性存储 SPU/SKU 及订单信息)
* **缓存与异步组件:** Valkey 7+ (Redis 社区正统分支，负责高并发 Lua 原子扣减库存与异步延迟队列) + NATS JetStream (负责跨服务事件发布与消费)
* **API 规范:** Swagger (利用 `swaggo/swag` 自动化生成可交互的 RESTful API 文档，本地完美渲染)

---

## 🚀 极速本地启动 (Quick Start)

本项目设计为开箱即用，方便进行本地调试与代码审查。

### 1. 环境准备

请确保你的本地机器或服务器已安装以下基础组件：

* Go 1.20 或更高版本
* PostgreSQL
* Valkey (或 Redis 7.0+)

### 2. 数据库初始化

1. 在 PostgreSQL 中创建一个空数据库（如命名为 `goshop`）。
2. 导入项目根目录下的初始化 SQL 文件：

```bash
psql -U your_username -d goshop -f init.sql

```

### 3. 修改配置

复制一份示例配置文件，并根据你的本地环境修改数据库和 Valkey 的连接信息：

```bash
cp config.example.yaml config.yaml

```

### 4. 运行服务

由于本项目已实现**单端口统一托管模式**，启动服务前需要先将前端项目编译打包至 `web/dist`（Gin 会自动静态托管该目录）：

1. **构建前端静态资源**（请确保已安装 `bun`，或使用 `npm`/`yarn` 代替）：

   ```bash
   cd web
   bun install
   bun run build
   cd ..
   ```

2. **启动 Go 后端单体服务**：

   ```bash
   go mod tidy
   go run main.go
   ```

启动成功后，终端将输出：`GoShop 服务已启动，监听端口 :3233 ...`。

直接打开浏览器访问以下地址，即可同时访问完整的 API 与前端控制台：
👉 **[http://localhost:3233](https://shop.lzzz.ink)**

### 5. 微服务过渡形态

为了平滑演进并降低单体拆分风险，本项目提供了一种单仓库、多进程的微服务过渡运行形态。

#### 核心运行特征

* **多进程独立端口**：9 个微服务分别监听独立的默认端口（`8101` ~ `8109`），各服务的具体职责及接口映射可见 [docs/microservices.md](docs/microservices.md)。
* **后台任务集中化**：在微服务形态下，所有异步后台 Worker（Outbox 消息发布器、延迟队列超时释放 Worker）都脱离 HTTP 业务进程，统一在 `goshop-scheduler-service`（`:8109`）中以后台协程方式运行。
* **Caddy 统一网关分发**：使用 Caddy 网关监听 `:8080` 端口，它不仅代理前端静态文件，还根据请求的 API 前缀将请求反向代理到不同的微服务进程。
  * *特别提示*：对于高并发秒杀路由 `/api/seckill`，Caddyfile 会将其强制分发给专门的 `goshop-inventory-service`（`:8103`），利用其内存 Lua 脚本预扣库存引擎保护数据库，而订单生成和查询路由则导向 `goshop-order-service`（`:8105`）。
* **配置覆盖**：每个服务进程启动时，支持以下环境变量配置：
  * `GOSHOP_CONFIG`：配置文件路径（默认为 `config.yaml`）。
  * `PORT` 或 `GOSHOP_SERVICE_PORT`：可直接覆盖该进程的监听端口。
  * 服务特定端口变量：例如 `GOSHOP_USER_PORT` 可以覆盖特定服务的默认端口。

#### 启动命令

1. **方式 A：通过 go run 逐个启动（本地调试）**：

   ```bash
   GOSHOP_CONFIG=config.yaml go run ./cmd/goshop-user-service
   GOSHOP_CONFIG=config.yaml go run ./cmd/goshop-product-service
   GOSHOP_CONFIG=config.yaml go run ./cmd/goshop-inventory-service
   GOSHOP_CONFIG=config.yaml go run ./cmd/goshop-promotion-service
   GOSHOP_CONFIG=config.yaml go run ./cmd/goshop-order-service
   GOSHOP_CONFIG=config.yaml go run ./cmd/goshop-payment-service
   GOSHOP_CONFIG=config.yaml go run ./cmd/goshop-aftersale-service
   GOSHOP_CONFIG=config.yaml go run ./cmd/goshop-cart-service
   GOSHOP_CONFIG=config.yaml go run ./cmd/goshop-scheduler-service
   ```

2. **方式 B：编译为二进制并启动**：

   ```bash
   mkdir -p bin
   go build -o bin/goshop-user-service ./cmd/goshop-user-service
   go build -o bin/goshop-product-service ./cmd/goshop-product-service
   go build -o bin/goshop-inventory-service ./cmd/goshop-inventory-service
   go build -o bin/goshop-promotion-service ./cmd/goshop-promotion-service
   go build -o bin/goshop-order-service ./cmd/goshop-order-service
   go build -o bin/goshop-payment-service ./cmd/goshop-payment-service
   go build -o bin/goshop-aftersale-service ./cmd/goshop-aftersale-service
   go build -o bin/goshop-cart-service ./cmd/goshop-cart-service
   go build -o bin/goshop-scheduler-service ./cmd/goshop-scheduler-service
   
   # 启动示例
   GOSHOP_CONFIG=config.yaml ./bin/goshop-user-service
   ```

3. **配置 Caddy 反向代理网关**：
   使用项目根目录下的 `deploy/Caddyfile.microservices` 启动 Caddy：

   ```bash
   caddy run --config deploy/Caddyfile.microservices
   ```

   启动后，直接访问网关端口：👉 **[http://localhost:8080](http://localhost:8080)**

默认端口和 Caddy 路由细则详见 [docs/microservices.md](docs/microservices.md) 与 [deploy/Caddyfile.microservices](deploy/Caddyfile.microservices)。

---

## 🛠️ 微服务生产化后续工作 (Production Hardening Work)

当前微服务形态已经完成独立进程、服务专属数据库配置、局部表迁移、HTTP-RPC 内部调用、NATS JetStream Outbox/Inbox 消息对账等硬拆分基础能力。后续若要进入更接近生产的长期运行形态，仍建议继续补齐以下工作：

1. **数据库权限最小化**：为每个服务配置独立账号和只读/读写权限边界，并补充迁移审计。
2. **跨服务契约治理**：将 `/api/internal/*` HTTP-RPC 契约沉淀为版本化接口文档或 OpenAPI 子文档，并补充兼容性测试。
3. **消息可靠性运营化**：为 NATS JetStream 消费延迟、重试、死信和 Inbox 堆积增加监控告警。
4. **商家管理后台与支付系统重构**：剥离出独立的 Admin 前端，接入生产级第三方支付渠道并补充每日财务对账任务。

---

## 📚 API 文档 (Swagger)

本项目集成了 Swagger 文档以方便接口调试与对齐。
在本地服务启动后，打开浏览器访问以下地址即可查看完整的 RESTful 接口细节与传参规范，支持本地接口调试与交互：
👉 **[http://localhost:3233/swagger/index.html](http://localhost:3233/swagger/index.html)**

（在线演示版文档可访问：[https://shop.lzzz.ink/swagger/index.html](https://shop.lzzz.ink/swagger/index.html)）

## ✅ 验证与后续测试交接

当前核心回归命令：

```bash
GOCACHE=/tmp/goshop-go-build go test ./...
cd web && npm run build
```

非核心的扩展测试、微服务冒烟、前端 E2E 和 Swagger QA 已整理为可交给其他 agent 的任务提示词，见 [docs/agent_test_prompts.md](docs/agent_test_prompts.md)。

---

## 📂 目录结构简析

```text
.
├── cmd/            # 多进程微服务入口
├── config/         # 配置读取与配置项映射
├── core/           # 核心组件初始化 (PostgreSQL 读写分离连接池、Valkey 客户端)
├── deploy/         # Caddy 与 systemd 部署样例
├── handlers/       # HTTP 控制器层
├── internal/       # checkout/order/payment/inventory/promotion/outbox 等领域服务
├── models/         # 数据模型层 (GORM 结构体，SPU 与 SKU 关系映射)
├── web/            # Vue 3 前端项目
├── docs/           # Swagger 自动生成的接口说明文档文件
├── init.sql        # 数据库快速初始化脚本
└── main.go         # 单体入口

```
