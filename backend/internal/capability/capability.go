// Package capability maps models to the features they support, so the
// dispatcher never silently falls back to a model that cannot honor the
// request (e.g. routing a tool-calling request to a model without tools, or a
// vision request to a text-only model).
//
// The matrix is heuristic: it matches model ids by substring against a set of
// known families. Unknown models are assumed to support the baseline set
// (text + streaming) only, which is the safe conservative default.
package capability

import (
	"strings"

	"github.com/ardhycrjr24/nexusrouter/backend/internal/core"
)

// rule associates a model-id substring with the capabilities that family has.
// When exact is true, the match is compared for equality (not substring).
type rule struct {
	match string
	caps  []core.Capability
	exact bool
}

// rules are evaluated in order; all matching rules union their capabilities.
// Substrings are lowercased before matching.
var rules = []rule{
	// Frontier chat models: full feature set.
	{"gpt-4", []core.Capability{core.CapToolCalling, core.CapVision, core.CapStructuredOutput, core.CapLongContext}, false},
	{"gpt-5", []core.Capability{core.CapToolCalling, core.CapVision, core.CapReasoning, core.CapStructuredOutput, core.CapLongContext}, false},
	{"o1", []core.Capability{core.CapToolCalling, core.CapReasoning, core.CapStructuredOutput}, false},
	{"o3", []core.Capability{core.CapToolCalling, core.CapReasoning, core.CapStructuredOutput}, false},
	{"o4", []core.Capability{core.CapToolCalling, core.CapReasoning, core.CapStructuredOutput}, false},
	{"claude", []core.Capability{core.CapToolCalling, core.CapVision, core.CapReasoning, core.CapLongContext}, false},
	{"gemini", []core.Capability{core.CapToolCalling, core.CapVision, core.CapAudioInput, core.CapLongContext}, false},
	{"deepseek", []core.Capability{core.CapToolCalling, core.CapReasoning}, false},
	{"glm", []core.Capability{core.CapToolCalling, core.CapLongContext}, false},
	{"minimax", []core.Capability{core.CapToolCalling, core.CapLongContext}, false},
	{"qwen", []core.Capability{core.CapToolCalling}, false},
	{"kimi", []core.Capability{core.CapToolCalling, core.CapLongContext}, false},
	{"grok", []core.Capability{core.CapToolCalling, core.CapVision}, false},
	{"llama", []core.Capability{core.CapToolCalling}, false},
	{"mistral", []core.Capability{core.CapToolCalling}, false},
	{"mimo-v2-omni", []core.Capability{core.CapToolCalling, core.CapVision, core.CapLongContext}, false},
	{"mimo-v2.5", []core.Capability{core.CapToolCalling, core.CapVision, core.CapLongContext}, true},
	{"mimo-auto", []core.Capability{core.CapToolCalling, core.CapReasoning, core.CapVision, core.CapLongContext}, true},
	{"mimo", []core.Capability{core.CapToolCalling, core.CapLongContext}, false},
	{"mixtral", []core.Capability{core.CapToolCalling}, false},
	{"nemotron", []core.Capability{core.CapToolCalling}, false},
	{"phi-4", []core.Capability{core.CapToolCalling}, false},
	{"phi-3", []core.Capability{core.CapToolCalling}, false},
	{"codestral", []core.Capability{core.CapToolCalling}, false},

	// ByteDance / Volcengine models (doubao, ep-* endpoints).
	{"doubao", []core.Capability{core.CapToolCalling, core.CapLongContext}, false},
	{"bytedance", []core.Capability{core.CapToolCalling, core.CapLongContext}, false},
	{"ep-", []core.Capability{core.CapToolCalling, core.CapLongContext}, false},

	// Perplexity Sonar models.
	{"sonar", []core.Capability{core.CapToolCalling, core.CapLongContext}, false},

	// Cohere Command models.
	{"command", []core.Capability{core.CapToolCalling}, false},

	// Cerebras (serves llama-family, fast inference).
	{"cerebras", []core.Capability{core.CapToolCalling}, false},

	// Cloudflare Workers AI models.
	{"@cf/", []core.Capability{core.CapToolCalling}, false},

	// OpenAI Responses API models (codex, gpt-4o via responses).
	{"codex", []core.Capability{core.CapToolCalling, core.CapReasoning}, false},

	// Kiro / CodeWhisperer.
	{"kiro", []core.Capability{core.CapToolCalling}, false},
	{"codewhisperer", []core.Capability{core.CapToolCalling}, false},

	// Qoder (coding assistant platform). Model IDs are generic tier names,
	// so we use exact matches to avoid false positives with other providers.
	{"auto", []core.Capability{core.CapToolCalling, core.CapLongContext}, true},
	{"ultimate", []core.Capability{core.CapToolCalling, core.CapVision, core.CapLongContext}, true},
	{"performance", []core.Capability{core.CapToolCalling, core.CapLongContext}, true},
	{"efficient", []core.Capability{core.CapToolCalling, core.CapLongContext}, true},
	{"lite", []core.Capability{core.CapToolCalling}, true},
	{"qmodel", []core.Capability{core.CapToolCalling, core.CapLongContext}, false},
	{"dmodel", []core.Capability{core.CapToolCalling, core.CapReasoning}, false},
	{"dfmodel", []core.Capability{core.CapToolCalling, core.CapReasoning}, false},
	{"gm51model", []core.Capability{core.CapToolCalling, core.CapVision, core.CapLongContext}, true},
	{"kmodel", []core.Capability{core.CapToolCalling, core.CapLongContext}, true},
	{"mmodel", []core.Capability{core.CapToolCalling, core.CapLongContext}, true},
}

// baseline is granted to every model.
var baseline = []core.Capability{core.CapStreaming}

// Of returns the capability set for a model id.
func Of(model string) core.CapabilitySet {
	set := core.NewCapabilitySet(baseline...)
	lower := strings.ToLower(model)
	for _, r := range rules {
		matched := false
		if r.exact {
			matched = (lower == r.match)
		} else {
			matched = strings.Contains(lower, r.match)
		}
		if matched {
			for _, c := range r.caps {
				set.Add(c)
			}
		}
	}
	return set
}

// Supports reports whether a model satisfies all required capabilities.
func Supports(model string, required core.CapabilitySet) bool {
	return Of(model).Satisfies(required)
}

// Required infers the capabilities a request needs from its content, so the
// dispatcher can reject incapable fallback targets. It is conservative: it only
// flags capabilities that are unambiguously required by the request shape.
func Required(req *core.ChatRequest) core.CapabilitySet {
	set := core.NewCapabilitySet()
	if len(req.Tools) > 0 {
		set.Add(core.CapToolCalling)
	}
	if req.Stream {
		set.Add(core.CapStreaming)
	}
	if req.Reasoning != nil && (req.Reasoning.Effort != "" || req.Reasoning.MaxTokens > 0) {
		set.Add(core.CapReasoning)
	}
	if len(req.ResponseFormat) > 0 {
		set.Add(core.CapStructuredOutput)
	}
	for _, m := range req.Messages {
		for _, p := range m.Content {
			switch p.Type {
			case core.PartImage:
				set.Add(core.CapVision)
			case core.PartAudio:
				set.Add(core.CapAudioInput)
			}
		}
	}
	return set
}
