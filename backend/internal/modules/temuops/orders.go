package temuops

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/trademind-ai/trademind/backend/internal/providers/platform/temu"
	"gorm.io/gorm"
)

type PullOrdersInput struct {
	IdempotencyKey string `json:"idempotencyKey"`
	PageSize       int    `json:"pageSize"`
}

type PullOrdersResult struct {
	Task    TemuOpsTask         `json:"task"`
	Orders  []TemuOrderSnapshot `json:"orders"`
	Created int                 `json:"created"`
	Updated int                 `json:"updated"`
	Replay  bool                `json:"replay"`
}

type FulfillmentInput struct {
	IdempotencyKey string `json:"idempotencyKey"`
	Carrier        string `json:"carrier"`
	TrackingNo     string `json:"trackingNo"`
	ConfirmLive    bool   `json:"confirmLive"`
}

type FulfillmentResult struct {
	Task        TemuOpsTask       `json:"task"`
	Order       TemuOrderSnapshot `json:"order"`
	Fulfillment TemuFulfillment   `json:"fulfillment"`
	Replay      bool              `json:"replay"`
}

func (s *Service) ListOrders(ctx context.Context, tenantID int64, page, pageSize int) ([]TemuOrderSnapshot, Page, error) {
	page, pageSize = normalizePage(page, pageSize)
	query := s.DB.WithContext(ctx).Model(&TemuOrderSnapshot{}).Where("tenant_id = ?", tenantID)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, Page{}, err
	}
	var rows []TemuOrderSnapshot
	err := query.Order("pulled_at DESC").Limit(pageSize).Offset((page - 1) * pageSize).Find(&rows).Error
	return rows, Page{Page: page, PageSize: pageSize, Total: total}, err
}

func (s *Service) PullOrders(ctx context.Context, tenantID int64, in PullOrdersInput) (*PullOrdersResult, error) {
	if in.PageSize <= 0 || in.PageSize > 50 {
		in.PageSize = 20
	}
	cfg, err := s.gateway().resolve(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	replay, err := s.begin(ctx, tenantID, ModuleOrders, "pull", in.IdempotencyKey, cfg.Mode, map[string]any{"pageSize": in.PageSize})
	if err != nil {
		return nil, err
	}
	if replay.Replay {
		rows, _, listErr := s.ListOrders(ctx, tenantID, 1, 20)
		if listErr != nil {
			return nil, listErr
		}
		return &PullOrdersResult{Task: replay.Task, Orders: rows, Replay: true}, nil
	}
	orders, mode, pullErr := s.fetchOrders(ctx, tenantID, cfg, in.PageSize)
	created, updated := 0, 0
	saved := make([]TemuOrderSnapshot, 0, len(orders))
	if pullErr == nil {
		for _, order := range orders {
			row, isNew, saveErr := s.upsertOrder(ctx, tenantID, order, mode)
			if saveErr != nil {
				pullErr = saveErr
				break
			}
			if isNew {
				created++
			} else {
				updated++
			}
			saved = append(saved, row)
		}
	}
	status := StatusSuccess
	message := ""
	if pullErr != nil {
		status = StatusFailed
		message = pullErr.Error()
	}
	if err := s.finish(ctx, &replay.Task, status, message, map[string]any{"created": created, "updated": updated}); err != nil {
		return nil, err
	}
	return &PullOrdersResult{Task: replay.Task, Orders: saved, Created: created, Updated: updated}, nil
}

func (s *Service) fetchOrders(ctx context.Context, tenantID int64, resolved temu.Config, pageSize int) ([]TemuOrderSnapshot, string, error) {
	if resolved.Mode != "live" {
		now := time.Now()
		return []TemuOrderSnapshot{
			{ExternalOrderID: "TM-DEMO-1001", OrderNo: "TM-DEMO-1001", Status: StatusPendingShipment, Currency: "USD", TotalAmountMinor: 1999, BuyerMasked: "买家 A***", CountryCode: "US", ItemSummary: "演示商品 A x1", Mode: resolved.Mode, PulledAt: now, FulfillmentStatus: StatusPendingShipment},
			{ExternalOrderID: "TM-DEMO-1002", OrderNo: "TM-DEMO-1002", Status: StatusPendingShipment, Currency: "USD", TotalAmountMinor: 850, BuyerMasked: "买家 B***", CountryCode: "US", ItemSummary: "演示商品 B x1", Mode: resolved.Mode, PulledAt: now, FulfillmentStatus: StatusPendingShipment},
		}, resolved.Mode, nil
	}
	result, _, err := s.gateway().call(ctx, tenantID, resolved.Types.Orders, map[string]any{"pageNumber": 1, "pageSize": pageSize})
	if err != nil {
		return nil, resolved.Mode, err
	}
	return mapRemoteOrders(result.Result, resolved.Mode), resolved.Mode, nil
}

func mapRemoteOrders(result map[string]any, mode string) []TemuOrderSnapshot {
	rawList := firstList(result)
	now := time.Now()
	out := make([]TemuOrderSnapshot, 0, len(rawList))
	for _, item := range rawList {
		row, ok := item.(map[string]any)
		if !ok {
			continue
		}
		externalID := stringField(row, "orderSn", "parentOrderSn", "order_sn")
		if externalID == "" {
			continue
		}
		out = append(out, TemuOrderSnapshot{
			ExternalOrderID:   clampText(externalID, 128),
			OrderNo:           clampText(firstNonEmpty(stringField(row, "parentOrderSn", "orderSn"), externalID), 128),
			Status:            StatusPendingShipment,
			Currency:          firstNonEmpty(stringField(row, "currency"), "USD"),
			TotalAmountMinor:  minorFromMajor(firstValue(row, "amount", "totalAmount", "payAmount")),
			BuyerMasked:       "买家已隐藏",
			CountryCode:       clampText(stringField(row, "region", "country"), 8),
			ItemSummary:       clampText(stringField(row, "goodsName", "itemSummary"), 500),
			RawJSON:           mustJSON(row),
			Mode:              mode,
			PulledAt:          now,
			FulfillmentStatus: StatusPendingShipment,
		})
	}
	return out
}

func firstList(result map[string]any) []any {
	for _, key := range []string{"orderList", "pageItems", "orders", "list"} {
		if rows, ok := result[key].([]any); ok {
			return rows
		}
	}
	return nil
}

func firstValue(row map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, ok := row[key]; ok {
			return value
		}
	}
	return nil
}

