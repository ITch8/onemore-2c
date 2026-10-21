package temu

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	platformp "github.com/trademind-ai/trademind/backend/internal/providers/platform"
)

// Provider is the Temu shop connector. Seller automation flows live in the
// temuops module. This type does not implement the shared publish, order, or
// inventory worker interfaces, so those existing pipelines stay fail-closed.
type Provider struct {
	cfg  Config
	http *http.Client
}

func NewProvider(cfg Config, httpClient *http.Client) *Provider {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 20 * time.Second
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: cfg.Timeout}
	}
	return &Provider{cfg: cfg, http: httpClient}
}

func (p *Provider) Platform() string { return PlatformID }

func (p *Provider) Name() string { return "Temu" }

func (p *Provider) Status() string { return platformp.StatusBeta }

func (p *Provider) Capabilities() []platformp.Capability {
	return []platformp.Capability{
		platformp.CapShopInfo,
		platformp.CapProductPublish,
		platformp.CapInventorySync,
		platformp.CapOrderSync,
		platformp.CapLogisticsSync,
		platformp.CapCustomerMessage,
		platformp.CapRefundAfterSale,
	}
}

func (p *Provider) AuthSchema() platformp.AuthSchema {
	return platformp.AuthSchema{
		AuthType: "api_key",
		Fields: []platformp.AuthField{
			{Name: "region", Label: "站点区域", Type: "text", Required: false, Sensitive: false, Hint: "us、eu 或 global"},
		},
	}
}

func (p *Provider) AppConfigSchema() platformp.PlatformAppConfigSchema {
	return platformp.PlatformAppConfigSchema{
		GroupKey:    "platform_temu",
		Title:       "Temu 开放平台",
		Description: "填写 Temu Partner 应用参数。应用密钥和访问令牌加密保存。默认不访问正式接口。",
		Fields: []platformp.AppConfigField{
			{Name: "app_key", Label: "应用标识", Type: "text", Required: false, Sensitive: false},
			{Name: "app_secret", Label: "应用密钥", Type: "password", Required: false, Sensitive: true},
			{Name: "access_token", Label: "访问令牌", Type: "password", Required: false, Sensitive: true},
			{Name: "region", Label: "站点区域", Type: "select", Required: false, Sensitive: false, DefaultValue: "us", Options: []platformp.AppConfigOption{
				{Label: "美国", Value: "us"},
				{Label: "欧洲", Value: "eu"},
				{Label: "全球", Value: "global"},
			}},
			{Name: "api_base_url", Label: "接口地址", Type: "text", Required: false, Sensitive: false, Help: "留空时按区域使用 Temu 开放平台默认路由。"},
		},
	}
}

func (p *Provider) PublishConfigSchema() platformp.PlatformAppConfigSchema {
	return platformp.PlatformAppConfigSchema{
		GroupKey:    "platform_publish_temu",
		Title:       "Temu 刊登默认参数",
		Description: "仅作为上架草稿的默认类目和币种。不会因此向 Temu 提交商品。",
		Fields: []platformp.AppConfigField{
			{Name: "default_category_id", Label: "默认类目编号", Type: "text", Required: false, Sensitive: false},
			{Name: "default_currency", Label: "默认币种", Type: "text", Required: false, Sensitive: false, DefaultValue: "USD"},
			{Name: "publish_as_draft", Label: "默认只保存草稿", Type: "switch", Required: false, Sensitive: false, DefaultValue: true},
		},
	}
}

func (p *Provider) TestConnection(ctx context.Context, req platformp.TestConnectionRequest) (*platformp.TestConnectionResult, error) {
	cfg := p.cfg
	if strings.TrimSpace(req.AppKey) != "" {
		cfg.AppKey = strings.TrimSpace(req.AppKey)
	}
	if strings.TrimSpace(req.AppSecret) != "" {
		cfg.AppSecret = strings.TrimSpace(req.AppSecret)
	}
	if strings.TrimSpace(req.AccessToken) != "" {
		cfg.AccessToken = strings.TrimSpace(req.AccessToken)
	}
	if req.Extra != nil && strings.TrimSpace(req.Extra["region"]) != "" {
		cfg.Region = strings.ToLower(strings.TrimSpace(req.Extra["region"]))
	}
	if cfg.Mode != ModeLive {
		return &platformp.TestConnectionResult{
			OK:      true,
			Message: "演示模式：未访问 Temu 开放接口",
			Region:  cfg.Region,
		}, nil
	}
	if !cfg.HasCredentials() {
		return nil, fmt.Errorf("正式接口缺少应用标识、应用密钥或访问令牌")
	}
	client := NewClient(cfg, p.http)
	callCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()
	if _, err := client.Call(callCtx, cfg.Types.Ping, nil); err != nil {
		return nil, fmt.Errorf("Temu 连接测试失败")
	}
	return &platformp.TestConnectionResult{
		OK:      true,
		Message: "已连通 Temu 开放接口",
		Region:  cfg.Region,
	}, nil
}
