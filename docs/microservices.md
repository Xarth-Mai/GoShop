# GoShop Microservice Split

This repository now supports two deployment shapes:

1. `go run .`: the existing Gin monolith with embedded frontend hosting.
2. `go run ./cmd/goshop-*-service`: single-repository, multi-process services routed by Caddy.

The split has evolved from a transitional shared-DB split to a **fully decoupled physical microservice split**. Each service now connects to its own independent database schema and runs local database migrations on startup. Cross-service communication is achieved synchronously via point-to-point HTTP-RPC (with local unit-testing SQLite memory DB fallbacks to prevent import cycles and make local verification plug-and-play) and asynchronously via a NATS JetStream message bus with idempotent Inbox consumer reconciliation.

## Services

| Service | Default port | Entrypoint | Exact API Routes (Registered in Code) |
| --- | ---: | --- | --- |
| user | 8101 | `./cmd/goshop-user-service` | `/api/auth/*` (auth/sign-key, login, register, refresh), `/api/addresses*` |
| product | 8102 | `./cmd/goshop-product-service` | `/api/categories`, `/api/products*` |
| inventory | 8103 | `./cmd/goshop-inventory-service` | `/api/seckill` (High-concurrency memory stock pre-deduct) |
| promotion | 8104 | `./cmd/goshop-promotion-service` | `/api/coupons`, `/api/user-coupons*` |
| order | 8105 | `./cmd/goshop-order-service` | `/api/checkout/*`, `/api/orders*`, `/api/seckill` (Fallback route, not proxied here by gateway) |
| payment | 8106 | `./cmd/goshop-payment-service` | `/api/payments/callback/mock`, `/api/payments`, `/api/payments/:id`, `/api/pay` |
| aftersale | 8107 | `./cmd/goshop-aftersale-service` | `/api/orders/:id/refund` (Apply), `/api/admin/orders/:id/refund/audit` (Admin audit) |
| cart | 8108 | `./cmd/goshop-cart-service` | `/api/cart*` |
| scheduler | 8109 | `./cmd/goshop-scheduler-service` | `/health`, `/metrics` (Runs background outbox publisher and delay queue worker) |

Each service supports configuration overrides via environment variables.

### Microservice Routing & Gateway Rules

1. **SecKill Traffic Routing**:
   - Both `goshop-inventory-service` (port `8103`) and `goshop-order-service` (port `8105`) register the `POST /api/seckill` handler in code.
   - In the multi-process microservice runtime, Caddy gateway (see `deploy/Caddyfile.microservices`) directs all `/api/seckill` traffic exclusively to the `inventory` service on port `8103`. The `order` service on port `8105` only handles normal checkout, listings, and detail lookups.

2. **AfterSale Refund Routing Clarification**:
   - The Gin code registers `/api/orders/:id/refund` for users to apply for refunds, and `/api/admin/orders/:id/refund/audit` under the admin route group for auditing.
   - Note that `deploy/Caddyfile.microservices` includes a rule matching `/api/orders/*/refund/audit`. This is redundant as the actual Go backend registers the audit handler at the `/api/admin/...` prefix. All valid audit requests route to port `8107` via `/api/admin/orders/*/refund/audit`.

3. **Background Worker Concentration**:
   - Unlike the monolith mode where background tasks start implicitly inside `main.go`, in the microservice transition mode, HTTP services (ports `8101` ~ `8108`) remain completely stateless.
   - The `goshop-scheduler-service` (port `8109`) is the single host for background processes. Upon startup, it spawns the transactional Outbox Event Publisher (`outbox.NewPublisher`) to publish events to NATS JetStream subjects such as `goshop.events.payment.paymentsucceeded`, and the Reliable Delay Queue Worker (`StartReliableDelayQueueWorker`) to handle ticket timeouts and stock rollbacks.

### Port and Configuration Overrides

Each microservice supports configuration and port adjustments via environment variables (resolved in `internal/app/runtime.go`):

1. **Configuration File**: Specify the config path using `GOSHOP_CONFIG`. Defaults to `config.yaml` in the working directory.
2. **Port Binding Precedence**:
   - First Priority: `PORT` environment variable (e.g. `PORT=9000`).
   - Second Priority: `GOSHOP_SERVICE_PORT` environment variable.
   - Third Priority: Service-specific port environment variables, which default to:
     - `GOSHOP_USER_PORT` (for `user` service, default `8101`)
     - `GOSHOP_PRODUCT_PORT` (for `product` service, default `8102`)
     - `GOSHOP_INVENTORY_PORT` (for `inventory` service, default `8103`)
     - `GOSHOP_PROMOTION_PORT` (for `promotion` service, default `8104`)
     - `GOSHOP_ORDER_PORT` (for `order` service, default `8105`)
     - `GOSHOP_PAYMENT_PORT` (for `payment` service, default `8106`)
     - `GOSHOP_AFTERSALE_PORT` (for `aftersale` service, default `8107`)
     - `GOSHOP_CART_PORT` (for `cart` service, default `8108`)
     - `GOSHOP_SCHEDULER_PORT` (for `scheduler` service, default `8109`)
   - Fourth Priority: If none of the above are set, it falls back to the port specified in `config.yaml` (e.g., `server.port` which is `3233`).

