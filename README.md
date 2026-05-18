# inventory-service

Warehouse inventory tracking for CargoCloud — stock levels, reservations and
replenishment. Written in Go 1.22 with a stdlib-only HTTP stack.

## Role at CargoCloud

When [orders-api](https://github.com/shipit-ai-demo-org/orders-api) emits an
`order.created` event, this service places soft reservations against warehouse
stock so pickers never race each other for the last unit. Physical intake and
putaway counts arrive from
[warehouse-scanner](https://github.com/shipit-ai-demo-org/warehouse-scanner)
via `PUT /v1/stock/{sku}`.

## API surface

```
GET    /healthz                      liveness
GET    /v1/stock/{sku}               read stock record
PUT    /v1/stock/{sku}               upsert on-hand count (warehouse intake)
POST   /v1/reservations              place a soft hold against stock
DELETE /v1/reservations/{id}         release a hold
```

## Concepts

| Term | Meaning |
| ---- | ------- |
| On hand | Units physically in the warehouse |
| Reserved | Units soft-held for unshipped orders |
| Available | `onHand - reserved` |
| Reorder point | Threshold below which replenishment is flagged |

## Local development

```bash
go run .
curl localhost:8080/healthz
```

Architecture notes live in [`docs/architecture.md`](docs/architecture.md).
