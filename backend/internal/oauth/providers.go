package oauth

// ProviderConfig holds the OAuth endpoints and client identity for one
// provider. It is a curated subset of 9router's OAuth constants, covering the
// providers NexusRouter can drive with the standard authorization-code and
// device-code grants.
type ProviderConfig struct {
	// Provider is the catalog provider id this config authenticates.
	Provider string
	Flow     FlowType

	ClientID     string
	ClientSecret string // empty for public PKCE clients

	AuthorizeURL  string // authorization_code(_pkce)
	DeviceCodeURL string // device_code
	TokenURL      string
	RefreshURL    string // defaults to TokenURL when empty

	// Scopes is space-joined into the request.
	Scopes []string
	// PKCEVerifierBytes overrides the default 32-byte verifier entropy.
	PKCEVerifierBytes int
	// UsesBasicAuth sends client_id:client_secret as HTTP Basic on token calls.
	UsesBasicAuth bool
	// ExtraAuthParams are appended to the authorize URL (provider quirks).
	ExtraAuthParams map[string]string
	// ExtraAuthParamOrder preserves provider-specific parameter order when the
	// authorize URL needs CLI-compatible percent encoding.
	ExtraAuthParamOrder []string
	// EncodeAuthSpacesAsPercent mirrors CLIs that build authorize URLs with
	// encodeURIComponent, where spaces become %20 rather than +.
	EncodeAuthSpacesAsPercent bool
	// SkipStandardAuthParams omits response_type, client_id, scope from the
	// authorize URL (provider quirks, e.g. Cline only wants client_type +
	// callback_url + redirect_uri).
	SkipStandardAuthParams bool
	// NonceBytes adds a random hex nonce parameter to the authorize URL.
	NonceBytes int
	// TokenContentType is "form" (x-www-form-urlencoded, default) or "json".
	TokenContentType string
	// ExtraTokenParams are added to the token exchange request body
	// (provider quirks, e.g. Cline requires client_type=extension).
	ExtraTokenParams map[string]string
	// DeviceCodePKCE enables PKCE for device-code flows (e.g. Qwen requires
	// code_challenge + code_challenge_method on the device-code request).
	DeviceCodePKCE bool
	// UserAgent overrides the default Go User-Agent header on HTTP requests.
	// Some providers (Qwen) sit behind a WAF that blocks Go-http-client/1.1.
	UserAgent string
	// ClientDeviceCode tells the gateway to let the browser (frontend) make
	// the device-code HTTP request instead of the Go backend.  This is
	// required when the provider's WAF uses TLS fingerprinting to block
	// non-browser clients (e.g. Qwen / Alibaba Cloud WAF).
	ClientDeviceCode bool

	// CallbackPath and FixedLoopbackPort mirror CLI OAuth loopback callbacks.
	// Providers with FixedLoopbackPort set ignore the dashboard-provided
	// redirect host and use http://LoopbackHost:FixedLoopbackPort/CallbackPath.
	CallbackPath      string
	FixedLoopbackPort int
	LoopbackHost      string

	// UserInfoURL is the endpoint called after token exchange to retrieve the
	// connected user's email and display name.  When empty, no profile fetch
	// is attempted and the account label falls back to the provider name.
	UserInfoURL string
}

// refreshURL returns the configured refresh URL, defaulting to TokenURL.
func (c ProviderConfig) refreshURL() string {
	if c.RefreshURL != "" {
		return c.RefreshURL
	}
	return c.TokenURL
}

