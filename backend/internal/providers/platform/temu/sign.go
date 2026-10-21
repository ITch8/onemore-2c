package temu

import (
	"crypto/md5"
	"encoding/hex"
	"sort"
	"strings"
)

// Sign builds the Temu Partner router signature:
// uppercase MD5(app_secret + sorted key/value pairs + app_secret).
// The sign field itself is excluded. Empty values are excluded.
func Sign(secret string, params map[string]string) string {
	keys := make([]string, 0, len(params))
	for key, value := range params {
		if key == "sign" || strings.TrimSpace(key) == "" || value == "" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var builder strings.Builder
	builder.WriteString(secret)
	for _, key := range keys {
		builder.WriteString(key)
		builder.WriteString(params[key])
	}
	builder.WriteString(secret)
	sum := md5.Sum([]byte(builder.String()))
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}
