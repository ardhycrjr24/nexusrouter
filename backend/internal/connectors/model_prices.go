package connectors

// ModelPrice holds per-million-token rates for a specific model.
type ModelPrice struct {
	Provider        string
	Model           string
	InputPerM       float64 // USD per million standard input tokens
	OutputPerM      float64 // USD per million standard output tokens
	CachedInputPerM float64 // USD per million cache-read input tokens (0 = no cache discount)
	CacheWritePerM  float64 // USD per million cache-write input tokens (0 = no separate rate)
	ReasoningPerM   float64 // USD per million reasoning tokens (0 = no separate rate)
}

// ModelPriceByProviderModel looks up the price for a specific provider/model pair.
func ModelPriceByProviderModel(provider, model string) (ModelPrice, bool) {
	// Try exact "model" field match (may include provider prefix like "openai/gpt-5").
	key := provider + "/" + model
	for _, mp := range ModelPricingTable() {
		if mp.Provider+"/"+mp.Model == key {
			return mp, true
		}
	}
	// Try matching just the model suffix (e.g. model="gpt-4o" matches provider="openai", model="gpt-4o").
	for _, mp := range ModelPricingTable() {
		if mp.Provider == provider && mp.Model == model {
			return mp, true
		}
	}
	return ModelPrice{}, false
}

// ModelPricingTable returns accurate per-model pricing for all major providers.
// Prices are in USD per million tokens. Cached input rates reflect each
// provider's prompt caching discount (typically 50-75% off standard input).
func ModelPricingTable() []ModelPrice {
	var out []ModelPrice
	out = append(out, openaiModelPrices()...)
	out = append(out, anthropicModelPrices()...)
	out = append(out, deepseekModelPrices()...)
	out = append(out, geminiModelPrices()...)
	out = append(out, groqModelPrices()...)
	out = append(out, mistralModelPrices()...)
	out = append(out, xaiModelPrices()...)
	out = append(out, perplexityModelPrices()...)
	out = append(out, cohereModelPrices()...)
	out = append(out, togetherModelPrices()...)
	out = append(out, fireworksModelPrices()...)
	out = append(out, cerebrasModelPrices()...)
	out = append(out, nebiusModelPrices()...)
	out = append(out, nvidiaModelPrices()...)
	out = append(out, openrouterModelPrices()...)
	out = append(out, minimaxModelPrices()...)
	out = append(out, glmModelPrices()...)
	out = append(out, mimoModelPrices()...)
	out = append(out, kiroModelPrices()...)
	return out
}

