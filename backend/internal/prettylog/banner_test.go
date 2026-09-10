package prettylog

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func TestPrintBanner(t *testing.T) {
	var buf bytes.Buffer
	PrintBanner(&buf, BannerConfig{
		Version:  "v0.1.18",
		Addr:     "127.0.0.1:20180",
		DBDriver: "sqlite",
		Cache:    "disabled",
		DataDir:  "~/.nexusrouter",
		LogLevel: "info (pretty)",
	})
	out := buf.String()
	if !strings.Contains(out, "NexusRouter v0.1.18") {
		t.Errorf("expected version line in banner, got: %s", out)
	}
	fmt.Print(out)
}
