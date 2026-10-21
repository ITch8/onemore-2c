package temuops

import (
	"time"

	"github.com/google/uuid"
	"github.com/trademind-ai/trademind/backend/internal/pkg/model"
	"gorm.io/datatypes"
)

const (
	ModuleListing    = "listing"
	ModulePriceStock = "price_stock"
	ModuleOrders     = "orders"
	ModuleAds        = "ads"
	ModuleCS         = "cs"

	StatusPending           = "pending"
	StatusRunning           = "running"
	StatusSuccess           = "success"
	StatusFailed            = "failed"
	StatusCancelled         = "cancelled"
	StatusNeedsConfirm      = "needs_confirm"
	StatusDraft             = "draft"
	StatusDemoSaved         = "demo_saved"
	StatusSubmitted         = "submitted"
	StatusLabelReady        = "label_ready"
	StatusShipmentConfirmed = "shipment_confirmed"
	StatusSuggested         = "suggested"
	StatusConfirmed         = "confirmed"
	StatusApplied           = "applied"
	StatusAPIGap            = "api_gap"
	StatusOpen              = "open"
	StatusRepliedLocal      = "replied_local"
	StatusSent              = "sent"
	StatusActive            = "active"
	StatusPendingShipment   = "pending_shipment"

	TransportOfficial = "official_api"
	TransportAPIGap   = "api_gap"
	TransportLocal    = "local_only"
)

// TemuOpsTask records every seller-ops action, including input, output, and timing.
type TemuOpsTask struct {
	model.Base
	TenantID       int64          `gorm:"not null;uniqueIndex:ux_temu_ops_task_idem,priority:1;index" json:"tenantId"`
	Module         string         `gorm:"size:32;not null;uniqueIndex:ux_temu_ops_task_idem,priority:2;index" json:"module"`
	Action         string         `gorm:"size:64;not null;index" json:"action"`
	Status         string         `gorm:"size:32;not null;index" json:"status"`
	Mode           string         `gorm:"size:16;not null;index" json:"mode"`
	IdempotencyKey string         `gorm:"size:128;not null;uniqueIndex:ux_temu_ops_task_idem,priority:3" json:"idempotencyKey"`
	ExternalRef    string         `gorm:"size:128;index" json:"externalRef,omitempty"`
	ShopID         *uuid.UUID     `gorm:"type:char(36);index" json:"shopId,omitempty"`
	ProductID      *uuid.UUID     `gorm:"type:char(36);index" json:"productId,omitempty"`
	InputJSON      datatypes.JSON `gorm:"type:jsonb" json:"input,omitempty"`
	OutputJSON     datatypes.JSON `gorm:"type:jsonb" json:"output,omitempty"`
	ErrorMessage   string         `gorm:"size:1024" json:"errorMessage,omitempty"`
	StartedAt      *time.Time     `json:"startedAt,omitempty"`
	FinishedAt     *time.Time     `json:"finishedAt,omitempty"`
}

func (TemuOpsTask) TableName() string { return "temu_ops_tasks" }

// TemuListing is a local Temu listing draft or submission record.
type TemuListing struct {
	model.Base
	TenantID        int64          `gorm:"not null;index" json:"tenantId"`
	TaskID          uuid.UUID      `gorm:"type:char(36);index" json:"taskId"`
	ProductID       *uuid.UUID     `gorm:"type:char(36);index" json:"productId,omitempty"`
	ShopID          *uuid.UUID     `gorm:"type:char(36);index" json:"shopId,omitempty"`
	Source          string         `gorm:"size:64;index" json:"source,omitempty"`
	SourceURL       string         `gorm:"size:2048" json:"sourceUrl,omitempty"`
	Title           string         `gorm:"size:512;not null" json:"title"`
	Description     string         `gorm:"type:text" json:"description,omitempty"`
	Currency        string         `gorm:"size:8;not null" json:"currency"`
	CategoryID      string         `gorm:"size:128" json:"categoryId,omitempty"`
	Status          string         `gorm:"size:32;not null;index" json:"status"`
	Mode            string         `gorm:"size:16;not null;index" json:"mode"`
	ExternalGoodsID string         `gorm:"size:128;index" json:"externalGoodsId,omitempty"`
	ExternalURL     string         `gorm:"size:2048" json:"externalUrl,omitempty"`
	PayloadJSON     datatypes.JSON `gorm:"type:jsonb" json:"payload,omitempty"`
	ResultJSON      datatypes.JSON `gorm:"type:jsonb" json:"result,omitempty"`
	ErrorMessage    string         `gorm:"size:1024" json:"errorMessage,omitempty"`
}