// kiroModelPrices returns estimated per-million-token rates for the Kiro
// provider. Kiro itself is subscription/credit-based rather than per-token, so
// these are retail-equivalent estimates of the underlying model (e.g. Claude
// Sonnet/Opus), surfaced so usage statistics can display an approximate cost.
//
// The recorded model ID is the client-facing string with synthetic suffixes
// intact (-thinking, -agentic, -thinking-agentic), so each base model is
// expanded into all four variants at the same rate.
func kiroModelPrices() []ModelPrice {
	// base holds one logical model and its rate; variants are generated below.
	type base struct {
		model           string
		inputPerM       float64
		outputPerM      float64
		cachedInputPerM float64
		cacheWritePerM  float64
		reasoningPerM   float64
	}
	const (
		sonnetIn, sonnetOut, sonnetCached, sonnetWrite = 3.0, 15.0, 0.375, 3.75
		opusIn, opusOut, opusCached, opusWrite         = 15.0, 75.0, 1.875, 18.75
	)
	bases := []base{
		// Claude Sonnet family (current + announced versions).
		{"claude-sonnet-4.5", sonnetIn, sonnetOut, sonnetCached, sonnetWrite, 0},
		{"claude-sonnet-4.6", sonnetIn, sonnetOut, sonnetCached, sonnetWrite, 0},
		{"claude-sonnet-4.7", sonnetIn, sonnetOut, sonnetCached, sonnetWrite, 0},
		{"claude-sonnet-4.8", sonnetIn, sonnetOut, sonnetCached, sonnetWrite, 0},
		// Claude Opus family.
		{"claude-opus-4.6", opusIn, opusOut, opusCached, opusWrite, 0},
		{"claude-opus-4.7", opusIn, opusOut, opusCached, opusWrite, 0},
		{"claude-opus-4.8", opusIn, opusOut, opusCached, opusWrite, 0},
		// Claude Haiku family.
		{"claude-haiku-4.5", 0.8, 4.0, 0.08, 1.0, 0},
		// Non-Claude models exposed by Kiro (estimated from underlying provider).
		{"deepseek-3.2", 0.27, 1.1, 0.07, 0.27, 0},
		{"glm-5", 0.6, 2.2, 0, 0, 0},
		{"MiniMax-M2.5", 0.3, 1.1, 0, 0, 0},
		{"qwen3-coder-next", 0.3, 1.2, 0, 0, 0},
	}
	// Suffix variants share the base rate. "auto" maps to Sonnet 4.5 as a
	// neutral default since Kiro picks the model server-side.
	suffixes := []string{"", "-thinking", "-agentic", "-thinking-agentic"}

	var out []ModelPrice
	add := func(model string, in, outp, cached, write, reason float64) {
		for _, sfx := range suffixes {
			out = append(out, ModelPrice{
				Provider: "kiro", Model: model + sfx,
				InputPerM: in, OutputPerM: outp,
				CachedInputPerM: cached, CacheWritePerM: write, ReasoningPerM: reason,
			})
		}
	}
	for _, b := range bases {
		add(b.model, b.inputPerM, b.outputPerM, b.cachedInputPerM, b.cacheWritePerM, b.reasoningPerM)
	}
	// "auto" only exposes base + -thinking variants in the catalog.
	out = append(out,
		ModelPrice{Provider: "kiro", Model: "auto", InputPerM: sonnetIn, OutputPerM: sonnetOut, CachedInputPerM: sonnetCached, CacheWritePerM: sonnetWrite},
		ModelPrice{Provider: "kiro", Model: "auto-thinking", InputPerM: sonnetIn, OutputPerM: sonnetOut, CachedInputPerM: sonnetCached, CacheWritePerM: sonnetWrite},
	)
	return out
}

func openaiModelPrices() []ModelPrice {
	return []ModelPrice{
		// GPT-5 family — cache write = standard input (OpenAI doesn't charge extra)
		{Provider: "openai", Model: "gpt-5", InputPerM: 2.5, OutputPerM: 10, CachedInputPerM: 1.25, CacheWritePerM: 2.5},
		{Provider: "openai", Model: "gpt-5-mini", InputPerM: 0.4, OutputPerM: 1.6, CachedInputPerM: 0.2, CacheWritePerM: 0.4},
		{Provider: "openai", Model: "gpt-5-nano", InputPerM: 0.1, OutputPerM: 0.4, CachedInputPerM: 0.05, CacheWritePerM: 0.1},
		{Provider: "openai", Model: "gpt-5.4", InputPerM: 2.5, OutputPerM: 10, CachedInputPerM: 1.25, CacheWritePerM: 2.5},
		{Provider: "openai", Model: "gpt-5.4-mini", InputPerM: 0.4, OutputPerM: 1.6, CachedInputPerM: 0.2, CacheWritePerM: 0.4},
		{Provider: "openai", Model: "gpt-5.3-codex", InputPerM: 2.5, OutputPerM: 10, CachedInputPerM: 1.25, CacheWritePerM: 2.5},
		// GPT-4o family
		{Provider: "openai", Model: "gpt-4o", InputPerM: 2.5, OutputPerM: 10, CachedInputPerM: 1.25, CacheWritePerM: 2.5},
		{Provider: "openai", Model: "gpt-4o-2024-11-20", InputPerM: 2.5, OutputPerM: 10, CachedInputPerM: 1.25, CacheWritePerM: 2.5},
		{Provider: "openai", Model: "gpt-4o-2024-08-06", InputPerM: 2.5, OutputPerM: 10, CachedInputPerM: 1.25, CacheWritePerM: 2.5},
		{Provider: "openai", Model: "gpt-4o-mini", InputPerM: 0.15, OutputPerM: 0.6, CachedInputPerM: 0.075, CacheWritePerM: 0.15},
		{Provider: "openai", Model: "gpt-4o-mini-2024-07-18", InputPerM: 0.15, OutputPerM: 0.6, CachedInputPerM: 0.075, CacheWritePerM: 0.15},
		// o-series (reasoning) — cache write = standard input
		{Provider: "openai", Model: "o1", InputPerM: 15, OutputPerM: 60, CachedInputPerM: 7.5, CacheWritePerM: 15, ReasoningPerM: 60},
		{Provider: "openai", Model: "o1-pro", InputPerM: 150, OutputPerM: 600, CachedInputPerM: 75, CacheWritePerM: 150, ReasoningPerM: 600},
		{Provider: "openai", Model: "o3", InputPerM: 2, OutputPerM: 8, CachedInputPerM: 0.5, CacheWritePerM: 2, ReasoningPerM: 8},
		{Provider: "openai", Model: "o3-mini", InputPerM: 1.1, OutputPerM: 4.4, CachedInputPerM: 0.55, CacheWritePerM: 1.1, ReasoningPerM: 4.4},
		{Provider: "openai", Model: "o4-mini", InputPerM: 1.1, OutputPerM: 4.4, CachedInputPerM: 0.275, CacheWritePerM: 1.1, ReasoningPerM: 4.4},
		// Older models (no prompt caching)
		{Provider: "openai", Model: "gpt-4-turbo", InputPerM: 10, OutputPerM: 30},
		{Provider: "openai", Model: "gpt-4", InputPerM: 30, OutputPerM: 60},
		{Provider: "openai", Model: "gpt-3.5-turbo", InputPerM: 0.5, OutputPerM: 1.5},
		// Embeddings
		{Provider: "openai", Model: "text-embedding-3-small", InputPerM: 0.02, OutputPerM: 0},
		{Provider: "openai", Model: "text-embedding-3-large", InputPerM: 0.13, OutputPerM: 0},
		{Provider: "openai", Model: "text-embedding-ada-002", InputPerM: 0.1, OutputPerM: 0},
	}
}

