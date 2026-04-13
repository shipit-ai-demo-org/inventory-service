# inventory-service architecture

## Overview

`inventory-service` is the source of truth for warehouse stock levels at
CargoCloud. It tracks on-hand and reserved quantities per SKU per warehouse,
and exposes a small REST API plus an event consumer.

```
                 ┌──────────────┐   order.created    ┌───────────────────┐
                 │  orders-api  ├───────────────────▶│  events.Consumer  │
                 └──────────────┘  (broker bridge)   └─────────┬─────────┘
                                                               │ Reserve()
┌───────────────────┐   PUT /v1/stock/{sku}           ┌────────▼────────┐
│ warehouse-scanner ├────────────────────────────────▶│   store.Store   │
└───────────────────┘   (putaway completes)           └─────────────────┘
```

## Components

- **`internal/store`** — in-memory stock ledger guarded by a RWMutex. Each
  `StockRecord` holds `OnHand` and `Reserved`; available stock is the
  difference. Reservations are soft holds with a TTL; expired holds are swept
  lazily before new reservations are granted.
- **`internal/handlers`** — HTTP handlers for stock upserts/reads and
  reservation create/release.
- **`internal/events`** — long-polls the broker HTTP bridge for `order.*`
  events published by orders-api and converts `order.created` line items into
  reservations.

## Durability note

The in-memory store is rebuilt from the warehouse WMS snapshot on boot. A
Postgres-backed ledger is on the roadmap for FY27 Q1.
