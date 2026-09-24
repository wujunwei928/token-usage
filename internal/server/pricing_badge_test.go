package server

import (
	"context"
	"testing"
	"time"
)

// Ticket 08: a family-estimated model in today's usage flips the dashboard's
// CostEstimated flag (the「估算」badge on the cost card).
func TestDashboardCostEstimatedBadge(t *testing.T) {
	store, pricing, srv := newTestServer(t)
	_, token := seedUser(t, store, "est")
	today := time.Now().Format("2006-01-02")
	// glm-5.3 is absent from the seed snapshot → family-estimated pricing.
	postReport(t, srv, token, samplePayload("est1", today,
		HourRow{Hour: 9, Tool: "zcode", Model: "glm-5.3", Input: 1000, Output: 100, CacheRead: 8900},
	))
	user, _ := store.UserByName(context.Background(), "est")
	data := store.Dashboard(context.Background(), user, pricing)
	if !data.CostEstimated {
		t.Fatal("family-estimated model today must set CostEstimated")
	}
	if data.TodayCost <= 0 {
		t.Fatalf("derived cache rates must price the reads: cost = %v", data.TodayCost)
	}
	// The derived read rate (0.1×input) keeps the reads from being free.
	card, _ := pricing.Card("glm-5.3")
	want := (1000*card.Input + 100*card.Output + 8900*card.CacheRead) / 1e6
	if d := data.TodayCost - want; d > 1e-9 || d < -1e-9 {
		t.Fatalf("today cost = %v, want %v (card %+v)", data.TodayCost, want, card)
	}
}
