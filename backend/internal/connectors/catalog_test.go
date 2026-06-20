package connectors

import (
	"testing"

	"github.com/ardhycrjr24/nexusrouter/backend/internal/core"
)

func TestCatalogHasCoreProviders(t *testing.T) {
	want := []string{"openai", "anthropic", "deepseek", "groq", "gemini", "openrouter", "tavily", "elevenlabs"}
	for _, id := range want {
		if _, ok := SpecByID(id); !ok {
			t.Errorf("catalog missing provider %q", id)
		}
	}
}

func TestSpecByAlias(t *testing.T) {
	spec, ok := SpecByAlias("ds")
	if !ok || spec.ID != "deepseek" {
		t.Fatalf("alias ds should resolve to deepseek, got %q ok=%v", spec.ID, ok)
	}
	// id also resolves.
	if spec, ok := SpecByAlias("openai"); !ok || spec.ID != "openai" {
		t.Fatalf("id openai should resolve, got %q ok=%v", spec.ID, ok)
	}
}

func TestSpecsByKind(t *testing.T) {
	cases := map[core.ServiceKind][]string{
		core.ServiceImage:  {"nanobanana", "fal-ai", "stability-ai"},
		core.ServiceSTT:    {"deepgram", "assemblyai", "groq"},
		core.ServiceTTS:    {"elevenlabs", "inworld"},
		core.ServiceSearch: {"tavily", "brave-search", "serper", "exa"},
		core.ServiceFetch:  {"firecrawl", "exa", "tavily"},
	}
	for kind, wantProviders := range cases {
		specs := SpecsByKind(kind)
		got := map[string]bool{}
		for _, s := range specs {
			got[s.ID] = true
		}
		for _, id := range wantProviders {
			if !got[id] {
				t.Errorf("kind %q: expected provider %q in results", kind, id)
			}
		}
	}
}

func TestModelsByKind(t *testing.T) {
	// Every LLM model should belong to a provider that advertises the LLM kind.
	llms := ModelsByKind(core.ServiceLLM)
	if len(llms) == 0 {
		t.Fatal("expected at least one LLM model in catalog")
	}
	for _, pm := range llms {
		if pm.Model.Kind != core.ServiceLLM {
			t.Errorf("ModelsByKind(llm) returned non-llm model %q (%q)", pm.Model.ID, pm.Model.Kind)
		}
	}

	// Image models should be present and tagged correctly.
	images := ModelsByKind(core.ServiceImage)
	if len(images) == 0 {
		t.Fatal("expected at least one image model in catalog")
	}
	for _, pm := range images {
		if pm.Model.Kind != core.ServiceImage {
			t.Errorf("ModelsByKind(image) returned non-image model %q", pm.Model.ID)
		}
	}
}

func TestFindModel(t *testing.T) {
	if _, ok := FindModel("openai", "gpt-4o"); !ok {
		t.Error("expected to find openai/gpt-4o")
	}
	if _, ok := FindModel("commandcode", "deepseek/deepseek-v4-pro"); !ok {
		t.Error("expected to find commandcode/deepseek/deepseek-v4-pro")
	}
	if _, ok := FindModel("openai", "nonexistent-model"); ok {
		t.Error("expected miss for nonexistent model")
	}
}

func TestCommandCodeCatalogVisible(t *testing.T) {
	spec, ok := SpecByID("commandcode")
	if !ok {
		t.Fatal("catalog missing commandcode")
	}
	if spec.Hidden {
		t.Fatal("commandcode should be visible")
	}
	if spec.APIKeyURL != "https://commandcode.ai/studio" {
		t.Fatalf("unexpected commandcode APIKeyURL %q", spec.APIKeyURL)
	}
	if len(ModelsForProvider("commandcode")) == 0 {
		t.Fatal("commandcode should have static models")
	}
}

func TestDrivableDialect(t *testing.T) {
	for _, d := range []core.Dialect{
		core.DialectOpenAI, core.DialectAnthropic, core.DialectGemini, core.DialectOllama,
	} {
		if !DrivableDialect(d) {
			t.Errorf("dialect %q must be drivable", d)
		}
	}
	// All proprietary/subscription dialects are now drivable: the full
	// transport surface has been ported.
	for _, d := range []core.Dialect{
		core.DialectKiro, core.DialectGeminiCLI, core.DialectAntigravity,
		core.DialectCommandCode, core.DialectCursor, core.DialectQoder,
	} {
		if !DrivableDialect(d) {
			t.Errorf("dialect %q must be drivable", d)
		}
	}
	// An unknown dialect remains non-drivable.
	if DrivableDialect(core.Dialect("does-not-exist")) {
		t.Error("unknown dialect must not be drivable")
	}
}

func TestRegistryRegistersDrivableProviders(t *testing.T) {
	r := DefaultRegistry()
	// OpenAI/Anthropic chat, Gemini, Ollama, and web providers should be registered.
	for _, id := range []string{"openai", "anthropic", "gemini", "ollama", "ollama-local", "tavily", "firecrawl"} {
		if !r.Has(id) {
			t.Errorf("registry should have connector for %q", id)
		}
	}
	// Every subscription/proprietary provider now has a connector.
	for _, id := range []string{"kiro", "gemini-cli", "antigravity", "commandcode", "cursor", "github", "qwen", "iflow"} {
		if !r.Has(id) {
			t.Errorf("registry should have connector for %q", id)
		}
	}
	// An unknown provider still has no connector.
	if r.Has("does-not-exist") {
		t.Error("registry should not have a connector for an unknown provider")
	}
}
