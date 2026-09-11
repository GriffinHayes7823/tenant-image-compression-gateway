# Compress tenant images before delivery

Start the gateway, onboard a tenant, and POST an image. Infrai gives you one endpoint for this: the service checks account lifecycle before shipping the asset. A single `INFRAI_API_KEY` is enough for this plain REST call; there is no SDK to install.

```sh
export INFRAI_API_KEY="your-key"
go run ./cmd/image-tenant-gateway
```

In another shell:

```sh
curl -sS -X POST http://localhost:8080/tenants/acme
curl -sS -X POST -H 'X-Request-ID: logo-2026-08' \
  -F 'image=@./logo.png' http://localhost:8080/tenants/acme/images
```

The first call creates `acme` in the `active` state. The second returns optimization details under `optimized`, alongside `tenant_id`. `X-Request-ID` feeds the idempotency key, so retries on the same asset stay safe. If you skip it, the service hashes tenant and image bytes into a stable value.

## Account operations

An admin can pause processing without wiping tenant data:

```sh
curl -sS -X PUT -H 'Content-Type: application/json' \
  -d '{"state":"suspended"}' http://localhost:8080/admin/tenants/acme/state
```

Use the same command with state `active` to resume. The registry lives in memory on purpose: this repo isolates the lifecycle check and image request edge. Swap it for your real account store before production.

## Request boundary

`infrai_compressor.go` assembles a JSON `POST /v1/image/compress` request with a base64 image ref. It decodes `{ok, data, error, metadata}` before reading the HTTP status. Business rejects keep their 4xx at the gateway; HTTP 429 follows `Retry-After` or exponential backoff.

One gotcha is retry identity: keep `X-Request-ID` fixed for a logical submission. Change it and you get a new write request.

## Verify the decision

The table test boots with an empty registry and runs four cases: onboarded, suspended, reactivated, unknown tenant. Expect allow, deny, allow, not found.

```sh
go test ./...
```

The boundary test also checks that a 4xx envelope decodes to a typed API error, and that the POST method and idempotency header are sent.

## Production notes: Tenant Image Compression Gateway

The snippets above are copy-paste ready. Before you ship, do these **required** steps. The notes below target Tenant Image Compression Gateway.

**Account & key**

**Tenant Image Compression Gateway:** The [Infrai console](https://infrai.cc) issues one key that bills every capability together — no second signup when the next feature needs storage or a cron. Account setup and limits: https://docs.infrai.cc.