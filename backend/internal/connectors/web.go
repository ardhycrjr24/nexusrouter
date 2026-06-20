package connectors

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"

	"github.com/ardhycrjr24/nexusrouter/backend/internal/core"
)

// WebConnector drives web-search and web-fetch providers. Each provider has its
// own wire format and auth scheme, so the connector dispatches on the provider
// id to build the right request and parse the right response shape.
//
// Supported search providers: tavily, exa, serper, brave-search, searxng.
// Supported fetch providers:  tavily, exa, firecrawl, jina-reader.
type WebConnector struct {
	id          string
	defaultBase string
}

// NewWebConnector builds a web search/fetch connector for a provider.
func NewWebConnector(id, defaultBaseURL string) *WebConnector {
	return &WebConnector{id: id, defaultBase: defaultBaseURL}
}

func (c *WebConnector) ID() string            { return c.id }
func (c *WebConnector) Dialect() core.Dialect { return core.DialectOpenAI }

// Chat is unsupported on web connectors; they serve search/fetch only.
func (c *WebConnector) Chat(ctx context.Context, req *core.ChatRequest, creds core.Credentials) (*core.ChatResponse, error) {
	return nil, &core.ProviderError{Kind: core.ErrBadRequest, Provider: c.id, Message: "provider does not support chat"}
}

// Stream is unsupported on web connectors.
func (c *WebConnector) Stream(ctx context.Context, req *core.ChatRequest, creds core.Credentials, cfg core.StreamConfig) (<-chan core.StreamChunk, error) {
	return nil, &core.ProviderError{Kind: core.ErrBadRequest, Provider: c.id, Message: "provider does not support streaming"}
}

func (c *WebConnector) baseURL(creds core.Credentials) string {
	if creds.BaseURL != "" {
		return creds.BaseURL
	}
	return c.defaultBase
}

// Validate probes a lightweight web-search/fetch endpoint. As in 9router, any
// reached non-auth response means the credential and provider configuration are
// accepted; 401/403 still fail.
func (c *WebConnector) Validate(ctx context.Context, creds core.Credentials) error {
	var err error
	switch c.id {
	case "tavily":
		body, _ := json.Marshal(map[string]any{"query": "ping", "max_results": 1, "search_depth": "basic"})
		_, err = doJSON(ctx, c.id, "validate", joinURL(c.baseURL(creds), "search"), body, map[string]string{"Authorization": bearer(creds.APIKey)})
	case "exa":
		body, _ := json.Marshal(map[string]any{"query": "ping", "numResults": 1})
		_, err = doJSON(ctx, c.id, "validate", joinURL(c.baseURL(creds), "search"), body, map[string]string{"x-api-key": creds.APIKey})
	case "serper":
		body, _ := json.Marshal(map[string]any{"q": "ping", "num": 1})
		_, err = doJSON(ctx, c.id, "validate", joinURL(c.baseURL(creds), "search"), body, map[string]string{"X-API-KEY": creds.APIKey})
	case "brave-search":
		q := url.Values{}
		q.Set("q", "ping")
		q.Set("count", "1")
		_, err = doJSONMethod(ctx, "GET", c.id, "validate", joinURL(c.baseURL(creds), "web/search")+"?"+q.Encode(), nil, map[string]string{
			"X-Subscription-Token": creds.APIKey,
			"Accept":               "application/json",
		})
	case "firecrawl":
		body, _ := json.Marshal(map[string]any{"url": "https://example.com", "formats": []string{"markdown"}})
		_, err = doJSON(ctx, c.id, "validate", joinURL(c.baseURL(creds), "scrape"), body, map[string]string{"Authorization": bearer(creds.APIKey)})
	case "jina-reader":
		headers := map[string]string{"Accept": "text/plain"}
		if creds.APIKey != "" {
			headers["Authorization"] = bearer(creds.APIKey)
		}
		_, err = doJSONMethod(ctx, "GET", c.id, "validate", strings.TrimRight(c.baseURL(creds), "/")+"/https://example.com", nil, headers)
	default:
		return nil
	}
	if err == nil {
		return nil
	}
	if validationAuthError(err) || !validationReachedUpstream(err) {
		return err
	}
	return nil
}

// ---- Web search -------------------------------------------------------------

// Search runs a web search against the configured provider.
func (c *WebConnector) Search(ctx context.Context, req *core.SearchRequest, creds core.Credentials) (*core.SearchResponse, error) {
	maxResults := req.MaxResults
	if maxResults <= 0 {
		maxResults = 5
	}
	switch c.id {
	case "tavily":
		return c.searchTavily(ctx, req, creds, maxResults)
	case "exa":
		return c.searchExa(ctx, req, creds, maxResults)
	case "serper":
		return c.searchSerper(ctx, req, creds, maxResults)
	case "brave-search":
		return c.searchBrave(ctx, req, creds, maxResults)
	case "searxng":
		return c.searchSearxng(ctx, req, creds, maxResults)
	default:
		return nil, &core.ProviderError{Kind: core.ErrBadRequest, Provider: c.id, Message: "web search not implemented for provider " + c.id}
	}
}

