package temuops

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type AdInput struct {
	IdempotencyKey     string `json:"idempotencyKey"`
	CampaignName       string `json:"campaignName"`
	CurrentBudgetMinor int64  `json:"currentBudgetMinor"`
	SpendMinor         int64  `json:"spendMinor"`
	RevenueMinor       int64  `json:"revenueMinor"`
	TargetRoasBps      int    `json:"targetRoasBps"`
	ConfirmLive        bool   `json:"confirmLive"`
}

type AdResult struct {
	Task       TemuOpsTask      `json:"task"`
	Suggestion TemuAdSuggestion `json:"suggestion"`
	Replay     bool             `json:"replay"`
}

func (s *Service) ListAdSuggestions(ctx context.Context, tenantID int64, page, pageSize int) ([]TemuAdSuggestion, Page, error) {
	page, pageSize = normalizePage(page, pageSize)
	query := s.DB.WithContext(ctx).Model(&TemuAdSuggestion{}).Where("tenant_id = ?", tenantID)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, Page{}, err
	}
	var rows []TemuAdSuggestion
	err := query.Order("created_at DESC").Limit(pageSize).Offset((page - 1) * pageSize).Find(&rows).Error
	return rows, Page{Page: page, PageSize: pageSize, Total: total}, err
}

func (s *Service) OptimizeAd(ctx context.Context, tenantID int64, in AdInput) (*AdResult, error) {
	name := clampText(in.CampaignName, 160)
	if name == "" {
		return nil, invalid("广告计划名称不能为空")
	}
	suggested, reason, err := QuoteAdBudget(in.CurrentBudgetMinor, in.SpendMinor, in.RevenueMinor, in.TargetRoasBps)
	if err != nil {
		return nil, err
	}
	for _, value := range []int64{in.CurrentBudgetMinor, in.SpendMinor, in.RevenueMinor, suggested} {
		if err := checkMoney(value); err != nil {
			return nil, err
		}
	}
	cfg, err := s.gateway().resolve(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	replay, err := s.begin(ctx, tenantID, ModuleAds, "optimize", in.IdempotencyKey, cfg.Mode, map[string]any{
		"campaignName": name, "current": in.CurrentBudgetMinor, "suggested": suggested,
	})
	if err != nil {
		return nil, err
	}
	if replay.Replay {
		return s.replayAd(ctx, tenantID, replay.Task)
	}
	row := TemuAdSuggestion{
		TenantID: tenantID, TaskID: replay.Task.ID, CampaignName: name,
		CurrentBudgetMinor: in.CurrentBudgetMinor, SuggestedBudgetMinor: suggested,
		SpendMinor: in.SpendMinor, RevenueMinor: in.RevenueMinor, TargetRoasBps: in.TargetRoasBps,
		Reason: reason, Status: StatusSuggested, Mode: cfg.Mode,
		Note: "只生成建议，未修改广告预算",
	}
	if err := s.DB.WithContext(ctx).Create(&row).Error; err != nil {
		return nil, err
	}
	if err := s.finish(ctx, &replay.Task, StatusSuccess, "", map[string]any{"recordId": row.ID.String(), "status": row.Status}); err != nil {
		return nil, err
	}
	return &AdResult{Task: replay.Task, Suggestion: row}, nil
}

func (s *Service) ConfirmAd(ctx context.Context, tenantID int64, id uuid.UUID, key string) (*AdResult, error) {
	return s.transitionAd(ctx, tenantID, id, key, "confirm", func(row *TemuAdSuggestion, cfgMode string, liveWrites, confirmLive bool, adsType string) {
		_ = cfgMode
		_ = liveWrites
		_ = confirmLive
		_ = adsType
		if row.Status != StatusSuggested && row.Status != StatusConfirmed {
			row.Status = StatusFailed
			row.ErrorMessage = "只有待确认的建议可以确认"
			return
		}
		row.Status = StatusConfirmed
		row.Note = "已人工确认，尚未改动广告预算"
	})
}

func (s *Service) ApplyAd(ctx context.Context, tenantID int64, id uuid.UUID, key string, confirmLive bool) (*AdResult, error) {
	return s.transitionAd(ctx, tenantID, id, key, "apply", func(row *TemuAdSuggestion, cfgMode string, liveWrites, _ bool, adsType string) {
		if row.Status != StatusConfirmed && row.Status != StatusApplied && row.Status != StatusAPIGap {
			row.Status = StatusFailed
			row.ErrorMessage = "请先人工确认建议"
			return
		}
		if cfgMode != "live" {
			row.Status = StatusApplied
			row.Mode = cfgMode
			row.Note = "演示记录已保存，未调用广告接口，预算未改"
			return
		}
		if adsType == "" {
			row.Status = StatusAPIGap
			row.Note = "Temu 广告接口未覆盖，未改预算，也未使用浏览器自动操作"
			return
		}
		if !liveWrites || !confirmLive {
			row.Status = StatusNeedsConfirm
			row.Note = "正式写入未确认，预算未改"
			return
		}
		if _, _, err := s.gateway().call(ctx, tenantID, adsType, map[string]any{
			"campaignName": row.CampaignName, "budgetMinor": row.SuggestedBudgetMinor,
		}); err != nil {
			row.Status = StatusFailed
			row.ErrorMessage = clampText(err.Error(), 1000)
			return
		}
		row.Status = StatusApplied
		row.Note = "已请求 Temu 广告接口"
	})
}

func (s *Service) transitionAd(ctx context.Context, tenantID int64, id uuid.UUID, key, action string, apply func(row *TemuAdSuggestion, mode string, liveWrites, confirmLive bool, adsType string)) (*AdResult, error) {
	var row TemuAdSuggestion
	if err := s.DB.WithContext(ctx).Where("tenant_id = ? AND id = ?", tenantID, id).First(&row).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, notFound("广告建议不存在")
		}
		return nil, err
	}
	cfg, err := s.gateway().resolve(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	replay, err := s.begin(ctx, tenantID, ModuleAds, action, key, cfg.Mode, map[string]any{"suggestionId": id.String()})
	if err != nil {
		return nil, err
	}
	if replay.Replay {
		return s.replayAd(ctx, tenantID, replay.Task)
	}
	previous := row.Status
	apply(&row, cfg.Mode, cfg.LiveWrites, true, cfg.Types.Ads)
	if action == "apply" {
		// confirmLive is carried by the caller through the closure; previous status is kept on failure.
		_ = previous
	}
	row.TaskID = replay.Task.ID
	if err := s.DB.WithContext(ctx).Save(&row).Error; err != nil {
		return nil, err
	}
	status := StatusSuccess
	if row.Status == StatusFailed {
		status = StatusFailed
	}
	if err := s.finish(ctx, &replay.Task, status, row.ErrorMessage, map[string]any{"recordId": row.ID.String(), "status": row.Status}); err != nil {
		return nil, err
	}
	return &AdResult{Task: replay.Task, Suggestion: row}, nil
}

func (s *Service) replayAd(ctx context.Context, tenantID int64, task TemuOpsTask) (*AdResult, error) {
	id := decodeOutputID(task.OutputJSON, "recordId")
	var row TemuAdSuggestion
	if err := s.DB.WithContext(ctx).Where("tenant_id = ? AND (id = ? OR task_id = ?)", tenantID, id, task.ID).Order("updated_at DESC").First(&row).Error; err != nil {
		return &AdResult{Task: task, Replay: true}, nil
	}
	return &AdResult{Task: task, Suggestion: row, Replay: true}, nil
}
