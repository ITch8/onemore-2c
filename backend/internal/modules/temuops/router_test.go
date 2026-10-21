package temuops

import (
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRegisterExposesTemuOpsRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	Register(router.Group("/api/v1"), &Handler{})
	want := map[string]bool{
		"GET /api/v1/temu-ops/status":                       false,
		"GET /api/v1/temu-ops/tasks":                        false,
		"GET /api/v1/temu-ops/listings":                     false,
		"POST /api/v1/temu-ops/listings":                    false,
		"POST /api/v1/temu-ops/listings/:id/submit":         false,
		"GET /api/v1/temu-ops/price-stock/rules":            false,
		"POST /api/v1/temu-ops/price-stock/preview":         false,
		"POST /api/v1/temu-ops/price-stock/apply":           false,
		"GET /api/v1/temu-ops/orders":                       false,
		"POST /api/v1/temu-ops/orders/pull":                 false,
		"POST /api/v1/temu-ops/orders/:id/label":            false,
		"POST /api/v1/temu-ops/orders/:id/ship":             false,
		"GET /api/v1/temu-ops/ads/suggestions":              false,
		"POST /api/v1/temu-ops/ads/optimize":                false,
		"POST /api/v1/temu-ops/ads/suggestions/:id/confirm": false,
		"POST /api/v1/temu-ops/ads/suggestions/:id/apply":   false,
		"GET /api/v1/temu-ops/cs/tickets":                   false,
		"POST /api/v1/temu-ops/cs/tickets/pull":             false,
		"POST /api/v1/temu-ops/cs/tickets/:id/suggest":      false,
		"POST /api/v1/temu-ops/cs/tickets/:id/confirm":      false,
	}
	for _, route := range router.Routes() {
		if _, ok := want[route.Method+" "+route.Path]; ok {
			want[route.Method+" "+route.Path] = true
		}
	}
	for route, found := range want {
		if !found {
			t.Fatalf("missing route %s", route)
		}
	}
}