## Local Run

```bash
GOSHOP_CONFIG=config.yaml go run ./cmd/goshop-user-service
GOSHOP_CONFIG=config.yaml go run ./cmd/goshop-product-service
GOSHOP_CONFIG=config.yaml go run ./cmd/goshop-order-service
GOSHOP_CONFIG=config.yaml go run ./cmd/goshop-payment-service
GOSHOP_CONFIG=config.yaml go run ./cmd/goshop-aftersale-service
GOSHOP_CONFIG=config.yaml go run ./cmd/goshop-cart-service
GOSHOP_CONFIG=config.yaml go run ./cmd/goshop-promotion-service
GOSHOP_CONFIG=config.yaml go run ./cmd/goshop-inventory-service
GOSHOP_CONFIG=config.yaml go run ./cmd/goshop-scheduler-service
```

Use `deploy/Caddyfile.microservices` to route the current frontend API paths to these ports without changing the Vue app.

For Cloudflare Tunnel deployment, see `docs/deployment.md` and `deploy/cloudflared/config.yml`.

## Event Bus (NATS JetStream)

GoShop uses **NATS JetStream** as its enterprise-level event message bus.
Transactional business events are written to `outbox_events` in the same database transaction as the state change. The scheduler service polls pending events and publishes them to NATS JetStream under the subject pattern: `goshop.events.<aggregate>.<event>`.

Upon successful publication and NATS ACK, the scheduler marks the outbox event as sent. Idempotent consumers subscribe to these subjects via Queue Groups (e.g. `goshop-order-service-group`) and process incoming events wrapped in transactional Inbox tables (`inbox_events`) to prevent double-spending and ensure eventual consistency.

Current NATS JetStream events:

- `goshop.events.order.ordercreated`
- `goshop.events.order.ordercanceled`
- `goshop.events.payment.paymentsucceeded`
- `goshop.events.aftersale.aftersaleapplied`
- `goshop.events.aftersale.aftersalerejected`
- `goshop.events.refund.refundsucceeded`

---

## Internal Synchronous HTTP-RPC Contracts

To ensure absolute database isolation, services communicate synchronously via point-to-point HTTP-RPC under the `/api/internal/*` path.
All internal requests require a secure verification header:

- `X-Internal-Token`: `goshop_internal_communication_secret_token`

### 1. Product Service (:8102)

#### GET `/api/internal/products/:id`

- **Description**: Fetch Sku detail information by Sku ID.
- **Response**: Sku JSON object.

### 2. Inventory Service (:8103)

#### POST `/api/internal/inventory/reserve`

- **Description**: Lock/reserve stock for an order.
- **Payload**:

  ```json
  {
    "orderId": "GS-1700000-01",
    "userId": 1,
    "items": [
      { "skuId": 1, "qty": 2 }
    ]
  }
  ```

- **Response**: `{ "status": "success" }`

#### POST `/api/internal/inventory/release`

- **Description**: Release/unlock reserved stock for canceled orders.
- **Payload**: `{ "orderId": "GS-1700000-01" }`
- **Response**: `{ "status": "success" }`

### 3. Promotion/Coupon Service (:8104)

#### POST `/api/internal/promotion/candidates`

- **Description**: Fetch all eligible coupons and calculate potential discounts for a shopping cart.
- **Payload**:

  ```json
  {
    "userId": 1,
    "selectedUserCouponId": 0,
    "subtotal": 45000
  }
  ```

- **Response**: Array of eligible CouponCandidate objects.

#### POST `/api/internal/promotion/lock`

- **Description**: Lock a user coupon to prevent double-spending during order checkout.
- **Payload**:

  ```json
  {
    "userId": 1,
    "userCouponId": 1,
    "orderId": "GS-1700000-01",
    "subtotal": 45000
  }
  ```

- **Response**: `{ "discountAmount": 1000 }`

#### POST `/api/internal/promotion/release`

- **Description**: Release a locked user coupon for canceled orders.
- **Payload**:

  ```json
  {
    "userCouponId": 1,
    "orderId": "GS-1700000-01"
  }
  ```

- **Response**: `{ "status": "success" }`

---

## Build

```bash
# Compile all 9 microservices to bin/
./deploy/build.sh
```