func anthropicModelPrices() []ModelPrice {
	return []ModelPrice{
		// Claude 4 family — cache write = 1.25x standard input
		{Provider: "anthropic", Model: "claude-opus-4-20250514", InputPerM: 15, OutputPerM: 75, CachedInputPerM: 1.875, CacheWritePerM: 18.75},
		{Provider: "anthropic", Model: "claude-opus-4-7", InputPerM: 15, OutputPerM: 75, CachedInputPerM: 1.875, CacheWritePerM: 18.75},
		{Provider: "anthropic", Model: "claude-sonnet-4-20250514", InputPerM: 3, OutputPerM: 15, CachedInputPerM: 0.375, CacheWritePerM: 3.75},
		{Provider: "anthropic", Model: "claude-sonnet-4-6", InputPerM: 3, OutputPerM: 15, CachedInputPerM: 0.375, CacheWritePerM: 3.75},
		{Provider: "anthropic", Model: "claude-haiku-4-5-20251001", InputPerM: 0.8, OutputPerM: 4, CachedInputPerM: 0.08, CacheWritePerM: 1.0},
		// Claude 3.5 family
		{Provider: "anthropic", Model: "claude-3-5-sonnet-20241022", InputPerM: 3, OutputPerM: 15, CachedInputPerM: 0.375, CacheWritePerM: 3.75},
		{Provider: "anthropic", Model: "claude-3-5-sonnet-latest", InputPerM: 3, OutputPerM: 15, CachedInputPerM: 0.375, CacheWritePerM: 3.75},
		{Provider: "anthropic", Model: "claude-3-5-haiku-20241022", InputPerM: 0.8, OutputPerM: 4, CachedInputPerM: 0.08, CacheWritePerM: 1.0},
		// Claude 3 family
		{Provider: "anthropic", Model: "claude-3-opus-20240229", InputPerM: 15, OutputPerM: 75, CachedInputPerM: 1.875, CacheWritePerM: 18.75},
		{Provider: "anthropic", Model: "claude-3-sonnet-20240229", InputPerM: 3, OutputPerM: 15, CachedInputPerM: 0.375, CacheWritePerM: 3.75},
		{Provider: "anthropic", Model: "claude-3-haiku-20240307", InputPerM: 0.25, OutputPerM: 1.25, CachedInputPerM: 0.03, CacheWritePerM: 0.3125},
	}
}

