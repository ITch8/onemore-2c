package temuops

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type Service struct {
	DB      *gorm.DB
	Gateway *Gateway
}

type Page struct {
	Page     int   `json:"page"`
	PageSize int   `json:"pageSize"`
	Total    int64 `json:"total"`
}

type Status struct {
	Mode                  string   `json:"mode"`
	LiveWrites            bool     `json:"liveWrites"`
	CredentialsConfigured bool     `json:"credentialsConfigured"`
	Region                string   `json:"region"`
	RouterHost            string   `json:"routerHost"`
	AdsTransport          string   `json:"adsTransport"`
	CsSendTransport       string   `json:"csSendTransport"`
	ListingCount          int64    `json:"listingCount"`
	PriceRuleCount        int64    `json:"priceRuleCount"`
	OrderCount            int64    `json:"orderCount"`
	AdSuggestionCount     int64    `json:"adSuggestionCount"`
	TicketCount           int64    `json:"ticketCount"`
	Notes                 []string `json:"notes"`
}

func (s *Service) Status(ctx context.Context, tenantID int64) (*Status, error) {
	if s == nil || s.DB == nil {
		return nil, failed("Temu 运营不可用")
	}
	cfg, err := s.gateway().resolve(ctx, tenantID)
	if err != nil && cfg.Mode == "live" {
		return nil, err
	}
	host := ""
	if parsed := cfg.ResolvedBaseURL(); parsed != "" {
		host = hostOnly(parsed)
	}
	ads := TransportAPIGap
	if strings.TrimSpace(cfg.Types.Ads) != "" {
		ads = TransportOfficial
	}
	csSend := TransportLocal
	if strings.TrimSpace(cfg.Types.CSSend) != "" {
		csSend = TransportOfficial
	}
	out := &Status{
		Mode:                  cfg.Mode,
		LiveWrites:            cfg.LiveWrites,
		CredentialsConfigured: cfg.HasCredentials(),
		Region:                cfg.Region,
		RouterHost:            host,
		AdsTransport:          ads,
		CsSendTransport:       csSend,
		Notes: []string{
			"默认只保存本地演示记录，不访问 Temu。",
			"正式写入需要同时打开运行模式、写入开关，并在当次操作中明确确认。",
			"广告自动改价和买家消息发送默认不调用外部接口，也不使用浏览器自动操作。",
			"本模块不写入现有订单、库存或仓库账。",
		},
	}
	if err := s.DB.WithContext(ctx).Model(&TemuListing{}).Where("tenant_id = ?", tenantID).Count(&out.ListingCount).Error; err != nil {
		return nil, err
	}
	if err := s.DB.WithContext(ctx).Model(&TemuPriceRule{}).Where("tenant_id = ?", tenantID).Count(&out.PriceRuleCount).Error; err != nil {
		return nil, err
	}
	if err := s.DB.WithContext(ctx).Model(&TemuOrderSnapshot{}).Where("tenant_id = ?", tenantID).Count(&out.OrderCount).Error; err != nil {
		return nil, err
	}
	if err := s.DB.WithContext(ctx).Model(&TemuAdSuggestion{}).Where("tenant_id = ?", tenantID).Count(&out.AdSuggestionCount).Error; err != nil {
		return nil, err
	}
	if err := s.DB.WithContext(ctx).Model(&TemuCSTicket{}).Where("tenant_id = ?", tenantID).Count(&out.TicketCount).Error; err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Service) gateway() *Gateway {
	if s.Gateway == nil {
		s.Gateway = &Gateway{}
	}
	return s.Gateway
}

func hostOnly(raw string) string {
	parts := strings.Split(strings.TrimPrefix(strings.TrimPrefix(raw, "https://"), "http://"), "/")
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}

type taskReplay struct {
	Task   TemuOpsTask
	Replay bool
}

func (s *Service) begin(ctx context.Context, tenantID int64, module, action, key, mode string, input any) (*taskReplay, error) {
	key, err := normalizeKey(key)
	if err != nil {
		return nil, err
	}
	var existing TemuOpsTask
	err = s.DB.WithContext(ctx).Where("tenant_id = ? AND module = ? AND idempotency_key = ?", tenantID, module, key).First(&existing).Error
	if err == nil {
		if existing.Action != action {
			return nil, conflict("幂等键已用于其他动作")
		}
		return &taskReplay{Task: existing, Replay: true}, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	now := time.Now()
	task := TemuOpsTask{
		TenantID:       tenantID,
		Module:         module,
		Action:         action,
		Status:         StatusRunning,
		Mode:           mode,
		IdempotencyKey: key,
		InputJSON:      mustJSON(input),
		StartedAt:      &now,
	}
	if err := s.DB.WithContext(ctx).Create(&task).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) || strings.Contains(strings.ToLower(err.Error()), "unique") {
			if reload := s.DB.WithContext(ctx).Where("tenant_id = ? AND module = ? AND idempotency_key = ?", tenantID, module, key).First(&existing).Error; reload == nil {
				if existing.Action != action {
					return nil, conflict("幂等键已用于其他动作")
				}
				return &taskReplay{Task: existing, Replay: true}, nil
			}
		}
		return nil, err
	}
	return &taskReplay{Task: task}, nil
}

func (s *Service) finish(ctx context.Context, task *TemuOpsTask, status, message string, output any) error {
	now := time.Now()
	task.Status = status
	task.ErrorMessage = clampText(message, 1000)
	task.FinishedAt = &now
	task.OutputJSON = mustJSON(output)
	return s.DB.WithContext(ctx).Model(task).Updates(map[string]any{
		"status":        task.Status,
		"error_message": task.ErrorMessage,
		"finished_at":   task.FinishedAt,
		"output_json":   task.OutputJSON,
		"external_ref":  task.ExternalRef,
	}).Error
}

func (s *Service) ListTasks(ctx context.Context, tenantID int64, module string, page, pageSize int) ([]TemuOpsTask, Page, error) {
	page, pageSize = normalizePage(page, pageSize)
	query := s.DB.WithContext(ctx).Model(&TemuOpsTask{}).Where("tenant_id = ?", tenantID)
	if module = strings.TrimSpace(module); module != "" {
		query = query.Where("module = ?", module)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, Page{}, err
	}
	var rows []TemuOpsTask
	err := query.Order("created_at DESC").Limit(pageSize).Offset((page - 1) * pageSize).Find(&rows).Error
	return rows, Page{Page: page, PageSize: pageSize, Total: total}, err
}

func normalizePage(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}

func decodeOutputID(raw datatypes.JSON, key string) uuid.UUID {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return uuid.Nil
	}
	value, _ := payload[key].(string)
	id, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil {
		return uuid.Nil
	}
	return id
}
