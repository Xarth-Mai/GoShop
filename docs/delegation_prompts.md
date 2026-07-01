# Delegation Prompts

Use these prompts for non-core follow-up agents. They should not change the core transaction or microservice implementation without first reporting findings.

## Test Expansion Agent

Prompt:

```text
You are working in /home/lzzz/MyProjects/GoShop. Expand non-core test coverage for the completed Phase 1 transaction flow and microservice split. Do not change production behavior unless a test exposes a confirmed bug. Add focused tests for route registration, service entrypoint compile coverage, Caddy route assumptions where feasible, and frontend order list/detail behavior. Run `GOCACHE=/tmp/goshop-go-build go test ./...` and `npm run build` in web. Report files changed and any untested residual risks.
```

## Documentation QA Agent

Prompt:

```text
You are working in /home/lzzz/MyProjects/GoShop. Review README.md, goshop_enterprise_plan.md, docs/microservices.md, and deploy/ for consistency with the current code. Update documentation only. Ensure the docs clearly distinguish the monolith runtime from the transitional multi-process microservice runtime, list ports and commands, and identify remaining hard-split work. Do not edit Go or Vue source. Run markdown/link sanity checks with available local tools and report gaps.
```

## Deployment Script Agent

Prompt:

```text
You are working in /home/lzzz/MyProjects/GoShop. Add non-invasive deployment helpers for the existing microservice split: build scripts, optional systemd install notes, and a local smoke-run script that starts services only when explicitly invoked. Do not alter business code. Verify scripts with dry-run or shell syntax checks only unless services and databases are available. Preserve current ports and Caddy routing from deploy/Caddyfile.microservices.
```