func deepseekModelPrices() []ModelPrice {
	return []ModelPrice{
		{Provider: "deepseek", Model: "deepseek-chat", InputPerM: 0.27, OutputPerM: 1.1, CachedInputPerM: 0.07, CacheWritePerM: 0.27},
		{Provider: "deepseek", Model: "deepseek-coder", InputPerM: 0.27, OutputPerM: 1.1, CachedInputPerM: 0.07, CacheWritePerM: 0.27},
		{Provider: "deepseek", Model: "deepseek-reasoner", InputPerM: 0.55, OutputPerM: 2.19, CachedInputPerM: 0.14, CacheWritePerM: 0.55, ReasoningPerM: 2.19},
	}
}

func geminiModelPrices() []ModelPrice {
	return []ModelPrice{
		// Gemini 2.5 — cache write = standard input
		{Provider: "gemini", Model: "gemini-2.5-pro", InputPerM: 1.25, OutputPerM: 10, CachedInputPerM: 0.3125, CacheWritePerM: 1.25},
		{Provider: "gemini", Model: "gemini-2.5-flash", InputPerM: 0.15, OutputPerM: 0.6, CachedInputPerM: 0.0375, CacheWritePerM: 0.15},
		{Provider: "gemini", Model: "gemini-2.5-flash-lite", InputPerM: 0.075, OutputPerM: 0.3, CachedInputPerM: 0.01875, CacheWritePerM: 0.075},
		// Gemini 2.0
		{Provider: "gemini", Model: "gemini-2.0-flash", InputPerM: 0.1, OutputPerM: 0.4, CachedInputPerM: 0.025, CacheWritePerM: 0.1},
		{Provider: "gemini", Model: "gemini-2.0-flash-lite", InputPerM: 0.075, OutputPerM: 0.3, CachedInputPerM: 0.01875, CacheWritePerM: 0.075},
		// Gemini 1.5
		{Provider: "gemini", Model: "gemini-1.5-pro", InputPerM: 1.25, OutputPerM: 5, CachedInputPerM: 0.3125, CacheWritePerM: 1.25},
		{Provider: "gemini", Model: "gemini-1.5-flash", InputPerM: 0.075, OutputPerM: 0.3, CachedInputPerM: 0.01875, CacheWritePerM: 0.075},
	}
}

func groqModelPrices() []ModelPrice {
	return []ModelPrice{
		{Provider: "groq", Model: "llama-3.3-70b-versatile", InputPerM: 0.59, OutputPerM: 0.79},
		{Provider: "groq", Model: "llama-3.1-8b-instant", InputPerM: 0.05, OutputPerM: 0.08},
		{Provider: "groq", Model: "mixtral-8x7b-32768", InputPerM: 0.24, OutputPerM: 0.24},
		{Provider: "groq", Model: "gemma2-9b-it", InputPerM: 0.2, OutputPerM: 0.2},
		{Provider: "groq", Model: "whisper-large-v3", InputPerM: 0.0, OutputPerM: 0.0},
		{Provider: "groq", Model: "whisper-large-v3-turbo", InputPerM: 0.0, OutputPerM: 0.0},
	}
}

func mistralModelPrices() []ModelPrice {
	return []ModelPrice{
		{Provider: "mistral", Model: "mistral-large-latest", InputPerM: 2, OutputPerM: 6},
		{Provider: "mistral", Model: "mistral-small-latest", InputPerM: 0.1, OutputPerM: 0.3},
		{Provider: "mistral", Model: "codestral-latest", InputPerM: 0.3, OutputPerM: 0.9},
		{Provider: "mistral", Model: "pixtral-large-latest", InputPerM: 2, OutputPerM: 6},
	}
}

func xaiModelPrices() []ModelPrice {
	return []ModelPrice{
		{Provider: "xai", Model: "grok-3", InputPerM: 3, OutputPerM: 15, CachedInputPerM: 0.75, CacheWritePerM: 3},
		{Provider: "xai", Model: "grok-3-fast", InputPerM: 5, OutputPerM: 25, CachedInputPerM: 1.25, CacheWritePerM: 5},
		{Provider: "xai", Model: "grok-3-mini", InputPerM: 0.3, OutputPerM: 0.5, CachedInputPerM: 0.075, CacheWritePerM: 0.3, ReasoningPerM: 0.5},
		{Provider: "xai", Model: "grok-2", InputPerM: 2, OutputPerM: 10, CachedInputPerM: 0.5, CacheWritePerM: 2},
	}
}

