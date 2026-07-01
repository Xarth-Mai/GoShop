# Agent Test Prompts

The core implementation work is kept in this repository. The following prompts are intended for follow-up agents to expand non-core verification without changing the service contracts unless a real bug is found.

## Prompt 1: Microservice Smoke Flow

You are working in `/home/lzzz/MyProjects/GoShop`. Do not redesign the architecture. Verify the current physical split runtime with NATS JetStream and per-service databases.

Tasks:

1. Run `GOCACHE=/tmp/goshop-go-build go test ./...` and `npm run build` inside `web`.
2. Use `./deploy/build.sh` to build all service binaries.
3. Use `./deploy/smoke_run.sh start` to start NATS and all GoShop services, then check `./deploy/smoke_run.sh status`.
4. Through Caddy or direct service ports, manually verify: login as `test_user` with password `123456`, list products, add cart, checkout preview, create order, create payment, mock pay, view order detail.
5. Stop services with `./deploy/smoke_run.sh stop`.

Deliverable:

- A concise report with command outputs, failing logs if any, and exact file/line references for confirmed bugs only.

## Prompt 2: Frontend E2E Coverage

You are working in `/home/lzzz/MyProjects/GoShop`. Add Playwright or the repository's existing frontend test style for user-facing flows. Keep backend business logic unchanged unless the tests reveal a confirmed defect.

Flows to cover:

1. Register a user through `/register`, then log in.
2. Log in as `test_user` / `123456`.
3. Product detail -> cart -> checkout preview -> order creation.
4. Order detail payment panel: create payment, call mock pay, show payment/order status.
5. Refund request from order detail/list if UI support exists; otherwise document the missing UI and verify API manually.

Deliverable:

- Test files and a short report explaining how to run them.
- If browser automation needs seed data or service startup changes, document the minimal setup rather than embedding sleeps or brittle assumptions.

## Prompt 3: Contract And Failure-Mode Tests

You are working in `/home/lzzz/MyProjects/GoShop`. Expand Go tests around the internal HTTP-RPC contracts and failure behavior. Do not remove local fallback support in `core.CallInternalService`.

Coverage targets:

1. User address snapshot fallback and missing address behavior.
2. Cart clear-items fallback idempotency.
3. Order payment-source, refund-source, seckill-create, cancel-pending, expired-pending fallback behavior.
4. Payment service should create/settle payment rows without requiring local `orders` table.
5. AfterSale service should apply/approve/reject without requiring local `orders` or `inventory` tables, using internal RPC fallback in shared test DB.
6. Inventory reservation/restock should reject negative transitions and preserve non-negative available/reserved/sold.

Deliverable:

- Focused Go tests with clear package ownership.
- Keep `GOCACHE=/tmp/goshop-go-build go test ./...` green.

## Prompt 4: Swagger And Documentation QA

You are working in `/home/lzzz/MyProjects/GoShop`. Validate that public API docs remain accurate after microservice split.

Tasks:

1. Regenerate Swagger with `GOCACHE=/tmp/goshop-go-build go run github.com/swaggo/swag/cmd/swag init -g main.go -o docs`.
2. Confirm `docs/swagger.json` contains all public routes and no `/api/internal/*` routes.
3. Compare `docs/microservices.md`, `README.md`, and `deploy/Caddyfile.microservices` for route drift.
4. Check that docs do not refer to Redis Stream as the current event bus; current event bus is NATS JetStream.

Deliverable:

- A short doc QA report and patches for confirmed drift only.
