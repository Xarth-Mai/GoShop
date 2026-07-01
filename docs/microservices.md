# GoShop Microservice Split

This repository now supports two deployment shapes:

1. `go run .`: the existing Gin monolith with embedded frontend hosting.
2. `go run ./cmd/goshop-*-service`: single-repository, multi-process services routed by Caddy.

The split is intentionally a transitional service split. Services still share the same Go module, PostgreSQL schema, Redis instance, models, and handler package. That keeps current product behavior stable while moving runtime ownership to separate processes. The next split step is replacing shared database access with service APIs and local snapshots.

## Services

| Service | Default port | Entrypoint | Routes |
| --- | ---: | --- | --- |
| user | 8101 | `./cmd/goshop-user-service` | `/api/auth/*`, `/api/addresses*` |
| product | 8102 | `./cmd/goshop-product-service` | `/api/categories`, `/api/products*` |
| inventory | 8103 | `./cmd/goshop-inventory-service` | `/api/seckill` |
| promotion | 8104 | `./cmd/goshop-promotion-service` | `/api/coupons`, `/api/user-coupons*` |
| order | 8105 | `./cmd/goshop-order-service` | `/api/checkout/*`, `/api/orders*`, `/api/seckill` |
| payment | 8106 | `./cmd/goshop-payment-service` | `/api/pay`, `/api/payments*` |
| aftersale | 8107 | `./cmd/goshop-aftersale-service` | refund apply/audit routes |
| cart | 8108 | `./cmd/goshop-cart-service` | `/api/cart*` |
| scheduler | 8109 | `./cmd/goshop-scheduler-service` | `/health`, `/metrics`, delay worker, outbox publisher |

Each service supports:

- `GOSHOP_CONFIG=/path/to/config.yaml`
- `PORT=NNNN`
- service-specific port envs such as `GOSHOP_ORDER_PORT=8105`

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

## Event Bus

Transactional business events are written to `outbox_events` in the same database transaction as the state change. The scheduler service polls pending events and publishes them to Redis Stream `goshop:events`, then marks the event as sent.

Current emitted events:

- `OrderCreated`
- `OrderCanceled`
- `PaymentSucceeded`
- `AfterSaleApplied`
- `AfterSaleRejected`
- `RefundSucceeded`

This gives the service split a reliable handoff point without adding NATS/RabbitMQ yet. When a dedicated MQ is introduced, replace the Redis Stream publisher while keeping the transactional outbox table and event payloads stable.

## Build

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
```

## Remaining Hard Split Work

- Move service-owned tables to separate schemas or databases.
- Replace direct cross-domain reads with gRPC/HTTP APIs.
- Add Inbox-backed idempotent event consumers for `OrderCreated`, `PaymentSucceeded`, and `OrderCanceled`.
- Replace the Redis Stream outbox publisher with NATS JetStream or RabbitMQ when operating beyond single-node/lightweight deployment.
- Give each service its own DB user with least privilege.
- Move shared handlers into service-specific packages as API contracts stabilize.
