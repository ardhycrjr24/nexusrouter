package prettylog

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// BannerConfig holds the values rendered into the startup banner.
type BannerConfig struct {
	Version  string
	Addr     string
	DBDriver string
	Cache    string // e.g. "disabled", "memory", "redis"
	DataDir  string
	LogLevel string
}

const logo = `
  ██   ██ ███████  ██ ██████   ██████  ██    ██ ████████ ███████ ██████
  ██  ██  ██      ██  ██   ██ ██    ██ ██    ██    ██    ██      ██   ██
  █████   █████   ██  ██████  ██    ██ ██    ██    ██    █████   ██████
  ██  ██  ██      ██  ██   ██ ██    ██ ██    ██    ██    ██      ██   ██
  ██   ██ ███████  ██ ██   ██  ██████   ██████     ██    ███████ ██   ██`

// PrintBanner writes the startup banner and config summary to w. It is a
// no-op when w is not a terminal, keeping production output clean.
func PrintBanner(w io.Writer, cfg BannerConfig) {
	if f, ok := w.(interface{ Fd() uintptr }); ok {
		if !IsTerminal(f.Fd()) {
			return
		}
	}

	rows := []struct {
		label, value string
	}{
		{"HTTP", "http://" + cfg.Addr},
		{"DB", cfg.DBDriver},
		{"Cache", cfg.Cache},
		{"Data", cfg.DataDir},
		{"Log", cfg.LogLevel},
	}

	versionLine := fmt.Sprintf("  NexusRouter %s", cfg.Version)

	// Box must be wide enough for the widest content row or the logo (~70 chars).
	const logoWidth = 70
	maxLen := logoWidth
	if l := len(versionLine); l > maxLen {
		maxLen = l
	}
	for _, r := range rows {
		l := len(r.label) + 4 + len(r.value)
		if l > maxLen {
			maxLen = l
		}
	}
	boxW := maxLen + 4

	hLine := strings.Repeat("─", boxW)
	top := ansiCyan + "  ╭" + hLine + "╮" + ansiReset
	bot := ansiCyan + "  ╰" + hLine + "╯" + ansiReset
	side := ansiCyan + "  │" + ansiReset

	fmt.Fprintln(w)
	for _, line := range strings.Split(logo, "\n") {
		if line != "" {
			fmt.Fprintf(w, "  %s%s%s\n", ansiCyan, line, ansiReset)
		}
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, top)
	fmt.Fprintf(w, "%s%s%s\n", side, padRow(versionLine, boxW), ansiReset)
	fmt.Fprintf(w, "%s%s%s\n", side, padRow("", boxW), ansiReset)
	for _, r := range rows {
		content := fmt.Sprintf("  %s%s%s  %s",
			ansiBold, r.label, ansiReset, r.value)
		rawLen := len(r.label) + 4 + len(r.value)
		gap := boxW - rawLen
		if gap < 0 {
			gap = 0
		}
		fmt.Fprintf(w, "%s%s%s%s\n",
			side, content, strings.Repeat(" ", gap), ansiReset)
	}
	fmt.Fprintln(w, bot)
	fmt.Fprintln(w)
}

func padRow(s string, width int) string {
	gap := width - len(s)
	if gap < 0 {
		gap = 0
	}
	return s + strings.Repeat(" ", gap)
}

// PrintBannerStdout is a convenience wrapper for the common case.
func PrintBannerStdout(cfg BannerConfig) {
	PrintBanner(os.Stdout, cfg)
}