// configs maps provider id -> OAuth config. Client ids/secrets are the public
// values published by the upstream CLIs (same as 9router); they are not
// secrets in the confidential sense.
var configs = map[string]ProviderConfig{
	"claude": {
		Provider:     "claude",
		Flow:         FlowAuthCodePKCE,
		ClientID:     "9d1c250a-e61b-44d9-88ed-5944d1962f5e",
		AuthorizeURL: "https://claude.ai/oauth/authorize",
		TokenURL:     "https://api.anthropic.com/v1/oauth/token",
		Scopes:       []string{"org:create_api_key", "user:profile", "user:inference"},
		// Claude's token endpoint expects a JSON body.
		TokenContentType: "json",
		ExtraAuthParams:  map[string]string{"code": "true"},
		UserInfoURL:      "https://api.anthropic.com/v1/me",
	},
	"codex": {
		Provider:                  "codex",
		Flow:                      FlowAuthCodePKCE,
		ClientID:                  "app_EMoamEEZ73f0CkXaXp7hrann",
		AuthorizeURL:              "https://auth.openai.com/oauth/authorize",
		TokenURL:                  "https://auth.openai.com/oauth/token",
		Scopes:                    []string{"openid", "profile", "email", "offline_access"},
		ExtraAuthParams:           map[string]string{"id_token_add_organizations": "true", "codex_cli_simplified_flow": "true", "originator": "codex_cli_rs"},
		ExtraAuthParamOrder:       []string{"id_token_add_organizations", "codex_cli_simplified_flow", "originator"},
		EncodeAuthSpacesAsPercent: true,
		CallbackPath:              "/auth/callback",
		FixedLoopbackPort:         1455,
		LoopbackHost:              "localhost",
		UserInfoURL:               "https://auth.openai.com/oauth/userinfo",
	},
	"gemini-cli": {
		Provider:        "gemini-cli",
		Flow:            FlowAuthCode,
		ClientID:        "DUMMY_CLIENT_ID",
		ClientSecret:    "DUMMY_SECRET",
		AuthorizeURL:    "https://accounts.google.com/o/oauth2/v2/auth",
		TokenURL:        "https://oauth2.googleapis.com/token",
		Scopes:          []string{"https://www.googleapis.com/auth/cloud-platform", "https://www.googleapis.com/auth/userinfo.email", "https://www.googleapis.com/auth/userinfo.profile"},
		ExtraAuthParams: map[string]string{"access_type": "offline", "prompt": "consent"},
		UserInfoURL:     "https://www.googleapis.com/oauth2/v1/userinfo",
	},
	"antigravity": {
		Provider:        "antigravity",
		Flow:            FlowAuthCode,
		ClientID:        "DUMMY_CLIENT_ID",
		ClientSecret:    "DUMMY_SECRET",
		AuthorizeURL:    "https://accounts.google.com/o/oauth2/v2/auth",
		TokenURL:        "https://oauth2.googleapis.com/token",
		Scopes:          []string{"https://www.googleapis.com/auth/cloud-platform", "https://www.googleapis.com/auth/userinfo.email", "https://www.googleapis.com/auth/userinfo.profile", "https://www.googleapis.com/auth/cclog", "https://www.googleapis.com/auth/experimentsandconfigs"},
		ExtraAuthParams: map[string]string{"access_type": "offline", "prompt": "consent"},
		UserInfoURL:     "https://www.googleapis.com/oauth2/v1/userinfo",
	},
	"xai": {
		Provider:                  "xai",
		Flow:                      FlowAuthCodePKCE,
		ClientID:                  "b1a00492-073a-47ea-816f-4c329264a828",
		AuthorizeURL:              "https://auth.x.ai/oauth2/authorize",
		TokenURL:                  "https://auth.x.ai/oauth2/token",
		Scopes:                    []string{"openid", "profile", "email", "offline_access", "grok-cli:access", "api:access"},
		PKCEVerifierBytes:         96,
		ExtraAuthParams:           map[string]string{"plan": "generic", "referrer": "cli-proxy-api"},
		ExtraAuthParamOrder:       []string{"plan", "referrer"},
		EncodeAuthSpacesAsPercent: true,
		NonceBytes:                16,
		CallbackPath:              "/callback",
		FixedLoopbackPort:         56121,
		LoopbackHost:              "127.0.0.1",
		UserInfoURL:               "https://auth.x.ai/oauth2/userinfo",
	},
	"github": {
		Provider:      "github",
		Flow:          FlowDeviceCode,
		ClientID:      "Iv1.b507a08c87ecfe98",
		DeviceCodeURL: "https://github.com/login/device/code",
		TokenURL:      "https://github.com/login/oauth/access_token",
		Scopes:        []string{"read:user"},
		UserInfoURL:   "https://api.github.com/user",
	},
	"qwen": {
		Provider:         "qwen",
		Flow:             FlowDeviceCode,
		ClientID:         "f0304373b74a44d2b584a3fb70ca9e56",
		DeviceCodeURL:    "https://chat.qwen.ai/api/v1/oauth2/device/code",
		TokenURL:         "https://chat.qwen.ai/api/v1/oauth2/token",
		Scopes:           []string{"openid", "profile", "email", "model.completion"},
		UserInfoURL:      "https://chat.qwen.ai/api/v1/oauth2/userinfo",
		DeviceCodePKCE:   true,
		UserAgent:        "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/137.0.0.0 Safari/537.36",
		ClientDeviceCode: true,
	},
	"cline": {
		Provider:               "cline",
		Flow:                   FlowAuthCode,
		AuthorizeURL:           "https://api.cline.bot/api/v1/auth/authorize",
		TokenURL:               "https://api.cline.bot/api/v1/auth/token",
		RefreshURL:             "https://api.cline.bot/api/v1/auth/refresh",
		UserInfoURL:            "https://api.cline.bot/api/v1/auth/userinfo",
		TokenContentType:       "json",
		SkipStandardAuthParams: true,
		ExtraAuthParams: map[string]string{
			"client_type": "extension",
		},
		ExtraTokenParams: map[string]string{
			"client_type": "extension",
		},
	},
	"iflow": {
		Provider:     "iflow",
		Flow:         FlowAuthCode,
		ClientID:     "10009311001",
		ClientSecret: "4Z3YjXycVsQvyGF1etiNlIBB4RsqSDtW",
		AuthorizeURL: "https://iflow.cn/oauth",
		TokenURL:     "https://iflow.cn/oauth/token",
		UserInfoURL:  "https://iflow.cn/api/oauth/getUserInfo",
		ExtraAuthParams: map[string]string{
			"loginMethod": "phone",
			"type":        "phone",
		},
	},
	"kimi-coding": {
		Provider:      "kimi-coding",
		Flow:          FlowDeviceCode,
		ClientID:      "17e5f671-d194-4dfb-9706-5516cb48c098",
		DeviceCodeURL: "https://auth.kimi.com/api/oauth/device_authorization",
		TokenURL:      "https://auth.kimi.com/api/oauth/token",
	},
}

// ConfigFor returns the OAuth config for a provider id.
func ConfigFor(provider string) (ProviderConfig, bool) {
	c, ok := configs[provider]
	return c, ok
}

// SupportedProviders lists provider ids with an OAuth config, for discovery.
func SupportedProviders() []string {
	out := make([]string, 0, len(configs))
	for id := range configs {
		out = append(out, id)
	}
	return out
}
