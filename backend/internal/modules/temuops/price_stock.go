package temuops

import (
	"context"
)

type PriceInput struct {
	IdempotencyKey       string `json:"idempotencyKey"`
	ListingID            string `json:"listingId"`
	SKUCode              string `json:"skuCode"`
	ExternalSKUID        string `json:"externalSkuId"`
	CostMinor            int64  `json:"costMinor"`
	MarginBps            int    `json:"marginBps"`
	MinPriceMinor        int64  `json:"minPriceMinor"`
	MaxPriceMinor        int64  `json:"maxPriceMinor"`
	CompetitorPriceMinor int64  `json:"competitorPriceMinor"`
	FollowDeltaMinor     int64  `json:"followDeltaMinor"`
	Stock                int    `json:"stock"`
	Currency             string `json:"currency"`
	ConfirmLive          bool   `json:"confirmLive"`
}

type PriceResult struct {
	Quote  PriceQuote     `json:"quote"`
	Rule   *TemuPriceRule `json:"rule,omitempty"`
	Task   *TemuOpsTask   `json:"task,omitempty"`
	Replay bool           `json:"replay"`
}

func (s *Service) PreviewPrice(_ context.Context, in PriceInput) (*PriceResult, error) {
	quote, currency, err := validatePrice(in)
	if err != nil {
		return nil, err
	}
	_ = currency
	return &PriceResult{Quote: quote}, nil
}

func (s *Service) ListPriceRules(ctx context.Context, tenantID int64, page, pageSize int) ([]TemuPriceRule, Page, error) {
	page, pageSize = normalizePage(page, pageSize)
	query := s.DB.WithContext(ctx).Model(&TemuPriceRule{}).Where("tenant_id = ?", tenantID)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, Page{}, err
	}
	var rows []TemuPriceRule
	err := query.Order("created_at DESC").Limit(pageSize).Offset((page - 1) * pageSize).Find(&rows).Error
	return rows, Page{Page: page, PageSize: pageSize, Total: total}, err
}

func (s *Service) ApplyPrice(ctx context.Context, tenantID int64, in PriceInput) (*PriceResult, error) {
	quote, currency, err := validatePrice(in)
	if err != nil {
		return nil, err
	}
	listingID, err := optionalUUID(in.ListingID)
	if err != nil {
		return nil, err
	}
	cfg, err := s.gateway().resolve(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	replay, err := s.begin(ctx, tenantID, ModulePriceStock, "apply", in.IdempotencyKey, cfg.Mode, map[string]any{
		"skuCode": in.SKUCode, "suggested": quote.SuggestedPriceMinor, "stock": in.Stock,
	})
	if err != nil {
		return nil, err
	}
	if replay.Replay {
		var rule TemuPriceRule
		if err := s.DB.WithContext(ctx).Where("tenant_id = ? AND task_id = ?", tenantID, replay.Task.ID).First(&rule).Error; err == nil {
			return &PriceResult{Quote: quote, Rule: &rule, Task: &replay.Task, Replay: true}, nil
		}
		return &PriceResult{Quote: quote, Task: &replay.Task, Replay: true}, nil
	}
	rule := TemuPriceRule{
		TenantID: tenantID, TaskID: replay.Task.ID, ListingID: listingID,
		SKUCode: clampText(in.SKUCode, 128), ExternalSKUID: clampText(in.ExternalSKUID, 128),
		CostMinor: in.CostMinor, MarginBps: in.MarginBps, MinPriceMinor: in.MinPriceMinor,
		MaxPriceMinor: in.MaxPriceMinor, CompetitorPriceMinor: in.CompetitorPriceMinor,
		FollowDeltaMinor: in.FollowDeltaMinor, SuggestedPriceMinor: quote.SuggestedPriceMinor,
		Stock: in.Stock, Currency: currency, Mode: cfg.Mode, Reason: quote.Reason, Status: StatusDemoSaved,
	}
	if cfg.Mode == "live" {
		if !cfg.LiveWrites || !in.ConfirmLive {
			rule.Status = StatusNeedsConfirm
			rule.Reason = quote.Reason + "。正式写入未确认，未调用 Temu"
		} else if _, _, callErr := s.gateway().call(ctx, tenantID, cfg.Types.Price, map[string]any{
			"skuCode": rule.SKUCode, "priceMinor": rule.SuggestedPriceMinor, "currency": rule.Currency,
		}); callErr != nil {
			rule.Status = StatusFailed
			rule.ErrorMessage = clampText(callErr.Error(), 1000)
		} else if _, _, stockErr := s.gateway().call(ctx, tenantID, cfg.Types.Stock, map[string]any{
			"skuCode": rule.SKUCode, "stock": rule.Stock,
		}); stockErr != nil {
			rule.Status = StatusFailed
			rule.ErrorMessage = clampText(stockErr.Error(), 1000)
		} else {
			rule.Status = StatusSubmitted
		}
	}
	if err := s.DB.WithContext(ctx).Create(&rule).Error; err != nil {
		return nil, err
	}
	status := StatusSuccess
	if rule.Status == StatusFailed {
		status = StatusFailed
	}
	if err := s.finish(ctx, &replay.Task, status, rule.ErrorMessage, map[string]any{"recordId": rule.ID.String(), "status": rule.Status}); err != nil {
		return nil, err
	}
	return &PriceResult{Quote: quote, Rule: &rule, Task: &replay.Task}, nil
}

func validatePrice(in PriceInput) (PriceQuote, string, error) {
	if clampText(in.SKUCode, 128) == "" {
		return PriceQuote{}, "", invalid("规格编码不能为空")
	}
	if in.Stock < 0 || in.Stock > 1_000_000 {
		return PriceQuote{}, "", invalid("库存超出允许范围")
	}
	for _, value := range []int64{in.CostMinor, in.MinPriceMinor, in.MaxPriceMinor, in.CompetitorPriceMinor, in.FollowDeltaMinor} {
		if err := checkMoney(value); err != nil {
			return PriceQuote{}, "", err
		}
	}
	currency, err := normalizeCurrency(in.Currency)
	if err != nil {
		return PriceQuote{}, "", err
	}
	quote, err := QuoteFollowPrice(in.CostMinor, in.MarginBps, in.MinPriceMinor, in.MaxPriceMinor, in.CompetitorPriceMinor, in.FollowDeltaMinor)
	if err != nil {
		return PriceQuote{}, "", err
	}
	return quote, currency, nil
}
