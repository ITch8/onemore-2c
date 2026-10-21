package temu

import (
	"crypto/md5"
	"encoding/hex"
	"strings"
	"testing"
)

func TestSignSortsKeysAndWrapsSecret(t *testing.T) {
	params := map[string]string{
		"type":      "bg.order.list.v2.get",
		"app_key":   "key",
		"timestamp": "1",
		"sign":      "ignored",
		"empty":     "",
	}
	got := Sign("secret", params)
	raw := "secret" + "app_key" + "key" + "timestamp" + "1" + "type" + "bg.order.list.v2.get" + "secret"
	sum := md5.Sum([]byte(raw))
	want := strings.ToUpper(hex.EncodeToString(sum[:]))
	if got != want {
		t.Fatalf("sign = %s, want %s", got, want)
	}
	if strings.Contains(got, "secret") || strings.Contains(got, "key") {
		t.Fatal("signature must not echo the secret or app key")
	}
}
