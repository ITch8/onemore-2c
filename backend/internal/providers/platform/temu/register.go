package temu

import platformp "github.com/trademind-ai/trademind/backend/internal/providers/platform"

// RegisterProvider replaces the planned Temu registry entry.
func RegisterProvider() {
	platformp.Register(NewProvider(LoadConfig(), nil))
}
