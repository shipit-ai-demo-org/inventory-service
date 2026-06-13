package events

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/shipit-ai-demo-org/inventory-service/internal/store"
)

// OrderEvent mirrors the envelope published by orders-api on the
// cargocloud.orders exchange (see shipit-ai-demo-org/orders-api).
type OrderEvent struct {
	Type       string `json:"type"`
	OrderID    string `json:"orderId"`
	OccurredAt string `json:"occurredAt"`
	Payload    struct {
		Items []struct {
			SKU      string `json:"sku"`
			Quantity int    `json:"quantity"`
		} `json:"items"`
	} `json:"payload"`
}

// Consumer drains the inventory-service queue bound to "order.*" via the
// broker's HTTP bridge. We long-poll rather than hold an AMQP connection so
// the pod stays stateless and restarts are cheap.
type Consumer struct {
	store     *store.Store
	bridgeURL string
	client    *http.Client
}

func NewConsumer(s *store.Store, bridgeURL string) *Consumer {
	return &Consumer{
		store:     s,
		bridgeURL: bridgeURL,
		client:    &http.Client{Timeout: 35 * time.Second},
	}
}

func (c *Consumer) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		evt, ok, err := c.poll(ctx)
		if err != nil {
			log.Printf("events: poll failed: %v", err)
			time.Sleep(5 * time.Second)
			continue
		}
		if !ok {
			continue
		}
		c.handle(ctx, evt)
	}
}

func (c *Consumer) poll(ctx context.Context) (OrderEvent, bool, error) {
	var evt OrderEvent
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.bridgeURL+"/queues/inventory-service/next?wait=30", nil)
	if err != nil {
		return evt, false, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return evt, false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		return evt, false, nil
	}
	if err := json.NewDecoder(resp.Body).Decode(&evt); err != nil {
		return evt, false, err
	}
	return evt, true, nil
}

func (c *Consumer) handle(ctx context.Context, evt OrderEvent) {
	switch evt.Type {
	case "order.created":
		for _, item := range evt.Payload.Items {
			if _, err := c.store.Reserve(ctx, evt.OrderID, item.SKU, item.Quantity, 30*time.Minute); err != nil {
				log.Printf("events: reservation failed for order %s sku %s: %v",
					evt.OrderID, item.SKU, err)
			}
		}
	case "order.shipped":
		// Shipment confirmation converts reservations to decrements in the
		// nightly reconciliation job; nothing to do inline yet.
	default:
		log.Printf("events: ignoring event type %q", evt.Type)
	}
}
