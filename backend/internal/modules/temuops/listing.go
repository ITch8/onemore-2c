package temuops

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ListingSKU struct {
	SKUCode    string `json:"skuCode"`
	PriceMinor int64  `json:"priceMinor"`
	Stock      int    `json:"stock"`
}

type CreateListingInput struct {
	IdempotencyKey string       `json:"idempotencyKey"`
	ProductID      string       `json:"productId"`
	ShopID         string       `json:"shopId"`
	SourceURL      string       `json:"sourceUrl"`
	Title          string       `json:"title"`
	Description    string       `json:"description"`
	Currency       string       `json:"currency"`
	CategoryID     string       `json:"categoryId"`
	SKUs           []ListingSKU `json:"skus"`
	Submit         bool         `json:"submit"`
	ConfirmLive    bool         `json:"confirmLive"`
}

type ListingResult struct {
	Task    TemuOpsTask `json:"task"`
	Listing TemuListing `json:"listing"`
	Replay  bool        `json:"replay"`
}

func (s *Service) ListListings(ctx context.Context, tenantID int64, page, pageSize int) ([]TemuListing, Page, error) {
	page, pageSize = normalizePage(page, pageSize)
	query := s.DB.WithContext(ctx).Model(&TemuListing{}).Where("tenant_id = ?", tenantID)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, Page{}, err
	}
	var rows []TemuListing
	err := query.Order("created_at DESC").Limit(pageSize).Offset((page - 1) * pageSize).Find(&rows).Error
	return rows, Page{Page: page, PageSize: pageSize, Total: total}, err
}

func (s *Service) CreateListing(ctx context.Context, tenantID int64, in CreateListingInput) (*ListingResult, error) {
	prepared, err := s.prepareListing(ctx, tenantID, in)
	if err != nil {
		return nil, err
	}
	cfg, err := s.gateway().resolve(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	replay, err := s.begin(ctx, tenantID, ModuleListing, "create", in.IdempotencyKey, cfg.Mode, prepared.publicInput)
	if err != nil {
		return nil, err
	}
	if replay.Replay {
		return s.replayListing(ctx, tenantID, replay.Task)
	}
	row := prepared.row
	row.TaskID = replay.Task.ID
	row.Mode = cfg.Mode
	row.Status = StatusDraft
	if in.Submit {
		if err := s.applyListingSubmit(ctx, tenantID, &row, cfg.Mode, cfg.LiveWrites, in.ConfirmLive); err != nil {
			row.Status = StatusFailed
			row.ErrorMessage = clampText(err.Error(), 1000)
		}
	}
	if err := s.DB.WithContext(ctx).Create(&row).Error; err != nil {
		_ = s.finish(ctx, &replay.Task, StatusFailed, "上架记录保存失败", nil)
		return nil, err
	}
	status := StatusSuccess
	message := ""
	if row.Status == StatusFailed {
		status = StatusFailed
		message = row.ErrorMessage
	}
	replay.Task.ExternalRef = row.ExternalGoodsID
	if err := s.finish(ctx, &replay.Task, status, message, map[string]any{"recordId": row.ID.String(), "status": row.Status}); err != nil {
		return nil, err
	}
	return &ListingResult{Task: replay.Task, Listing: row}, nil
}

func (s *Service) SubmitListing(ctx context.Context, tenantID int64, id uuid.UUID, key string, confirmLive bool) (*ListingResult, error) {
	var row TemuListing
	if err := s.DB.WithContext(ctx).Where("tenant_id = ? AND id = ?", tenantID, id).First(&row).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, notFound("上架记录不存在")
		}
		return nil, err
	}
	cfg, err := s.gateway().resolve(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	replay, err := s.begin(ctx, tenantID, ModuleListing, "submit", key, cfg.Mode, map[string]any{"listingId": id.String(), "confirmLive": confirmLive})
	if err != nil {
		return nil, err
	}
	if replay.Replay {
		return s.replayListing(ctx, tenantID, replay.Task)
	}
	if err := s.applyListingSubmit(ctx, tenantID, &row, cfg.Mode, cfg.LiveWrites, confirmLive); err != nil {
		row.Status = StatusFailed
		row.ErrorMessage = clampText(err.Error(), 1000)
	}
	row.TaskID = replay.Task.ID
	row.Mode = cfg.Mode
	if err := s.DB.WithContext(ctx).Save(&row).Error; err != nil {
		return nil, err
	}
	status := StatusSuccess
	message := ""
	if row.Status == StatusFailed {
		status = StatusFailed
		message = row.ErrorMessage
	}
	replay.Task.ExternalRef = row.ExternalGoodsID
	if err := s.finish(ctx, &replay.Task, status, message, map[string]any{"recordId": row.ID.String(), "status": row.Status}); err != nil {
		return nil, err
	}
	return &ListingResult{Task: replay.Task, Listing: row}, nil
}

