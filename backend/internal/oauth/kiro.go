package oauth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Kiro AI authenticates through AWS SSO OIDC (Builder ID / IAM Identity Center
// device flows) or by importing a refresh token exported from the Kiro IDE.
// This mirrors 9router's KiroService. The dashboard "Connect Kiro" modal offers:
//
//   - Builder ID:          device-authorization against the public Builder ID
//                          portal (free AWS account).
//   - IAM Identity Center: device-authorization against a customer start URL +
//                          region (enterprise SSO).
//   - Import Token:        paste a refresh token from the Kiro IDE; validated by
//                          refreshing it against the Kiro desktop auth service.
//
// Builder ID / IDC tokens refresh through SSO OIDC using the registered client
// credentials (persisted in account metadata). Imported tokens refresh through
// the Kiro desktop social auth service (no client credentials needed).

const (
	kiroDefaultRegion     = "us-east-1"
	kiroBuilderIDStartURL = "https://view.awsapps.com/start"
	kiroClientName        = "kiro-oauth-client"
	kiroIssuerURL         = "https://identitycenter.amazonaws.com/ssoins-722374e8c3c8e6c6"
	// kiroSocialAuthBase backs imported-token refresh (Kiro IDE / social auth).
	kiroSocialAuthBase = "https://prod.us-east-1.auth.desktop.kiro.dev"
	// kiroImportTokenPrefix is the expected prefix of a Kiro IDE refresh token.
	kiroImportTokenPrefix = "aorAAAAAG"
)

// kiroScopes are the CodeWhisperer scopes Kiro requests at client registration.
var kiroScopes = []string{
	"codewhisperer:completions",
	"codewhisperer:analysis",
	"codewhisperer:conversations",
}

// kiroGrantTypes are the grants requested at client registration.
var kiroGrantTypes = []string{
	"urn:ietf:params:oauth:grant-type:device_code",
	"refresh_token",
}

// kiroOIDCBase returns the SSO OIDC base URL for a region.
func kiroOIDCBase(region string) string {
	if region == "" {
		region = kiroDefaultRegion
	}
	return fmt.Sprintf("https://oidc.%s.amazonaws.com", region)
}

// KiroClient is a registered SSO OIDC public client. Its credentials are needed
// to start device authorization and to refresh Builder ID / IDC tokens.
type KiroClient struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	Region       string `json:"region"`
	StartURL     string `json:"start_url"`
}

// KiroDeviceAuth is the device-authorization response shown to the user.
type KiroDeviceAuth struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

// kiroHTTP posts a JSON body to a Kiro/SSO OIDC endpoint and returns the raw
// body and status. These endpoints always speak JSON.
func kiroHTTP(ctx context.Context, url string, body any) ([]byte, int, error) {
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, 0, fmt.Errorf("kiro: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("kiro: request: %w", err)
	}
	defer resp.Body.Close()
	out, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("kiro: read response: %w", err)
	}
	return out, resp.StatusCode, nil
}

// KiroRegisterClient registers a public SSO OIDC client for the given region.
func KiroRegisterClient(ctx context.Context, region, startURL string) (*KiroClient, error) {
	if region == "" {
		region = kiroDefaultRegion
	}
	if startURL == "" {
		startURL = kiroBuilderIDStartURL
	}
	body := map[string]any{
		"clientName": kiroClientName,
		"clientType": "public",
		"scopes":     kiroScopes,
		"grantTypes": kiroGrantTypes,
		"issuerUrl":  kiroIssuerURL,
	}
	raw, status, err := kiroHTTP(ctx, kiroOIDCBase(region)+"/client/register", body)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("kiro: register client failed (%d): %s", status, truncate(raw, 300))
	}
	var parsed struct {
		ClientID     string `json:"clientId"`
		ClientSecret string `json:"clientSecret"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("kiro: parse register response: %w", err)
	}
	if parsed.ClientID == "" || parsed.ClientSecret == "" {
		return nil, fmt.Errorf("kiro: register response missing client credentials")
	}
	return &KiroClient{
		ClientID:     parsed.ClientID,
		ClientSecret: parsed.ClientSecret,
		Region:       region,
		StartURL:     startURL,
	}, nil
}

// StartDeviceAuth begins device authorization, returning the user code and
// verification URL to display.
func (c *KiroClient) StartDeviceAuth(ctx context.Context) (*KiroDeviceAuth, error) {
	body := map[string]any{
		"clientId":     c.ClientID,
		"clientSecret": c.ClientSecret,
		"startUrl":     c.StartURL,
	}
	raw, status, err := kiroHTTP(ctx, kiroOIDCBase(c.Region)+"/device_authorization", body)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("kiro: start device auth failed (%d): %s", status, truncate(raw, 300))
	}
	var parsed struct {
		DeviceCode              string `json:"deviceCode"`
		UserCode                string `json:"userCode"`
		VerificationURI         string `json:"verificationUri"`
		VerificationURIComplete string `json:"verificationUriComplete"`
		ExpiresIn               int    `json:"expiresIn"`
		Interval                int    `json:"interval"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("kiro: parse device auth response: %w", err)
	}
	if parsed.Interval <= 0 {
		parsed.Interval = 5
	}
	return &KiroDeviceAuth{
		DeviceCode:              parsed.DeviceCode,
		UserCode:                parsed.UserCode,
		VerificationURI:         parsed.VerificationURI,
		VerificationURIComplete: parsed.VerificationURIComplete,
		ExpiresIn:               parsed.ExpiresIn,
		Interval:                parsed.Interval,
	}, nil
}