func (c *WebConnector) searchTavily(ctx context.Context, req *core.SearchRequest, creds core.Credentials, maxResults int) (*core.SearchResponse, error) {
	body, _ := json.Marshal(map[string]any{
		"query": req.Query, "max_results": maxResults, "search_depth": "basic",
	})
	headers := map[string]string{"Authorization": bearer(creds.APIKey)}
	raw, err := doJSON(ctx, c.id, "", joinURL(c.baseURL(creds), "search"), body, headers)
	if err != nil {
		return nil, err
	}
	var parsed struct {
		Results []struct {
			Title   string  `json:"title"`
			URL     string  `json:"url"`
			Content string  `json:"content"`
			Score   float64 `json:"score"`
		} `json:"results"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, parseErr(c.id, err)
	}
	out := &core.SearchResponse{Query: req.Query}
	for _, r := range parsed.Results {
		out.Results = append(out.Results, core.SearchResult{Title: r.Title, URL: r.URL, Snippet: r.Content, Score: r.Score})
	}
	return out, nil
}

func (c *WebConnector) searchExa(ctx context.Context, req *core.SearchRequest, creds core.Credentials, maxResults int) (*core.SearchResponse, error) {
	body, _ := json.Marshal(map[string]any{
		"query": req.Query, "numResults": maxResults, "contents": map[string]any{"text": true},
	})
	headers := map[string]string{"x-api-key": creds.APIKey}
	raw, err := doJSON(ctx, c.id, "", joinURL(c.baseURL(creds), "search"), body, headers)
	if err != nil {
		return nil, err
	}
	var parsed struct {
		Results []struct {
			Title string  `json:"title"`
			URL   string  `json:"url"`
			Text  string  `json:"text"`
			Score float64 `json:"score"`
		} `json:"results"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, parseErr(c.id, err)
	}
	out := &core.SearchResponse{Query: req.Query}
	for _, r := range parsed.Results {
		out.Results = append(out.Results, core.SearchResult{Title: r.Title, URL: r.URL, Snippet: truncate(r.Text, 500), Score: r.Score})
	}
	return out, nil
}

func (c *WebConnector) searchSerper(ctx context.Context, req *core.SearchRequest, creds core.Credentials, maxResults int) (*core.SearchResponse, error) {
	body, _ := json.Marshal(map[string]any{"q": req.Query, "num": maxResults})
	headers := map[string]string{"X-API-KEY": creds.APIKey}
	raw, err := doJSON(ctx, c.id, "", joinURL(c.baseURL(creds), "search"), body, headers)
	if err != nil {
		return nil, err
	}
	var parsed struct {
		Organic []struct {
			Title   string `json:"title"`
			Link    string `json:"link"`
			Snippet string `json:"snippet"`
		} `json:"organic"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, parseErr(c.id, err)
	}
	out := &core.SearchResponse{Query: req.Query}
	for _, r := range parsed.Organic {
		out.Results = append(out.Results, core.SearchResult{Title: r.Title, URL: r.Link, Snippet: r.Snippet})
	}
	return out, nil
}

func (c *WebConnector) searchBrave(ctx context.Context, req *core.SearchRequest, creds core.Credentials, maxResults int) (*core.SearchResponse, error) {
	q := url.Values{}
	q.Set("q", req.Query)
	q.Set("count", strconv.Itoa(maxResults))
	endpoint := joinURL(c.baseURL(creds), "web/search") + "?" + q.Encode()
	headers := map[string]string{"X-Subscription-Token": creds.APIKey, "Accept": "application/json"}
	raw, err := doJSONMethod(ctx, "GET", c.id, "", endpoint, nil, headers)
	if err != nil {
		return nil, err
	}
	var parsed struct {
		Web struct {
			Results []struct {
				Title       string `json:"title"`
				URL         string `json:"url"`
				Description string `json:"description"`
			} `json:"results"`
		} `json:"web"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, parseErr(c.id, err)
	}
	out := &core.SearchResponse{Query: req.Query}
	for _, r := range parsed.Web.Results {
		out.Results = append(out.Results, core.SearchResult{Title: r.Title, URL: r.URL, Snippet: r.Description})
	}
	return out, nil
}