func perplexityModelPrices() []ModelPrice {
	return []ModelPrice{
		{Provider: "perplexity", Model: "sonar-pro", InputPerM: 3, OutputPerM: 15},
		{Provider: "perplexity", Model: "sonar", InputPerM: 1, OutputPerM: 1},
		{Provider: "perplexity", Model: "sonar-reasoning-pro", InputPerM: 2, OutputPerM: 8},
		{Provider: "perplexity", Model: "sonar-deep-research", InputPerM: 2, OutputPerM: 8},
	}
}

func cohereModelPrices() []ModelPrice {
	return []ModelPrice{
		{Provider: "cohere", Model: "command-r-plus", InputPerM: 2.5, OutputPerM: 10},
		{Provider: "cohere", Model: "command-r", InputPerM: 0.15, OutputPerM: 0.6},
	}
}

func togetherModelPrices() []ModelPrice {
	return []ModelPrice{
		{Provider: "together", Model: "meta-llama/Meta-Llama-3.1-405B-Instruct-Turbo", InputPerM: 3.5, OutputPerM: 3.5},
		{Provider: "together", Model: "meta-llama/Meta-Llama-3.1-70B-Instruct-Turbo", InputPerM: 0.88, OutputPerM: 0.88},
		{Provider: "together", Model: "meta-llama/Meta-Llama-3.1-8B-Instruct-Turbo", InputPerM: 0.18, OutputPerM: 0.18},
		{Provider: "together", Model: "deepseek-ai/DeepSeek-V3", InputPerM: 1.25, OutputPerM: 1.25},
		{Provider: "together", Model: "Qwen/Qwen2.5-72B-Instruct-Turbo", InputPerM: 1.2, OutputPerM: 1.2},
	}
}

func fireworksModelPrices() []ModelPrice {
	return []ModelPrice{
		{Provider: "fireworks", Model: "accounts/fireworks/models/llama-v3p1-405b-instruct", InputPerM: 3, OutputPerM: 3},
		{Provider: "fireworks", Model: "accounts/fireworks/models/llama-v3p1-70b-instruct", InputPerM: 0.9, OutputPerM: 0.9},
		{Provider: "fireworks", Model: "accounts/fireworks/models/llama-v3p1-8b-instruct", InputPerM: 0.2, OutputPerM: 0.2},
		{Provider: "fireworks", Model: "accounts/fireworks/models/deepseek-v3", InputPerM: 0.9, OutputPerM: 0.9},
	}
}

func cerebrasModelPrices() []ModelPrice {
	return []ModelPrice{
		{Provider: "cerebras", Model: "llama3.1-8b", InputPerM: 0.1, OutputPerM: 0.1},
		{Provider: "cerebras", Model: "llama3.1-70b", InputPerM: 0.6, OutputPerM: 0.6},
		{Provider: "cerebras", Model: "llama-3.3-70b", InputPerM: 0.6, OutputPerM: 0.6},
	}
}

func nebiusModelPrices() []ModelPrice {
	return []ModelPrice{
		{Provider: "nebius", Model: "Meta-Llama-3.1-405B-Instruct", InputPerM: 1, OutputPerM: 1},
		{Provider: "nebius", Model: "Meta-Llama-3.1-70B-Instruct", InputPerM: 0.13, OutputPerM: 0.13},
		{Provider: "nebius", Model: "Qwen/Qwen2.5-72B-Instruct", InputPerM: 0.13, OutputPerM: 0.13},
	}
}

func nvidiaModelPrices() []ModelPrice {
	return []ModelPrice{
		{Provider: "nvidia", Model: "meta/llama-3.1-405b-instruct", InputPerM: 1, OutputPerM: 1},
		{Provider: "nvidia", Model: "meta/llama-3.1-70b-instruct", InputPerM: 0.13, OutputPerM: 0.13},
		{Provider: "nvidia", Model: "nvidia/llama-3.1-nemotron-70b-instruct", InputPerM: 0.13, OutputPerM: 0.13},
	}
}