type preparedListing struct {
	row         TemuListing
	publicInput map[string]any
}

func (s *Service) prepareListing(ctx context.Context, tenantID int64, in CreateListingInput) (*preparedListing, error) {
	productID, err := optionalUUID(in.ProductID)
	if err != nil {
		return nil, err
	}
	shopID, err := optionalUUID(in.ShopID)
	if err != nil {
		return nil, err
	}
	sourceURL, err := validateSourceURL(in.SourceURL)
	if err != nil {
		return nil, err
	}
	title := clampText(in.Title, 500)
	description := clampText(in.Description, 8000)
	currency, err := normalizeCurrency(in.Currency)
	if err != nil {
		return nil, err
	}
	source := ""
	if productID != nil {
		loaded, loadErr := s.loadProduct(ctx, tenantID, *productID)
		if loadErr != nil {
			return nil, loadErr
		}
		source = loaded.Source
		if title == "" {
			title = clampText(firstNonEmpty(loaded.AITitle, loaded.Title), 500)
		}
		if description == "" {
			description = clampText(firstNonEmpty(loaded.AIDescription, loaded.Description), 8000)
		}
		if sourceURL == "" {
			sourceURL = loaded.SourceURL
		}
		if in.Currency == "" && loaded.Currency != "" {
			currency, err = normalizeCurrency(loaded.Currency)
			if err != nil {
				return nil, err
			}
		}
		if len(in.SKUs) == 0 {
			in.SKUs = loaded.SKUs
		}
	}
	if title == "" {
		return nil, invalid("标题不能为空")
	}
	if len(in.SKUs) == 0 {
		return nil, invalid("至少填写一个规格")
	}
	if len(in.SKUs) > 50 {
		return nil, invalid("规格数量过多")
	}
	for i := range in.SKUs {
		in.SKUs[i].SKUCode = clampText(in.SKUs[i].SKUCode, 128)
		if in.SKUs[i].SKUCode == "" {
			return nil, invalid("规格编码不能为空")
		}
		if err := checkMoney(in.SKUs[i].PriceMinor); err != nil {
			return nil, err
		}
		if in.SKUs[i].Stock < 0 || in.SKUs[i].Stock > 1_000_000 {
			return nil, invalid("库存超出允许范围")
		}
	}
	row := TemuListing{
		TenantID:    tenantID,
		ProductID:   productID,
		ShopID:      shopID,
		Source:      source,
		SourceURL:   sourceURL,
		Title:       title,
		Description: description,
		Currency:    currency,
		CategoryID:  clampText(in.CategoryID, 128),
		PayloadJSON: mustJSON(map[string]any{"skus": in.SKUs}),
	}
	return &preparedListing{row: row, publicInput: map[string]any{
		"title": title, "sourceUrl": sourceURL, "currency": currency, "submit": in.Submit, "skuCount": len(in.SKUs),
	}}, nil
}

