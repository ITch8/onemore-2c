package temuops

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/trademind-ai/trademind/backend/internal/providers/platform/temu"
	"gorm.io/gorm"
)

func newService(t *testing.T, cfg temu.Config) *Service {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:temu-ops-%s?mode=memory&cache=shared", uuid.NewString())), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&TemuOpsTask{}, &TemuListing{}, &TemuPriceRule{}, &TemuOrderSnapshot{}, &TemuFulfillment{}, &TemuAdSuggestion{}, &TemuCSTicket{}); err != nil {
		t.Fatal(err)
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = time.Second
	}
	if cfg.Mode == "" {
		cfg.Mode = temu.ModeMock
	}
	return &Service{DB: db, Gateway: &Gateway{Config: cfg, HTTP: &http.Client{Timeout: time.Second, Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("unexpected Temu network call")
		return nil, context.Canceled
	})}}}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestQuoteFollowPriceRespectsFloor(t *testing.T) {
	quote, err := QuoteFollowPrice(1000, 1500, 1200, 5000, 1100, 0)
	if err != nil {
		t.Fatal(err)
	}
	if quote.SuggestedPriceMinor != 1200 {
		t.Fatalf("suggested = %d", quote.SuggestedPriceMinor)
	}
	follow, err := QuoteFollowPrice(1000, 1500, 1200, 5000, 1800, 50)
	if err != nil {
		t.Fatal(err)
	}
	if follow.SuggestedPriceMinor != 1750 {
		t.Fatalf("follow = %d", follow.SuggestedPriceMinor)
	}
}

func TestCreateListingMockDoesNotCallTemuAndReplays(t *testing.T) {
	svc := newService(t, temu.Config{Mode: temu.ModeMock})
	in := CreateListingInput{
		IdempotencyKey: "listing-key-1",
		SourceURL:      "https://detail.1688.com/offer/1.html",
		Title:          "演示水杯",
		Currency:       "USD",
		SKUs:           []ListingSKU{{SKUCode: "CUP", PriceMinor: 1999, Stock: 8}},
		Submit:         true,
	}
	first, err := svc.CreateListing(context.Background(), 7, in)
	if err != nil {
		t.Fatal(err)
	}
	if first.Listing.Status != StatusDemoSaved || first.Listing.Mode != temu.ModeMock || first.Listing.ExternalGoodsID == "" {
		t.Fatalf("listing = %#v", first.Listing)
	}
	second, err := svc.CreateListing(context.Background(), 7, in)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Replay || second.Listing.ID != first.Listing.ID {
		t.Fatalf("replay = %#v", second)
	}
}

func TestLiveListingStaysUnsentWithoutWriteSwitch(t *testing.T) {
	svc := newService(t, temu.Config{
		Mode: temu.ModeLive, LiveWrites: false, AppKey: "key", AppSecret: "secret", AccessToken: "token", Region: "us",
		Types: temu.APITypes{Listing: "bg.local.goods.add"},
	})
	out, err := svc.CreateListing(context.Background(), 7, CreateListingInput{
		IdempotencyKey: "listing-live-1", Title: "正式水杯", Currency: "USD", Submit: true, ConfirmLive: true,
		SKUs: []ListingSKU{{SKUCode: "CUP", PriceMinor: 1000, Stock: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Listing.Status != StatusNeedsConfirm {
		t.Fatalf("status = %s", out.Listing.Status)
	}
}

func TestPullOrdersAndRejectBadTracking(t *testing.T) {
	svc := newService(t, temu.Config{Mode: temu.ModeMock})
	pulled, err := svc.PullOrders(context.Background(), 7, PullOrdersInput{IdempotencyKey: "orders-pull-1", PageSize: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(pulled.Orders) != 2 {
		t.Fatalf("orders = %d", len(pulled.Orders))
	}
	if _, err := svc.ConfirmShipment(context.Background(), 7, pulled.Orders[0].ID, FulfillmentInput{IdempotencyKey: "ship-bad", Carrier: "USPS", TrackingNo: "bad"}); err == nil {
		t.Fatal("expected invalid tracking number")
	}
	shipped, err := svc.ConfirmShipment(context.Background(), 7, pulled.Orders[0].ID, FulfillmentInput{
		IdempotencyKey: "ship-ok-1", Carrier: "USPS", TrackingNo: "9400111899",
	})
	if err != nil {
		t.Fatal(err)
	}
	if shipped.Fulfillment.Status != StatusShipmentConfirmed {
		t.Fatalf("fulfillment = %#v", shipped.Fulfillment)
	}
}

func TestAdApplyRecordsGapAndReplyStaysLocal(t *testing.T) {
	svc := newService(t, temu.Config{Mode: temu.ModeLive, LiveWrites: true, AppKey: "key", AppSecret: "secret", AccessToken: "token"})
	ad, err := svc.OptimizeAd(context.Background(), 7, AdInput{
		IdempotencyKey: "ads-opt-1", CampaignName: "夏季", CurrentBudgetMinor: 10000, SpendMinor: 8000, RevenueMinor: 9000, TargetRoasBps: 20000,
	})
	if err != nil {
		t.Fatal(err)
	}
	confirmed, err := svc.ConfirmAd(context.Background(), 7, ad.Suggestion.ID, "ads-confirm-1")
	if err != nil {
		t.Fatal(err)
	}
	if confirmed.Suggestion.Status != StatusConfirmed {
		t.Fatalf("status = %s", confirmed.Suggestion.Status)
	}
	applied, err := svc.ApplyAd(context.Background(), 7, ad.Suggestion.ID, "ads-apply-1", true)
	if err != nil {
		t.Fatal(err)
	}
	if applied.Suggestion.Status != StatusAPIGap {
		t.Fatalf("apply status = %s note=%s", applied.Suggestion.Status, applied.Suggestion.Note)
	}
	svc.Gateway.Config.Mode = temu.ModeMock
	local, err := svc.PullTickets(context.Background(), 7, "cs-pull-demo")
	if err != nil {
		t.Fatal(err)
	}
	if len(local.Tickets) != 2 {
		t.Fatalf("tickets = %#v", local.Tickets)
	}
	suggested, err := svc.SuggestReply(context.Background(), 7, local.Tickets[0].ID, "cs-suggest-1")
	if err != nil {
		t.Fatal(err)
	}
	confirmedReply, err := svc.ConfirmReply(context.Background(), 7, suggested.Ticket.ID, ReplyInput{IdempotencyKey: "cs-confirm-1", Reply: "已帮您核实，今天安排发出。"})
	if err != nil {
		t.Fatal(err)
	}
	if confirmedReply.Ticket.Status != StatusRepliedLocal || confirmedReply.Ticket.ConfirmedReply == "" {
		t.Fatalf("ticket = %#v", confirmedReply.Ticket)
	}
}