// PollToken performs one poll of the SSO OIDC token endpoint for a device
// authorization. It reuses the generic PollResult shape.
func (c *KiroClient) PollToken(ctx context.Context, deviceCode string) PollResult {
	body := map[string]any{
		"clientId":     c.ClientID,
		"clientSecret": c.ClientSecret,
		"deviceCode":   deviceCode,
		"grantType":    "urn:ietf:params:oauth:grant-type:device_code",
	}
	raw, status, err := kiroHTTP(ctx, kiroOIDCBase(c.Region)+"/token", body)
	if err != nil {
		return PollResult{Err: err}
	}

	var parsed struct {
		AccessToken      string `json:"accessToken"`
		RefreshToken     string `json:"refreshToken"`
		ExpiresIn        int    `json:"expiresIn"`
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	_ = json.Unmarshal(raw, &parsed)

	if parsed.AccessToken != "" {
		return PollResult{Done: true, Tokens: &Tokens{
			AccessToken:  parsed.AccessToken,
			RefreshToken: parsed.RefreshToken,
			ExpiresIn:    parsed.ExpiresIn,
		}}
	}

	switch strings.ToLower(parsed.Error) {
	case "authorization_pending", "":
		return PollResult{Pending: true}
	case "slow_down":
		return PollResult{Pending: true, SlowDown: true}
	case "expired_token", "access_denied":
		return PollResult{Err: fmt.Errorf("kiro: %s: %s", parsed.Error, parsed.ErrorDescription)}
	default:
		if status == http.StatusOK {
			return PollResult{Pending: true}
		}
		return PollResult{Err: fmt.Errorf("kiro: device poll failed: %s %s", parsed.Error, parsed.ErrorDescription)}
	}
}

// Refresh exchanges a refresh token for a fresh access token via SSO OIDC,
// reusing the stored client credentials (Builder ID / IDC accounts).
func (c *KiroClient) Refresh(ctx context.Context, refreshToken string) (*Tokens, error) {
	if refreshToken == "" {
		return nil, fmt.Errorf("kiro: no refresh token")
	}
	body := map[string]any{
		"clientId":     c.ClientID,
		"clientSecret": c.ClientSecret,
		"refreshToken": refreshToken,
		"grantType":    "refresh_token",
	}
	raw, status, err := kiroHTTP(ctx, kiroOIDCBase(c.Region)+"/token", body)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("kiro: refresh failed (%d): %s", status, truncate(raw, 300))
	}
	return parseKiroRefresh(raw, refreshToken)
}

// KiroSocialRefresh refreshes an imported Kiro IDE token via the Kiro desktop
// social auth service. Imported tokens have no SSO client credentials.
func KiroSocialRefresh(ctx context.Context, refreshToken string) (*Tokens, error) {
	if refreshToken == "" {
		return nil, fmt.Errorf("kiro: no refresh token")
	}
	body := map[string]any{"refreshToken": refreshToken}
	raw, status, err := kiroHTTP(ctx, kiroSocialAuthBase+"/refreshToken", body)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("kiro: token refresh failed (%d): %s", status, truncate(raw, 300))
	}
	return parseKiroRefresh(raw, refreshToken)
}

// parseKiroRefresh normalizes a Kiro token response (camelCase) into Tokens.
func parseKiroRefresh(raw []byte, fallbackRefresh string) (*Tokens, error) {
	var parsed struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
		ProfileARN   string `json:"profileArn"`
		ExpiresIn    int    `json:"expiresIn"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("kiro: parse refresh response: %w", err)
	}
	if parsed.AccessToken == "" {
		return nil, fmt.Errorf("kiro: refresh response missing access token")
	}
	if parsed.RefreshToken == "" {
		parsed.RefreshToken = fallbackRefresh
	}
	if parsed.ExpiresIn <= 0 {
		parsed.ExpiresIn = 3600
	}
	extra := map[string]string{}
	if parsed.ProfileARN != "" {
		extra["profile_arn"] = parsed.ProfileARN
	}
	return &Tokens{
		AccessToken:  parsed.AccessToken,
		RefreshToken: parsed.RefreshToken,
		ExpiresIn:    parsed.ExpiresIn,
		Extra:        extra,
	}, nil
}

// KiroValidateImportToken validates and imports a refresh token from the Kiro
// IDE by refreshing it against the social auth service.
func KiroValidateImportToken(ctx context.Context, refreshToken string) (*Tokens, error) {
	refreshToken = strings.TrimSpace(refreshToken)
	if !strings.HasPrefix(refreshToken, kiroImportTokenPrefix) {
		return nil, fmt.Errorf("kiro: invalid token format; expected a token starting with %s…", kiroImportTokenPrefix)
	}
	tokens, err := KiroSocialRefresh(ctx, refreshToken)
	if err != nil {
		return nil, fmt.Errorf("kiro: token validation failed: %w", err)
	}
	return tokens, nil
}

// kiroFlowExpiry derives a session expiry from a device-auth response.
func kiroFlowExpiry(expiresIn int) time.Time {
	if expiresIn <= 0 {
		expiresIn = 600
	}
	return time.Now().Add(time.Duration(expiresIn) * time.Second)
}