func (s *Service) applyListingSubmit(ctx context.Context, tenantID int64, row *TemuListing, mode string, liveWrites, confirmLive bool) error {
	if mode != "live" {
		row.Status = StatusDemoSaved
		row.ExternalGoodsID = "demo-" + strings.ReplaceAll(uuid.NewString()[:8], "-", "")
		row.ResultJSON = mustJSON(map[string]string{"note": "演示记录已保存，未调用 Temu"})
		return nil
	}
	if !liveWrites || !confirmLive {
		row.Status = StatusNeedsConfirm
		row.ResultJSON = mustJSON(map[string]string{"note": "正式写入未确认，未调用 Temu"})
		return nil
	}
	result, _, err := s.gateway().call(ctx, tenantID, s.gateway().config().Types.Listing, map[string]any{
		"goodsName":  row.Title,
		"goodsDesc":  row.Description,
		"currency":   row.Currency,
		"categoryId": row.CategoryID,
		"sourceUrl":  row.SourceURL,
	})
	if err != nil {
		return err
	}
	row.Status = StatusSubmitted
	row.ExternalGoodsID = stringField(result.Result, "goodsId", "productId", "goods_id")
	row.ResultJSON = mustJSON(result.Result)
	return nil
}

func (s *Service) replayListing(ctx context.Context, tenantID int64, task TemuOpsTask) (*ListingResult, error) {
	id := decodeOutputID(task.OutputJSON, "recordId")
	var row TemuListing
	err := s.DB.WithContext(ctx).Where("tenant_id = ? AND (id = ? OR task_id = ?)", tenantID, id, task.ID).First(&row).Error
	if err != nil {
		return &ListingResult{Task: task, Replay: true}, nil
	}
	return &ListingResult{Task: task, Listing: row, Replay: true}, nil
}

type loadedProduct struct {
	Source        string
	SourceURL     string
	Title         string
	AITitle       string
	Description   string
	AIDescription string
	Currency      string
	SKUs          []ListingSKU
}

type sourceProduct struct {
	ID            uuid.UUID `gorm:"type:char(36);primaryKey"`
	TenantID      int64
	Source        string
	SourceURL     string
	Title         string
	AITitle       string `gorm:"column:ai_title"`
	Description   string
	AIDescription string `gorm:"column:ai_description"`
	Currency      string
	DeletedAt     gorm.DeletedAt
}

func (sourceProduct) TableName() string { return "products" }

type sourceSKU struct {
	ID        uuid.UUID `gorm:"type:char(36);primaryKey"`
	ProductID uuid.UUID `gorm:"type:char(36)"`
	SKUCode   string
	Price     *float64
	Stock     *int
}

func (sourceSKU) TableName() string { return "product_skus" }

func (s *Service) loadProduct(ctx context.Context, tenantID int64, id uuid.UUID) (*loadedProduct, error) {
	var row sourceProduct
	err := s.DB.WithContext(ctx).Where("tenant_id = ? AND id = ?", tenantID, id).First(&row).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, notFound("商品草稿不存在")
		}
		return nil, err
	}
	var skus []sourceSKU
	if err := s.DB.WithContext(ctx).Where("product_id = ?", id).Find(&skus).Error; err != nil {
		return nil, err
	}
	out := &loadedProduct{
		Source: row.Source, SourceURL: row.SourceURL, Title: row.Title, AITitle: row.AITitle,
		Description: row.Description, AIDescription: row.AIDescription, Currency: row.Currency,
	}
	for _, sku := range skus {
		item := ListingSKU{SKUCode: sku.SKUCode}
		if sku.Price != nil {
			item.PriceMinor = minorFromMajor(*sku.Price)
		}
		if sku.Stock != nil {
			item.Stock = *sku.Stock
		}
		if item.SKUCode != "" {
			out.SKUs = append(out.SKUs, item)
		}
	}
	return out, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
