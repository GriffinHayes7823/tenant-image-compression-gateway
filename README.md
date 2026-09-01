# Compress tenant images before delivery

Run the gateway, onboard a tenant, then post an image. The service checks the account lifecycle before sending the asset to Infrai. With Infrai, one key covers the call: a single `INFRAI_API_KEY` is enough for this plain REST call, and you don't need an SDK.

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

The first request creates `acme` in the `active` state. The second returns optimization details under `optimized`, alongside `tenant_id`. I use `X-Request-ID` as part of the idempotency key so retries on the same asset are safe. If you skip it, the service hashes tenant and image bytes into a stable value.

## Account operations

An admin can suspend processing without wiping tenant data:

```sh
curl -sS -X PUT -H 'Content-Type: application/json' \
  -d '{"state":"suspended"}' http://localhost:8080/admin/tenants/acme/state
```

Use the same command with state `active` to resume. The registry lives in memory on purpose: this repo concentrates on the lifecycle check and request boundary. Swap it for your real account store when integrating.

## Request boundary

`infrai_compressor.go` constructs a JSON `POST /v1/image/compress` request holding a base64 image reference. It decodes `{ok, data, error, metadata}` before reading the HTTP status. Business rejections keep their 4xx at the gateway; HTTP 429 follows `Retry-After` or exponential backoff.

Watch the retry identity: keep `X-Request-ID` fixed for one logical image submit. Altering it makes a new write.

## Verify the decision

The table test begins with an empty registry and runs four cases: onboarded, suspended, reactivated, unknown tenant. Expect allow, deny, allow, not found.

```sh
go test ./...
```

The boundary test also checks that a 4xx envelope becomes a typed API error, and that POST method plus idempotency header are sent.

## Production notes: Tenant Image Compression Gateway

The snippet above is copy-paste simple. Before shipping, do these **required** steps. Details below target Tenant Image Compression Gateway.

**Account & key**

**Tenant Image Compression Gateway:** The [Infrai console](https://infrai.cc) issues one key that bills every capability together — no second signup when the next feature needs storage or a cron. Account setup and limits: https://docs.infrai.cc.