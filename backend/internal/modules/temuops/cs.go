package temuops

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type PullTicketsResult struct {
	Task    TemuOpsTask    `json:"task"`
	Tickets []TemuCSTicket `json:"tickets"`
	Created int            `json:"created"`
	Updated int            `json:"updated"`
	Replay  bool           `json:"replay"`
}

type ReplyInput struct {
	IdempotencyKey string `json:"idempotencyKey"`
	Reply          string `json:"reply"`
	ConfirmLive    bool   `json:"confirmLive"`
}

type TicketResult struct {
	Task   TemuOpsTask  `json:"task"`
	Ticket TemuCSTicket `json:"ticket"`
	Replay bool         `json:"replay"`
}

func (s *Service) ListTickets(ctx context.Context, tenantID int64, page, pageSize int) ([]TemuCSTicket, Page, error) {
	page, pageSize = normalizePage(page, pageSize)
	query := s.DB.WithContext(ctx).Model(&TemuCSTicket{}).Where("tenant_id = ?", tenantID)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, Page{}, err
	}
	var rows []TemuCSTicket
	err := query.Order("updated_at DESC").Limit(pageSize).Offset((page - 1) * pageSize).Find(&rows).Error
	return rows, Page{Page: page, PageSize: pageSize, Total: total}, err
}