func openrouterModelPrices() []ModelPrice {
	// OpenRouter prices are pass-through to underlying providers.
	// These are typical markups; actual prices vary by model.
	return []ModelPrice{
		{Provider: "openrouter", Model: "anthropic/claude-opus-4-7", InputPerM: 15, OutputPerM: 75, CachedInputPerM: 1.875, CacheWritePerM: 18.75},
		{Provider: "openrouter", Model: "anthropic/claude-sonnet-4-6", InputPerM: 3, OutputPerM: 15, CachedInputPerM: 0.375, CacheWritePerM: 3.75},
		{Provider: "openrouter", Model: "openai/gpt-5", InputPerM: 2.5, OutputPerM: 10, CachedInputPerM: 1.25, CacheWritePerM: 2.5},
		{Provider: "openrouter", Model: "openai/gpt-4o", InputPerM: 2.5, OutputPerM: 10, CachedInputPerM: 1.25, CacheWritePerM: 2.5},
		{Provider: "openrouter", Model: "openai/gpt-4o-mini", InputPerM: 0.15, OutputPerM: 0.6, CachedInputPerM: 0.075, CacheWritePerM: 0.15},
		{Provider: "openrouter", Model: "deepseek/deepseek-chat", InputPerM: 0.27, OutputPerM: 1.1, CachedInputPerM: 0.07, CacheWritePerM: 0.27},
		{Provider: "openrouter", Model: "google/gemini-2.5-pro", InputPerM: 1.25, OutputPerM: 10, CachedInputPerM: 0.3125, CacheWritePerM: 1.25},
		{Provider: "openrouter", Model: "google/gemini-2.5-flash", InputPerM: 0.15, OutputPerM: 0.6, CachedInputPerM: 0.0375, CacheWritePerM: 0.15},
		{Provider: "openrouter", Model: "meta-llama/llama-3.3-70b-instruct", InputPerM: 0.1, OutputPerM: 0.1},
	}
}

func minimaxModelPrices() []ModelPrice {
	return []ModelPrice{
		{Provider: "minimax", Model: "MiniMax-Text-01", InputPerM: 0.2, OutputPerM: 1.1},
		{Provider: "minimax", Model: "MiniMax-M1", InputPerM: 0.2, OutputPerM: 1.1, ReasoningPerM: 1.1},
		{Provider: "minimax", Model: "MiniMax-M2.5", InputPerM: 0.3, OutputPerM: 1.1},
		{Provider: "minimax", Model: "MiniMax-M3", InputPerM: 0.4, OutputPerM: 1.6, ReasoningPerM: 1.6},
	}
}

func glmModelPrices() []ModelPrice {
	return []ModelPrice{
		{Provider: "glm", Model: "glm-4-plus", InputPerM: 0.6, OutputPerM: 0.6},
		{Provider: "glm", Model: "glm-4-flash", InputPerM: 0, OutputPerM: 0},
		{Provider: "glm", Model: "codegeex-4", InputPerM: 0.6, OutputPerM: 0.6},
	}
}

func mimoModelPrices() []ModelPrice {
	// Xiaomi MiMo pricing. Both xiaomi-mimo and xiaomi-tokenplan share models.
	// Flash pricing from genai-prices; Pro/standard tiers estimated from
	// comparable Chinese AI provider pricing (DeepSeek, Alibaba, GLM).
	var out []ModelPrice
	for _, provider := range []string{"xiaomi-mimo", "xiaomi-tokenplan"} {
		out = append(out, []ModelPrice{
			// Top-tier reasoning model
			{Provider: provider, Model: "mimo-v2.5-pro", InputPerM: 1.0, OutputPerM: 3.0},
			// Mid-tier general model
			{Provider: provider, Model: "mimo-v2.5", InputPerM: 0.2, OutputPerM: 0.6},
			// Previous-gen pro
			{Provider: provider, Model: "mimo-v2-pro", InputPerM: 0.5, OutputPerM: 1.5},
			// Multimodal (omni)
			{Provider: provider, Model: "mimo-v2-omni", InputPerM: 0.2, OutputPerM: 0.6},
			// Fast/cheap model (from genai-prices: $0.10/$0.30)
			{Provider: provider, Model: "mimo-v2-flash", InputPerM: 0.1, OutputPerM: 0.3},
		}...)
	}
	// mimo-free: free tier, zero cost.
	out = append(out, ModelPrice{Provider: "mimo-free", Model: "mimo-auto", InputPerM: 0, OutputPerM: 0})
	return out
}
