# GoShop Deployment Notes

## Topology

```text
Browser
  -> Cloudflare Edge
  -> cloudflared outbound tunnel
  -> Caddy 127.0.0.1:8080
  -> GoShop services on 127.0.0.1:8101-8109
```

The server does not need public inbound 80/443 access when using Cloudflare Tunnel. Caddy listens locally and routes API paths to the service processes.

## Files

- `deploy/Caddyfile.microservices`: local Caddy reverse proxy and static frontend hosting.
- `deploy/cloudflared/config.yml`: tunnel ingress example.
- `deploy/systemd/*.service`: service manager examples for GoShop processes and cloudflared.

## Build And Install Shape

```bash
mkdir -p /opt/goshop/bin
go build -o /opt/goshop/bin/goshop-user-service ./cmd/goshop-user-service
go build -o /opt/goshop/bin/goshop-product-service ./cmd/goshop-product-service
go build -o /opt/goshop/bin/goshop-inventory-service ./cmd/goshop-inventory-service
go build -o /opt/goshop/bin/goshop-promotion-service ./cmd/goshop-promotion-service
go build -o /opt/goshop/bin/goshop-order-service ./cmd/goshop-order-service
go build -o /opt/goshop/bin/goshop-payment-service ./cmd/goshop-payment-service
go build -o /opt/goshop/bin/goshop-aftersale-service ./cmd/goshop-aftersale-service
go build -o /opt/goshop/bin/goshop-cart-service ./cmd/goshop-cart-service
go build -o /opt/goshop/bin/goshop-scheduler-service ./cmd/goshop-scheduler-service
```

Copy `config.yaml`, `web/dist`, `deploy/Caddyfile.microservices`, and the relevant systemd units to the target host. Update hostnames in `deploy/cloudflared/config.yml` before use.

## Local Build & Smoke Run (本地一键构建与冒烟测试)

项目在 `deploy/` 目录提供了辅助脚本，用于在本地快速构建和测试微服务集群：

1. **一键构建所有微服务**：

   ```bash
   ./deploy/build.sh
   ```

   该脚本会自动编译所有 9 个微服务并将可执行二进制文件输出到本地的 `bin/` 目录。

2. **后台并发拉起所有微服务**：

   ```bash
   ./deploy/smoke_run.sh start
   ```

   它会使用 `nohup` 在后台并发启动所有微服务，并将日志重定向输出到 `logs/goshop-*-service.log`。

3. **查看微服务进程运行状态**：

   ```bash
   ./deploy/smoke_run.sh status
   ```

4. **一键清理并停止所有微服务**：

   ```bash
   ./deploy/smoke_run.sh stop
   ```

## Security Boundary

- Bind Caddy and GoShop services to loopback or private interfaces.
- Keep PostgreSQL and Redis private to the host or private network.
- Protect `admin.shop.example.com` with Cloudflare Access and keep app-level RBAC enabled.
- Keep the browser HMAC signing key treated as a demo integrity mechanism, not as a strong security boundary.
