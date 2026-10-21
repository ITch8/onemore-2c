package temu

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	platformp "github.com/trademind-ai/trademind/backend/internal/providers/platform"
)

func TestCallSignsAndParsesOfficialSuccess(t *testing.T) {
	var seen string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		seen = string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"errorCode":1000000,"result":{"orderList":[{"orderSn":"PO1"}]}}`))
	}))
	defer server.Close()

	client := NewClient(Config{
		AppKey:            "key",
		AppSecret:         "secret",
		AccessToken:       "token",
		BaseURL:           server.URL,
		AllowUnlistedHost: true,
		Timeout:           5 * time.Second,
	}, server.Client())
	client.Now = func() time.Time { return time.Unix(1, 0) }
	result, err := client.Call(context.Background(), "bg.order.list.v2.get", map[string]any{"page": 1})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success {
		t.Fatal("expected success")
	}
	if !strings.Contains(seen, `"sign":`) || strings.Contains(seen, `"secret"`) {
		t.Fatalf("request body leaked or missed the signature: %s", seen)
	}
	orders, _ := result.Result["orderList"].([]any)
	if len(orders) != 1 {
		t.Fatalf("orders = %#v", result.Result["orderList"])
	}
}

func TestValidateBaseURLRejectsUnlistedHost(t *testing.T) {
	if err := ValidateBaseURL("https://example.com/openapi/router", false); err == nil {
		t.Fatal("expected unlisted host to be rejected")
	}
	if err := ValidateBaseURL("https://openapi-b-us.temu.com/openapi/router", false); err != nil {
		t.Fatal(err)
	}
	if err := ValidateBaseURL("http://127.0.0.1/router", false); err == nil {
		t.Fatal("loopback http must stay disabled unless explicitly allowed")
	}
}

func TestProviderTestConnectionMockDoesNotCallNetwork(t *testing.T) {
	provider := NewProvider(Config{Mode: ModeMock, Region: "us", Timeout: time.Second}, &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			t.Fatal("mock mode must not call Temu")
			return nil, context.Canceled
		}),
	})
	result, err := provider.TestConnection(context.Background(), platformp.TestConnectionRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK || !strings.Contains(result.Message, "演示模式") {
		t.Fatalf("result = %#v", result)
	}
}

func TestProviderTestConnectionLiveRequiresCredentials(t *testing.T) {
	provider := NewProvider(Config{Mode: ModeLive, Timeout: time.Second}, nil)
	if _, err := provider.TestConnection(context.Background(), platformp.TestConnectionRequest{}); err == nil {
		t.Fatal("expected missing credentials to fail closed")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