func (s *Service) PullTickets(ctx context.Context, tenantID int64, key string) (*PullTicketsResult, error) {
	cfg, err := s.gateway().resolve(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	replay, err := s.begin(ctx, tenantID, ModuleCS, "pull", key, cfg.Mode, map[string]any{"source": "temu"})
	if err != nil {
		return nil, err
	}
	if replay.Replay {
		rows, _, listErr := s.ListTickets(ctx, tenantID, 1, 20)
		if listErr != nil {
			return nil, listErr
		}
		return &PullTicketsResult{Task: replay.Task, Tickets: rows, Replay: true}, nil
	}
	incoming, pullErr := s.fetchTickets(ctx, tenantID, cfg.Mode)
	created, updated := 0, 0
	saved := make([]TemuCSTicket, 0, len(incoming))
	if pullErr == nil {
		for _, ticket := range incoming {
			row, isNew, saveErr := s.upsertTicket(ctx, tenantID, ticket)
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
	return &PullTicketsResult{Task: replay.Task, Tickets: saved, Created: created, Updated: updated}, nil
}

func (s *Service) fetchTickets(ctx context.Context, tenantID int64, mode string) ([]TemuCSTicket, error) {
	if mode != "live" {
		return []TemuCSTicket{
			{ExternalTicketID: "TM-CS-1001", Kind: "after_sales", BuyerMasked: "买家 A***", Subject: "尺码偏小", Body: "收到的商品比描述偏小，希望处理。", Status: StatusOpen, Mode: mode},
			{ExternalTicketID: "TM-CS-1002", Kind: "message", BuyerMasked: "买家 B***", Subject: "何时发货", Body: "请问订单什么时候发出？", Status: StatusOpen, Mode: mode},
		}, nil
	}
	cfg, err := s.gateway().resolve(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	result, _, err := s.gateway().call(ctx, tenantID, cfg.Types.AfterSales, map[string]any{"pageNumber": 1, "pageSize": 20})
	if err != nil {
		return nil, err
	}
	rawList := firstList(result.Result)
	out := make([]TemuCSTicket, 0, len(rawList))
	for _, item := range rawList {
		row, ok := item.(map[string]any)
		if !ok {
			continue
		}
		externalID := stringField(row, "afterSalesSn", "ticketId", "id")
		if externalID == "" {
			continue
		}
		out = append(out, TemuCSTicket{
			ExternalTicketID: clampText(externalID, 128),
			Kind:             "after_sales",
			BuyerMasked:      "买家已隐藏",
			Subject:          clampText(firstNonEmpty(stringField(row, "reason", "subject"), "售后咨询"), 256),
			Body:             clampText(stringField(row, "desc", "content", "body"), 4000),
			Status:           StatusOpen,
			Mode:             mode,
		})
	}
	return out, nil
}

func (s *Service) upsertTicket(ctx context.Context, tenantID int64, ticket TemuCSTicket) (TemuCSTicket, bool, error) {
	var existing TemuCSTicket
	err := s.DB.WithContext(ctx).Where("tenant_id = ? AND external_ticket_id = ?", tenantID, ticket.ExternalTicketID).First(&existing).Error
	if err == nil {
		if existing.Status == StatusOpen {
			existing.Subject = ticket.Subject
			existing.Body = ticket.Body
		}
		existing.Mode = ticket.Mode
		return existing, false, s.DB.WithContext(ctx).Save(&existing).Error
	}
	if err != gorm.ErrRecordNotFound {
		return TemuCSTicket{}, false, err
	}
	ticket.TenantID = tenantID
	return ticket, true, s.DB.WithContext(ctx).Create(&ticket).Error
}

func (s *Service) SuggestReply(ctx context.Context, tenantID int64, id uuid.UUID, key string) (*TicketResult, error) {
	return s.updateTicket(ctx, tenantID, id, key, "suggest", func(row *TemuCSTicket) {
		row.SuggestedReply = suggestedReply(row)
		row.Status = StatusSuggested
		row.Note = "已生成建议回复，尚未发送给买家"
	})
}

func suggestedReply(row *TemuCSTicket) string {
	subject := strings.TrimSpace(row.Subject)
	if subject == "" {
		subject = "您的问题"
	}
	return "您好，我们已收到关于「" + subject + "」的咨询，会先核实订单和售后信息，确认后再回复。此内容需要人工确认，系统不会自动发送。"
}

func (s *Service) ConfirmReply(ctx context.Context, tenantID int64, id uuid.UUID, in ReplyInput) (*TicketResult, error) {
	return s.updateTicket(ctx, tenantID, id, in.IdempotencyKey, "confirm", func(row *TemuCSTicket) {
		reply := clampText(in.Reply, 4000)
		if reply == "" {
			reply = clampText(row.SuggestedReply, 4000)
		}
		if reply == "" {
			row.Status = StatusFailed
			row.ErrorMessage = "请先生成或填写回复"
			return
		}
		row.ConfirmedReply = reply
		cfg, err := s.gateway().resolve(ctx, tenantID)
		if err != nil {
			row.Status = StatusFailed
			row.ErrorMessage = err.Error()
			return
		}
		if cfg.Mode != "live" || strings.TrimSpace(cfg.Types.CSSend) == "" {
			row.Status = StatusRepliedLocal
			row.Note = "回复已保存在本地，尚未发送给买家"
			return
		}
		if !cfg.LiveWrites || !in.ConfirmLive {
			row.Status = StatusNeedsConfirm
			row.Note = "正式发送未确认，尚未发送给买家"
			return
		}
		if _, _, callErr := s.gateway().call(ctx, tenantID, cfg.Types.CSSend, map[string]any{
			"ticketId": row.ExternalTicketID, "content": reply,
		}); callErr != nil {
			row.Status = StatusFailed
			row.ErrorMessage = clampText(callErr.Error(), 1000)
			return
		}
		row.Status = StatusSent
		row.Note = "已按人工确认内容请求发送"
	})
}

func (s *Service) updateTicket(ctx context.Context, tenantID int64, id uuid.UUID, key, action string, apply func(row *TemuCSTicket)) (*TicketResult, error) {
	var row TemuCSTicket
	if err := s.DB.WithContext(ctx).Where("tenant_id = ? AND id = ?", tenantID, id).First(&row).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, notFound("客服记录不存在")
		}
		return nil, err
	}
	cfg, err := s.gateway().resolve(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	replay, err := s.begin(ctx, tenantID, ModuleCS, action, key, cfg.Mode, map[string]any{"ticketId": id.String()})
	if err != nil {
		return nil, err
	}
	if replay.Replay {
		id := decodeOutputID(replay.Task.OutputJSON, "recordId")
		_ = s.DB.WithContext(ctx).Where("tenant_id = ? AND id = ?", tenantID, id).First(&row).Error
		return &TicketResult{Task: replay.Task, Ticket: row, Replay: true}, nil
	}
	apply(&row)
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
	return &TicketResult{Task: replay.Task, Ticket: row}, nil
}