func (TemuListing) TableName() string { return "temu_listings" }

// TemuPriceRule stores one price-follow or stock snapshot decision.
type TemuPriceRule struct {
	model.Base
	TenantID             int64      `gorm:"not null;index" json:"tenantId"`
	TaskID               uuid.UUID  `gorm:"type:char(36);index" json:"taskId"`
	ListingID            *uuid.UUID `gorm:"type:char(36);index" json:"listingId,omitempty"`
	SKUCode              string     `gorm:"size:128;index" json:"skuCode"`
	ExternalSKUID        string     `gorm:"size:128;index" json:"externalSkuId,omitempty"`
	CostMinor            int64      `gorm:"not null;default:0" json:"costMinor"`
	MarginBps            int        `gorm:"not null;default:0" json:"marginBps"`
	MinPriceMinor        int64      `gorm:"not null;default:0" json:"minPriceMinor"`
	MaxPriceMinor        int64      `gorm:"not null;default:0" json:"maxPriceMinor"`
	CompetitorPriceMinor int64      `gorm:"not null;default:0" json:"competitorPriceMinor"`
	FollowDeltaMinor     int64      `gorm:"not null;default:0" json:"followDeltaMinor"`
	SuggestedPriceMinor  int64      `gorm:"not null;default:0" json:"suggestedPriceMinor"`
	Stock                int        `gorm:"not null;default:0" json:"stock"`
	Currency             string     `gorm:"size:8;not null" json:"currency"`
	Status               string     `gorm:"size:32;not null;index" json:"status"`
	Mode                 string     `gorm:"size:16;not null" json:"mode"`
	Reason               string     `gorm:"size:512" json:"reason,omitempty"`
	ErrorMessage         string     `gorm:"size:1024" json:"errorMessage,omitempty"`
}

func (TemuPriceRule) TableName() string { return "temu_price_rules" }

// TemuOrderSnapshot is a Temu order copy owned by this module.
// It does not write the ERP order, inventory, or warehouse ledgers.
type TemuOrderSnapshot struct {
	model.Base
	TenantID          int64          `gorm:"not null;uniqueIndex:ux_temu_order_ext,priority:1;index" json:"tenantId"`
	ExternalOrderID   string         `gorm:"size:128;not null;uniqueIndex:ux_temu_order_ext,priority:2" json:"externalOrderId"`
	OrderNo           string         `gorm:"size:128;index" json:"orderNo"`
	Status            string         `gorm:"size:32;not null;index" json:"status"`
	Currency          string         `gorm:"size:8;not null" json:"currency"`
	TotalAmountMinor  int64          `gorm:"not null;default:0" json:"totalAmountMinor"`
	BuyerMasked       string         `gorm:"size:128" json:"buyerMasked,omitempty"`
	CountryCode       string         `gorm:"size:8;index" json:"countryCode,omitempty"`
	ItemSummary       string         `gorm:"size:512" json:"itemSummary,omitempty"`
	RawJSON           datatypes.JSON `gorm:"type:jsonb" json:"raw,omitempty"`
	Mode              string         `gorm:"size:16;not null" json:"mode"`
	PulledAt          time.Time      `json:"pulledAt"`
	FulfillmentStatus string         `gorm:"size:32;index" json:"fulfillmentStatus,omitempty"`
}

