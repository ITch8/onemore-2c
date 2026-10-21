package temuops

import (
	"errors"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/trademind-ai/trademind/backend/internal/modules/operationlog"
	"github.com/trademind-ai/trademind/backend/internal/pkg/adminperm"
	"github.com/trademind-ai/trademind/backend/internal/pkg/httpapi"
	"github.com/trademind-ai/trademind/backend/internal/pkg/response"
)

const maxJSONBody = int64(64 << 10)

type Handler struct {
	Svc   *Service
	OpLog *operationlog.Service
}

func (h *Handler) authorize(c *gin.Context, permission string) (int64, *adminperm.Principal, bool) {
	if h == nil || h.Svc == nil || h.Svc.DB == nil {
		response.Fail(c, 500, response.CodeInternalError, "Temu 运营不可用")
		return 0, nil, false
	}
	principal, err := adminperm.LoadPrincipal(c, h.Svc.DB)
	write := permission == adminperm.PermTemuOperate
	if err != nil || principal == nil || !principal.Can(permission) || (write && principal.IsReadonly()) {
		response.Fail(c, 403, response.CodeForbidden, "无权访问 Temu 运营")
		return 0, nil, false
	}
	tenantID, err := adminperm.TenantIDFromGin(c)
	if err != nil {
		response.Fail(c, 403, response.CodeForbidden, "缺少租户信息")
		return 0, nil, false
	}
	return tenantID, principal, true
}

func (h *Handler) bind(c *gin.Context, dst any) bool {
	if err := httpapi.BindStrictJSON(c, dst, maxJSONBody); err != nil {
		response.Fail(c, 400, response.CodeBadRequest, "请求格式无效")
		return false
	}
	return true
}

func (h *Handler) id(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(strings.TrimSpace(c.Param("id")))
	if err != nil || id == uuid.Nil {
		response.Fail(c, 400, response.CodeBadRequest, "编号格式无效")
		return uuid.Nil, false
	}
	return id, true
}

func (h *Handler) fail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrInvalid), errors.Is(err, ErrCredentials), errors.Is(err, ErrAPIGap):
		response.Fail(c, 400, response.CodeBadRequest, err.Error())
	case errors.Is(err, ErrNotFound):
		response.Fail(c, 404, response.CodeNotFound, err.Error())
	case errors.Is(err, ErrConflict):
		response.Fail(c, 409, response.CodeBadRequest, err.Error())
	default:
		response.HandleError(c, err)
	}
}

func (h *Handler) audit(c *gin.Context, tenantID int64, principal *adminperm.Principal, action, resourceID, message string) {
	if h.OpLog == nil {
		return
	}
	var actor *uuid.UUID
	if principal != nil && principal.UserID != uuid.Nil {
		id := principal.UserID
		actor = &id
	}
	_ = h.OpLog.Write(c, operationlog.WriteOpts{
		TenantID:    tenantID,
		AdminUserID: actor,
		Action:      action,
		Resource:    "temu_ops",
		ResourceID:  resourceID,
		Platform:    "temu",
		Status:      "success",
		Message:     message,
	})
}

func auditID(primary, fallback uuid.UUID) string {
	if primary != uuid.Nil {
		return primary.String()
	}
	return fallback.String()
}

func pageQuery(c *gin.Context) (int, int) {
	page, _ := strconv.Atoi(c.Query("page"))
	pageSize, _ := strconv.Atoi(c.Query("pageSize"))
	return page, pageSize
}

func (h *Handler) Status(c *gin.Context) {
	tenantID, _, ok := h.authorize(c, adminperm.PermTemuView)
	if !ok {
		return
	}
	out, err := h.Svc.Status(c.Request.Context(), tenantID)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.OK(c, out)
}

func (h *Handler) ListTasks(c *gin.Context) {
	tenantID, _, ok := h.authorize(c, adminperm.PermTemuView)
	if !ok {
		return
	}
	pageNo, pageSize := pageQuery(c)
	rows, page, err := h.Svc.ListTasks(c.Request.Context(), tenantID, c.Query("module"), pageNo, pageSize)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.OK(c, gin.H{"list": rows, "pagination": page})
}

func (h *Handler) ListListings(c *gin.Context) {
	tenantID, _, ok := h.authorize(c, adminperm.PermTemuView)
	if !ok {
		return
	}
	pageNo, pageSize := pageQuery(c)
	rows, page, err := h.Svc.ListListings(c.Request.Context(), tenantID, pageNo, pageSize)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.OK(c, gin.H{"list": rows, "pagination": page})
}