func (s *Service) upsertOrder(ctx context.Context, tenantID int64, order TemuOrderSnapshot, mode string) (TemuOrderSnapshot, bool, error) {
	var existing TemuOrderSnapshot
	err := s.DB.WithContext(ctx).Where("tenant_id = ? AND external_order_id = ?", tenantID, order.ExternalOrderID).First(&existing).Error
	if err == nil {
		existing.PulledAt = order.PulledAt
		existing.TotalAmountMinor = order.TotalAmountMinor
		existing.ItemSummary = order.ItemSummary
		existing.Mode = mode
		if existing.RawJSON == nil {
			existing.RawJSON = order.RawJSON
		}
		return existing, false, s.DB.WithContext(ctx).Save(&existing).Error
	}
	if err != gorm.ErrRecordNotFound {
		return TemuOrderSnapshot{}, false, err
	}
	order.TenantID = tenantID
	order.Mode = mode
	return order, true, s.DB.WithContext(ctx).Create(&order).Error
}

func (s *Service) PrepareLabel(ctx context.Context, tenantID int64, orderID uuid.UUID, in FulfillmentInput) (*FulfillmentResult, error) {
	return s.fulfill(ctx, tenantID, orderID, in, "label")
}

func (s *Service) ConfirmShipment(ctx context.Context, tenantID int64, orderID uuid.UUID, in FulfillmentInput) (*FulfillmentResult, error) {
	in.Carrier = clampText(in.Carrier, 128)
	in.TrackingNo = strings.TrimSpace(in.TrackingNo)
	if in.Carrier == "" || !trackingPattern.MatchString(in.TrackingNo) {
		return nil, invalid("请填写承运商和 4 到 64 位运单号")
	}
	return s.fulfill(ctx, tenantID, orderID, in, "ship")
}

