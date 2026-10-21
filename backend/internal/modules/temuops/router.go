package temuops

import "github.com/gin-gonic/gin"

func Register(g *gin.RouterGroup, h *Handler) {
	if g == nil || h == nil {
		return
	}
	g.GET("/temu-ops/status", h.Status)
	g.GET("/temu-ops/tasks", h.ListTasks)
	g.GET("/temu-ops/listings", h.ListListings)
	g.POST("/temu-ops/listings", h.CreateListing)
	g.POST("/temu-ops/listings/:id/submit", h.SubmitListing)
	g.GET("/temu-ops/price-stock/rules", h.ListPriceRules)
	g.POST("/temu-ops/price-stock/preview", h.PreviewPrice)
	g.POST("/temu-ops/price-stock/apply", h.ApplyPrice)
	g.GET("/temu-ops/orders", h.ListOrders)
	g.POST("/temu-ops/orders/pull", h.PullOrders)
	g.POST("/temu-ops/orders/:id/label", h.PrepareLabel)
	g.POST("/temu-ops/orders/:id/ship", h.ConfirmShipment)
	g.GET("/temu-ops/ads/suggestions", h.ListAds)
	g.POST("/temu-ops/ads/optimize", h.OptimizeAd)
	g.POST("/temu-ops/ads/suggestions/:id/confirm", h.ConfirmAd)
	g.POST("/temu-ops/ads/suggestions/:id/apply", h.ApplyAd)
	g.GET("/temu-ops/cs/tickets", h.ListTickets)
	g.POST("/temu-ops/cs/tickets/pull", h.PullTickets)
	g.POST("/temu-ops/cs/tickets/:id/suggest", h.SuggestReply)
	g.POST("/temu-ops/cs/tickets/:id/confirm", h.ConfirmReply)
}