func (h *Handler) CreateListing(c *gin.Context) {
	tenantID, principal, ok := h.authorize(c, adminperm.PermTemuOperate)
	if !ok {
		return
	}
	var in CreateListingInput
	if !h.bind(c, &in) {
		return
	}
	out, err := h.Svc.CreateListing(c.Request.Context(), tenantID, in)
	if err != nil {
		h.fail(c, err)
		return
	}
	h.audit(c, tenantID, principal, "temu.listing.create", auditID(out.Listing.ID, out.Task.ID), "保存 Temu 上架记录")
	response.OK(c, out)
}

func (h *Handler) SubmitListing(c *gin.Context) {
	tenantID, principal, ok := h.authorize(c, adminperm.PermTemuOperate)
	if !ok {
		return
	}
	id, ok := h.id(c)
	if !ok {
		return
	}
	var in struct {
		IdempotencyKey string `json:"idempotencyKey"`
		ConfirmLive    bool   `json:"confirmLive"`
	}
	if !h.bind(c, &in) {
		return
	}
	out, err := h.Svc.SubmitListing(c.Request.Context(), tenantID, id, in.IdempotencyKey, in.ConfirmLive)
	if err != nil {
		h.fail(c, err)
		return
	}
	h.audit(c, tenantID, principal, "temu.listing.submit", id.String(), "提交 Temu 上架记录")
	response.OK(c, out)
}

func (h *Handler) ListPriceRules(c *gin.Context) {
	tenantID, _, ok := h.authorize(c, adminperm.PermTemuView)
	if !ok {
		return
	}
	pageNo, pageSize := pageQuery(c)
	rows, page, err := h.Svc.ListPriceRules(c.Request.Context(), tenantID, pageNo, pageSize)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.OK(c, gin.H{"list": rows, "pagination": page})
}

func (h *Handler) PreviewPrice(c *gin.Context) {
	tenantID, _, ok := h.authorize(c, adminperm.PermTemuView)
	if !ok {
		return
	}
	_ = tenantID
	var in PriceInput
	if !h.bind(c, &in) {
		return
	}
	out, err := h.Svc.PreviewPrice(c.Request.Context(), in)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.OK(c, out)
}

func (h *Handler) ApplyPrice(c *gin.Context) {
	tenantID, principal, ok := h.authorize(c, adminperm.PermTemuOperate)
	if !ok {
		return
	}
	var in PriceInput
	if !h.bind(c, &in) {
		return
	}
	out, err := h.Svc.ApplyPrice(c.Request.Context(), tenantID, in)
	if err != nil {
		h.fail(c, err)
		return
	}
	ruleID := out.Task.ID
	if out.Rule != nil {
		ruleID = out.Rule.ID
	}
	h.audit(c, tenantID, principal, "temu.price.apply", ruleID.String(), "记录 Temu 价格与库存")
	response.OK(c, out)
}

func (h *Handler) ListOrders(c *gin.Context) {
	tenantID, _, ok := h.authorize(c, adminperm.PermTemuView)
	if !ok {
		return
	}
	pageNo, pageSize := pageQuery(c)
	rows, page, err := h.Svc.ListOrders(c.Request.Context(), tenantID, pageNo, pageSize)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.OK(c, gin.H{"list": rows, "pagination": page})
}

func (h *Handler) PullOrders(c *gin.Context) {
	tenantID, principal, ok := h.authorize(c, adminperm.PermTemuOperate)
	if !ok {
		return
	}
	var in PullOrdersInput
	if !h.bind(c, &in) {
		return
	}
	out, err := h.Svc.PullOrders(c.Request.Context(), tenantID, in)
	if err != nil {
		h.fail(c, err)
		return
	}
	h.audit(c, tenantID, principal, "temu.order.pull", out.Task.ID.String(), "拉取 Temu 订单")
	response.OK(c, out)
}

func (h *Handler) PrepareLabel(c *gin.Context) {
	h.fulfill(c, "label")
}

func (h *Handler) ConfirmShipment(c *gin.Context) {
	h.fulfill(c, "ship")
}

func (h *Handler) fulfill(c *gin.Context, action string) {
	tenantID, principal, ok := h.authorize(c, adminperm.PermTemuOperate)
	if !ok {
		return
	}
	id, ok := h.id(c)
	if !ok {
		return
	}
	var in FulfillmentInput
	if !h.bind(c, &in) {
		return
	}
	var out *FulfillmentResult
	var err error
	auditAction := "temu.order.label"
	message := "准备 Temu 面单"
	if action == "ship" {
		out, err = h.Svc.ConfirmShipment(c.Request.Context(), tenantID, id, in)
		auditAction = "temu.order.ship"
		message = "回传 Temu 发货"
	} else {
		out, err = h.Svc.PrepareLabel(c.Request.Context(), tenantID, id, in)
	}
	if err != nil {
		h.fail(c, err)
		return
	}
	h.audit(c, tenantID, principal, auditAction, id.String(), message)
	response.OK(c, out)
}

