package oauth

import (
	"fmt"
	"net/url"
	"strings"
)

// ResolveRedirectURI returns the redirect URI that should be registered for a
// flow. Most providers use the dashboard callback supplied by the frontend;
// CLI-mirrored providers such as Codex/xAI require an exact fixed loopback URI.
func (c ProviderConfig) ResolveRedirectURI(requested string) string {
	path := c.CallbackPath
	if path == "" {
		path = "/callback"
	}

	if c.FixedLoopbackPort > 0 {
		host := c.LoopbackHost
		if host == "" {
			host = "127.0.0.1"
		}
		return fmt.Sprintf("http://%s:%d%s", host, c.FixedLoopbackPort, path)
	}

	if c.CallbackPath == "" || strings.TrimSpace(requested) == "" {
		return requested
	}
	u, err := url.Parse(requested)
	if err != nil {
		return requested
	}
	u.Path = path
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}
