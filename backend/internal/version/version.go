// Package version exposes the NexusRouter build version. It supports two sources,
// in priority order:
//
//  1. A value injected at build time via -ldflags "-X main.Version=...". This is
//     what release builds and `make build` use (derived from the git tag).
//  2. A committed VERSION file embedded into the binary. This is the fallback
//     for build environments that cannot inject ldflags or run git — notably
//     Docker/PaaS builds (Coolify, etc.) where the .git dir is absent. Because
//     the file is committed source, it always travels with the binary.
//
// The committed file means a deployed build reports a real version (e.g.
// "v0.1.6") instead of "dev", even when nothing stamps it at build time. Bump
// the VERSION file as part of cutting a release.
package version

import (
	_ "embed"
	"strings"
)

//go:embed VERSION
var embedded string

// Embedded returns the committed version string from the VERSION file, trimmed.
// Returns "" if the file is empty.
func Embedded() string {
	return strings.TrimSpace(embedded)
}

// Resolve picks the best available version. The ldflags-injected value wins when
// it is a real version; otherwise it falls back to the committed VERSION file,
// then to "dev".
func Resolve(injected string) string {
	injected = strings.TrimSpace(injected)
	if injected != "" && injected != "dev" {
		return injected
	}
	if e := Embedded(); e != "" {
		return e
	}
	if injected != "" {
		return injected
	}
	return "dev"
}