func (TemuOrderSnapshot) TableName() string { return "temu_order_snapshots" }

// TemuFulfillment records a local label or shipment-feedback attempt.
type TemuFulfillment struct {
	model.Base
	TenantID        int64          `gorm:"not null;index" json:"tenantId"`
	TaskID          uuid.UUID      `gorm:"type:char(36);index" json:"taskId"`
	OrderSnapshotID uuid.UUID      `gorm:"type:char(36);index;not null" json:"orderSnapshotId"`
	ExternalOrderID string         `gorm:"size:128;index" json:"externalOrderId"`
	Carrier         string         `gorm:"size:128" json:"carrier,omitempty"`
	TrackingNo      string         `gorm:"size:64;index" json:"trackingNo,omitempty"`
	LabelRef        string         `gorm:"size:256" json:"labelRef,omitempty"`
	Status          string         `gorm:"size:32;not null;index" json:"status"`
	Mode            string         `gorm:"size:16;not null" json:"mode"`
	Note            string         `gorm:"size:512" json:"note,omitempty"`
	ErrorMessage    string         `gorm:"size:1024" json:"errorMessage,omitempty"`
	ResultJSON      datatypes.JSON `gorm:"type:jsonb" json:"result,omitempty"`
}

func (TemuFulfillment) TableName() string { return "temu_fulfillments" }

// TemuAdSuggestion is a human-confirmed advertising recommendation.
type TemuAdSuggestion struct {
	model.Base
	TenantID             int64     `gorm:"not null;index" json:"tenantId"`
	TaskID               uuid.UUID `gorm:"type:char(36);index" json:"taskId"`
	CampaignName         string    `gorm:"size:160;not null;index" json:"campaignName"`
	CurrentBudgetMinor   int64     `gorm:"not null" json:"currentBudgetMinor"`
	SuggestedBudgetMinor int64     `gorm:"not null" json:"suggestedBudgetMinor"`
	SpendMinor           int64     `gorm:"not null;default:0" json:"spendMinor"`
	RevenueMinor         int64     `gorm:"not null;default:0" json:"revenueMinor"`
	TargetRoasBps        int       `gorm:"not null;default:0" json:"targetRoasBps"`
	Reason               string    `gorm:"size:512" json:"reason"`
	Status               string    `gorm:"size:32;not null;index" json:"status"`
	Mode                 string    `gorm:"size:16;not null" json:"mode"`
	Note                 string    `gorm:"size:512" json:"note,omitempty"`
	ErrorMessage         string    `gorm:"size:1024" json:"errorMessage,omitempty"`
}

func (TemuAdSuggestion) TableName() string { return "temu_ad_suggestions" }

// TemuCSTicket is a buyer message or after-sales ticket.
// Confirmed replies stay local unless an explicit send method is configured.
type TemuCSTicket struct {
	model.Base
	TenantID         int64  `gorm:"not null;uniqueIndex:ux_temu_cs_ticket,priority:1;index" json:"tenantId"`
	ExternalTicketID string `gorm:"size:128;not null;uniqueIndex:ux_temu_cs_ticket,priority:2" json:"externalTicketId"`
	Kind             string `gorm:"size:32;not null;index" json:"kind"`
	BuyerMasked      string `gorm:"size:128" json:"buyerMasked,omitempty"`
	Subject          string `gorm:"size:256" json:"subject"`
	Body             string `gorm:"type:text" json:"body,omitempty"`
	SuggestedReply   string `gorm:"type:text" json:"suggestedReply,omitempty"`
	ConfirmedReply   string `gorm:"type:text" json:"confirmedReply,omitempty"`
	Status           string `gorm:"size:32;not null;index" json:"status"`
	Mode             string `gorm:"size:16;not null" json:"mode"`
	Note             string `gorm:"size:512" json:"note,omitempty"`
	ErrorMessage     string `gorm:"size:1024" json:"errorMessage,omitempty"`
}

func (TemuCSTicket) TableName() string { return "temu_cs_tickets" }
