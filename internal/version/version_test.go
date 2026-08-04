package version

import (
	"strings"
	"testing"
)

func TestUserAgent(t *testing.T) {
	ua := UserAgent()
	if !strings.HasPrefix(ua, "NEOVIOLET v") {
		t.Fatalf("UserAgent() = %q, want NEOVIOLET v prefix", ua)
	}
	if !strings.Contains(ua, "https://github.com/AuroraStudio-aurorast/NeoViolet") {
		t.Fatalf("UserAgent() = %q, want repository URL", ua)
	}
}