func (c *WebConnector) searchSearxng(ctx context.Context, req *core.SearchRequest, creds core.Credentials, maxResults int) (*core.SearchResponse, error) {
	q := url.Values{}
	q.Set("q", req.Query)
	q.Set("format", "json")
	endpoint := joinURL(c.baseURL(creds), "search") + "?" + q.Encode()
	raw, err := doJSONMethod(ctx, "GET", c.id, "", endpoint, nil, nil)
	if err != nil {
		return nil, err
	}
	var parsed struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Content string `json:"content"`
		} `json:"results"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, parseErr(c.id, err)
	}
	out := &core.SearchResponse{Query: req.Query}
	for i, r := range parsed.Results {
		if i >= maxResults {
			break
		}
		out.Results = append(out.Results, core.SearchResult{Title: r.Title, URL: r.URL, Snippet: r.Content})
	}
	return out, nil
}

// ---- Web fetch --------------------------------------------------------------

// Fetch retrieves and extracts the content of a URL.
func (c *WebConnector) Fetch(ctx context.Context, req *core.FetchRequest, creds core.Credentials) (*core.FetchResponse, error) {
	switch c.id {
	case "tavily":
		return c.fetchTavily(ctx, req, creds)
	case "exa":
		return c.fetchExa(ctx, req, creds)
	case "firecrawl":
		return c.fetchFirecrawl(ctx, req, creds)
	case "jina-reader":
		return c.fetchJina(ctx, req, creds)
	default:
		return nil, &core.ProviderError{Kind: core.ErrBadRequest, Provider: c.id, Message: "web fetch not implemented for provider " + c.id}
	}
}

func (c *WebConnector) fetchTavily(ctx context.Context, req *core.FetchRequest, creds core.Credentials) (*core.FetchResponse, error) {
	body, _ := json.Marshal(map[string]any{"urls": []string{req.URL}})
	headers := map[string]string{"Authorization": bearer(creds.APIKey)}
	raw, err := doJSON(ctx, c.id, "", joinURL(c.baseURL(creds), "extract"), body, headers)
	if err != nil {
		return nil, err
	}
	var parsed struct {
		Results []struct {
			URL        string `json:"url"`
			RawContent string `json:"raw_content"`
		} `json:"results"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, parseErr(c.id, err)
	}
	content := ""
	if len(parsed.Results) > 0 {
		content = parsed.Results[0].RawContent
	}
	return &core.FetchResponse{URL: req.URL, Content: clampChars(content, req.MaxCharacters), Format: "text"}, nil
}

func (c *WebConnector) fetchExa(ctx context.Context, req *core.FetchRequest, creds core.Credentials) (*core.FetchResponse, error) {
	body, _ := json.Marshal(map[string]any{"ids": []string{req.URL}, "text": true})
	headers := map[string]string{"x-api-key": creds.APIKey}
	raw, err := doJSON(ctx, c.id, "", joinURL(c.baseURL(creds), "contents"), body, headers)
	if err != nil {
		return nil, err
	}
	var parsed struct {
		Results []struct {
			URL   string `json:"url"`
			Title string `json:"title"`
			Text  string `json:"text"`
		} `json:"results"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, parseErr(c.id, err)
	}
	out := &core.FetchResponse{URL: req.URL, Format: "text"}
	if len(parsed.Results) > 0 {
		out.Title = parsed.Results[0].Title
		out.Content = clampChars(parsed.Results[0].Text, req.MaxCharacters)
	}
	return out, nil
}

func (c *WebConnector) fetchFirecrawl(ctx context.Context, req *core.FetchRequest, creds core.Credentials) (*core.FetchResponse, error) {
	format := req.Format
	if format == "" {
		format = "markdown"
	}
	body, _ := json.Marshal(map[string]any{"url": req.URL, "formats": []string{format}})
	headers := map[string]string{"Authorization": bearer(creds.APIKey)}
	raw, err := doJSON(ctx, c.id, "", joinURL(c.baseURL(creds), "scrape"), body, headers)
	if err != nil {
		return nil, err
	}
	var parsed struct {
		Data struct {
			Markdown string `json:"markdown"`
			HTML     string `json:"html"`
			Text     string `json:"text"`
			Metadata struct {
				Title string `json:"title"`
			} `json:"metadata"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, parseErr(c.id, err)
	}
	content := parsed.Data.Markdown
	if content == "" {
		content = parsed.Data.Text
	}
	if content == "" {
		content = parsed.Data.HTML
	}
	return &core.FetchResponse{URL: req.URL, Title: parsed.Data.Metadata.Title, Content: clampChars(content, req.MaxCharacters), Format: format}, nil
}

func (c *WebConnector) fetchJina(ctx context.Context, req *core.FetchRequest, creds core.Credentials) (*core.FetchResponse, error) {
	// Jina Reader prepends the target URL to its base: https://r.jina.ai/<url>.
	endpoint := strings.TrimRight(c.baseURL(creds), "/") + "/" + req.URL
	headers := map[string]string{"Accept": "text/plain"}
	if creds.APIKey != "" {
		headers["Authorization"] = bearer(creds.APIKey)
	}
	raw, err := doJSONMethod(ctx, "GET", c.id, "", endpoint, nil, headers)
	if err != nil {
		return nil, err
	}
	return &core.FetchResponse{URL: req.URL, Content: clampChars(string(raw), req.MaxCharacters), Format: "markdown"}, nil
}

// ---- helpers ----------------------------------------------------------------

func parseErr(provider string, err error) error {
	return &core.ProviderError{Kind: core.ErrUpstream, Provider: provider, Message: "parse response: " + err.Error(), Cause: err}
}

func truncate(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

func clampChars(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max]
}