func (h *Handler) ListAds(c *gin.Context) {
	tenantID, _, ok := h.authorize(c, adminperm.PermTemuView)
	if !ok {
		return
	}
	pageNo, pageSize := pageQuery(c)
	rows, page, err := h.Svc.ListAdSuggestions(c.Request.Context(), tenantID, pageNo, pageSize)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.OK(c, gin.H{"list": rows, "pagination": page})
}

func (h *Handler) OptimizeAd(c *gin.Context) {
	tenantID, principal, ok := h.authorize(c, adminperm.PermTemuOperate)
	if !ok {
		return
	}
	var in AdInput
	if !h.bind(c, &in) {
		return
	}
	out, err := h.Svc.OptimizeAd(c.Request.Context(), tenantID, in)
	if err != nil {
		h.fail(c, err)
		return
	}
	h.audit(c, tenantID, principal, "temu.ads.optimize", out.Suggestion.ID.String(), "生成 Temu 广告建议")
	response.OK(c, out)
}

func (h *Handler) ConfirmAd(c *gin.Context) {
	tenantID, principal, ok := h.authorize(c, adminperm.PermTemuOperate)
	if !ok {
		return
	}
	id, ok := h.id(c)
	if !ok {
		return
	}
	var in struct {
		IdempotencyKey string `json:"idempotencyKey"`
	}
	if !h.bind(c, &in) {
		return
	}
	out, err := h.Svc.ConfirmAd(c.Request.Context(), tenantID, id, in.IdempotencyKey)
	if err != nil {
		h.fail(c, err)
		return
	}
	h.audit(c, tenantID, principal, "temu.ads.confirm", id.String(), "确认 Temu 广告建议")
	response.OK(c, out)
}

func (h *Handler) ApplyAd(c *gin.Context) {
	tenantID, principal, ok := h.authorize(c, adminperm.PermTemuOperate)
	if !ok {
		return
	}
	id, ok := h.id(c)
	if !ok {
		return
	}
	var in struct {
		IdempotencyKey string `json:"idempotencyKey"`
		ConfirmLive    bool   `json:"confirmLive"`
	}
	if !h.bind(c, &in) {
		return
	}
	out, err := h.Svc.ApplyAd(c.Request.Context(), tenantID, id, in.IdempotencyKey, in.ConfirmLive)
	if err != nil {
		h.fail(c, err)
		return
	}
	h.audit(c, tenantID, principal, "temu.ads.apply", id.String(), "记录 Temu 广告执行")
	response.OK(c, out)
}

func (h *Handler) ListTickets(c *gin.Context) {
	tenantID, _, ok := h.authorize(c, adminperm.PermTemuView)
	if !ok {
		return
	}
	pageNo, pageSize := pageQuery(c)
	rows, page, err := h.Svc.ListTickets(c.Request.Context(), tenantID, pageNo, pageSize)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.OK(c, gin.H{"list": rows, "pagination": page})
}

func (h *Handler) PullTickets(c *gin.Context) {
	tenantID, principal, ok := h.authorize(c, adminperm.PermTemuOperate)
	if !ok {
		return
	}
	var in struct {
		IdempotencyKey string `json:"idempotencyKey"`
	}
	if !h.bind(c, &in) {
		return
	}
	out, err := h.Svc.PullTickets(c.Request.Context(), tenantID, in.IdempotencyKey)
	if err != nil {
		h.fail(c, err)
		return
	}
	h.audit(c, tenantID, principal, "temu.cs.pull", out.Task.ID.String(), "拉取 Temu 客服与售后")
	response.OK(c, out)
}

func (h *Handler) SuggestReply(c *gin.Context) {
	tenantID, principal, ok := h.authorize(c, adminperm.PermTemuOperate)
	if !ok {
		return
	}
	id, ok := h.id(c)
	if !ok {
		return
	}
	var in struct {
		IdempotencyKey string `json:"idempotencyKey"`
	}
	if !h.bind(c, &in) {
		return
	}
	out, err := h.Svc.SuggestReply(c.Request.Context(), tenantID, id, in.IdempotencyKey)
	if err != nil {
		h.fail(c, err)
		return
	}
	h.audit(c, tenantID, principal, "temu.cs.suggest", id.String(), "生成 Temu 客服建议回复")
	response.OK(c, out)
}

func (h *Handler) ConfirmReply(c *gin.Context) {
	tenantID, principal, ok := h.authorize(c, adminperm.PermTemuOperate)
	if !ok {
		return
	}
	id, ok := h.id(c)
	if !ok {
		return
	}
	var in ReplyInput
	if !h.bind(c, &in) {
		return
	}
	out, err := h.Svc.ConfirmReply(c.Request.Context(), tenantID, id, in)
	if err != nil {
		h.fail(c, err)
		return
	}
	h.audit(c, tenantID, principal, "temu.cs.confirm", id.String(), "确认 Temu 客服回复")
	response.OK(c, out)
}
