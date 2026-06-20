package connectors

import (
	"testing"

	"github.com/ardhycrjr24/nexusrouter/backend/internal/core"
)

func TestCopilot_SupportsResponsesEndpoint(t *testing.T) {
	c := NewGitHubCopilot("github_copilot", "https://api.githubcopilot.com")
	cases := []struct {
		model string
		want  bool
	}{
		{"gpt-5", true},
		{"gpt-5-codex", true},
		{"o3", true},
		{"gemini-2.5-pro", false},
		{"claude-3.7-sonnet", false},
		{"GEMINI-PRO", false},   // case-insensitive
		{"Claude-Opus", false},  // case-insensitive
	}
	for _, tc := range cases {
		if got := c.supportsResponsesEndpoint(tc.model); got != tc.want {
			t.Errorf("supportsResponsesEndpoint(%q) = %v, want %v", tc.model, got, tc.want)
		}
	}
}

func TestCopilot_ResponsesCache(t *testing.T) {
	c := NewGitHubCopilot("github_copilot", "https://api.githubcopilot.com")
	if c.knowsResponses("gpt-5-codex") {
		t.Fatal("model should not be cached initially")
	}
	c.markResponses("gpt-5-codex")
	if !c.knowsResponses("gpt-5-codex") {
		t.Error("model should be cached after markResponses")
	}
	if c.knowsResponses("gpt-4o") {
		t.Error("unrelated model should not be cached")
	}
}

func TestIsResponsesEscalation(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "400 not accessible via chat/completions",
			err:  &core.ProviderError{StatusCode: 400, Message: "The model is not accessible via the /chat/completions endpoint"},
			want: true,
		},
		{
			name: "400 model not supported",
			err:  &core.ProviderError{StatusCode: 400, Message: "The requested model is not supported"},
			want: true,
		},
		{
			name: "400 unrelated error",
			err:  &core.ProviderError{StatusCode: 400, Message: "invalid temperature value"},
			want: false,
		},
		{
			name: "500 with escalation-like text (wrong status)",
			err:  &core.ProviderError{StatusCode: 500, Message: "the requested model is not supported"},
			want: false,
		},
		{
			name: "401 auth error",
			err:  &core.ProviderError{StatusCode: 401, Message: "bad token"},
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isResponsesEscalation(tc.err); got != tc.want {
				t.Errorf("isResponsesEscalation = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCopilot_ResponsesURL(t *testing.T) {
	c := NewGitHubCopilot("github_copilot", "https://api.githubcopilot.com")
	got := c.responsesURL(core.Credentials{})
	want := "https://api.githubcopilot.com/responses"
	if got != want {
		t.Errorf("responsesURL = %q, want %q", got, want)
	}

	// When base already includes /chat/completions, it is swapped for /responses.
	got = c.responsesURL(core.Credentials{BaseURL: "https://api.githubcopilot.com/chat/completions"})
	if got != want {
		t.Errorf("responsesURL with chat suffix = %q, want %q", got, want)
	}
}