func (s *Service) fulfill(ctx context.Context, tenantID int64, orderID uuid.UUID, in FulfillmentInput, action string) (*FulfillmentResult, error) {
	var order TemuOrderSnapshot
	if err := s.DB.WithContext(ctx).Where("tenant_id = ? AND id = ?", tenantID, orderID).First(&order).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, notFound("订单不存在")
		}
		return nil, err
	}
	cfg, err := s.gateway().resolve(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	replay, err := s.begin(ctx, tenantID, ModuleOrders, action, in.IdempotencyKey, cfg.Mode, map[string]any{
		"orderId": orderID.String(), "carrier": in.Carrier, "trackingNo": in.TrackingNo,
	})
	if err != nil {
		return nil, err
	}
	if replay.Replay {
		var row TemuFulfillment
		_ = s.DB.WithContext(ctx).Where("tenant_id = ? AND task_id = ?", tenantID, replay.Task.ID).First(&row).Error
		return &FulfillmentResult{Task: replay.Task, Order: order, Fulfillment: row, Replay: true}, nil
	}
	row := TemuFulfillment{
		TenantID: tenantID, TaskID: replay.Task.ID, OrderSnapshotID: order.ID,
		ExternalOrderID: order.ExternalOrderID, Carrier: in.Carrier, TrackingNo: in.TrackingNo, Mode: cfg.Mode,
	}
	if action == "label" {
		s.applyLabel(ctx, tenantID, &row, cfg)
	} else {
		s.applyShip(ctx, tenantID, &row, cfg, in.ConfirmLive)
	}
	if err := s.DB.WithContext(ctx).Create(&row).Error; err != nil {
		return nil, err
	}
	order.FulfillmentStatus = row.Status
	if err := s.DB.WithContext(ctx).Model(&order).Update("fulfillment_status", row.Status).Error; err != nil {
		return nil, err
	}
	status := StatusSuccess
	if row.Status == StatusFailed {
		status = StatusFailed
	}
	if err := s.finish(ctx, &replay.Task, status, row.ErrorMessage, map[string]any{"recordId": row.ID.String(), "status": row.Status}); err != nil {
		return nil, err
	}
	return &FulfillmentResult{Task: replay.Task, Order: order, Fulfillment: row}, nil
}

func (s *Service) applyLabel(ctx context.Context, tenantID int64, row *TemuFulfillment, cfg temu.Config) {
	if cfg.Mode != "live" {
		row.Status = StatusLabelReady
		row.LabelRef = "demo-label-" + row.ExternalOrderID
		row.Note = "演示面单引用已生成，未向 Temu 或承运商申请文件"
		return
	}
	if !cfg.LiveWrites {
		row.Status = StatusNeedsConfirm
		row.Note = "正式写入未开启，未申请面单"
		return
	}
	result, _, err := s.gateway().call(ctx, tenantID, cfg.Types.Label, map[string]any{"orderSn": row.ExternalOrderID})
	if err != nil {
		row.Status = StatusFailed
		row.ErrorMessage = clampText(err.Error(), 1000)
		return
	}
	row.Status = StatusLabelReady
	row.LabelRef = clampText(stringField(result.Result, "url", "documentUrl", "labelUrl"), 256)
	row.ResultJSON = mustJSON(result.Result)
	row.Note = "已请求 Temu 面单接口"
}

func (s *Service) applyShip(ctx context.Context, tenantID int64, row *TemuFulfillment, cfg temu.Config, confirmLive bool) {
	if cfg.Mode != "live" {
		row.Status = StatusShipmentConfirmed
		row.Note = "演示发货回传已保存，未通知 Temu"
		return
	}
	if !cfg.LiveWrites || !confirmLive {
		row.Status = StatusNeedsConfirm
		row.Note = "正式发货回传未确认，未通知 Temu"
		return
	}
	result, _, err := s.gateway().call(ctx, tenantID, cfg.Types.Ship, map[string]any{
		"orderSn": row.ExternalOrderID, "carrier": row.Carrier, "trackingNo": row.TrackingNo,
	})
	if err != nil {
		row.Status = StatusFailed
		row.ErrorMessage = clampText(err.Error(), 1000)
		return
	}
	row.Status = StatusShipmentConfirmed
	row.ResultJSON = mustJSON(result.Result)
	row.Note = "已请求 Temu 发货回传接口"
}
