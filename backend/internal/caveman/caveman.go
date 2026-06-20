// Package caveman implements output-token compression by injecting a terse
// "caveman speak" instruction into the request's system prompt.
//
// It is a faithful Go port of 9router's caveman injector, which itself adapts
// the caveman skill (https://github.com/JuliusBrussee/caveman). The model is
// instructed to keep all technical substance exact while dropping articles,
// filler, hedging, and pleasantries — cutting roughly 65-75% of output tokens
// without losing meaning.
//
// Like terse mode, this is a request-side transform that only touches the
// system prompt and runs before format translation, so it applies uniformly
// across every provider dialect.
package caveman

import (
	"strings"

	"github.com/ardhycrjr24/nexusrouter/backend/internal/core"
)

// sentinel marks the injected instruction so Apply is idempotent across retries
// and fallbacks. Models ignore HTML-style comments in their output.
const sentinel = "<!-- nexusrouter:caveman -->"

// Level selects how aggressively the model compresses its output.
type Level string

const (
	// LevelLite keeps grammar and full sentences but drops filler and hedging.
	LevelLite Level = "lite"
	// LevelFull uses terse caveman style: fragments, dropped articles, short
	// synonyms, while keeping all technical substance exact.
	LevelFull Level = "full"
	// LevelUltra is maximally compressed and telegraphic, with abbreviations
	// and causal arrows.
	LevelUltra Level = "ultra"
	// LevelWenyanLite is semi-classical: drop filler/hedging but keep grammar
	// structure and classical register.
	LevelWenyanLite Level = "wenyan-lite"
	// LevelWenyanFull is maximum classical terseness (文言文), achieving 80-90%
	// character reduction with classical sentence patterns and particles.
	LevelWenyanFull Level = "wenyan-full"
	// LevelWenyanUltra is extreme abbreviation while keeping classical Chinese
	// feel. Maximum compression combining ultra and wenyan brevity.
	LevelWenyanUltra Level = "wenyan-ultra"
)

// Config controls caveman behavior for a request.
type Config struct {
	Enabled bool
	Level   Level
}

// sharedExamples shows the contrast between filler-heavy and terse replies.
const sharedExamples = "Not: \"Sure! I'd be happy to help you with that. " +
	"The issue you're experiencing is likely caused by...\" " +
	"Yes: \"Bug in auth middleware. Token expiry check use `<` not `<=`. Fix:\""

// sharedExamplesExtra provides additional level-comparison examples used by
// the full and ultra prompts to illustrate the expected compression style.
const sharedExamplesExtra = "Example — \"Why React component re-render?\" " +
	"full: \"New object ref each render. Inline object prop = new ref = re-render. Wrap in `useMemo`.\" " +
	"ultra: \"Inline obj prop → new ref → re-render. `useMemo`.\" " +
	"Example — \"Explain database connection pooling.\" " +
	"full: \"Pool reuse open DB connections. No new connection per request. Skip handshake overhead.\" " +
	"ultra: \"Pool = reuse DB conn. Skip handshake → fast under load.\""

// sharedBoundaries protects content that must never be compressed.
const sharedBoundaries = "Code blocks, file paths, commands, errors, URLs: keep exact. " +
	"No self-reference. Never name or announce the style. No \"caveman mode on\", no third-person tags. " +
	"Output caveman-only — never normal answer plus recap. Exception: user explicitly ask what the mode is. " +
	"Preserve user's dominant language. User write Portuguese → reply Portuguese caveman. " +
	"Compress the style, not the language. No forced English openings or status phrases. " +
	"ALWAYS keep technical terms, code, API names, CLI commands, commit-type keywords (feat/fix/...), " +
	"and exact error strings verbatim — unless user explicitly ask for translation."

// sharedAutoClarity tells the model when to temporarily drop terse style.
const sharedAutoClarity = "Auto-Clarity: drop caveman for security warnings, irreversible actions, " +
	"multi-step sequences where fragment order or omitted conjunctions risk misread, " +
	"compression itself creates technical ambiguity (e.g. \"migrate table drop column backup first\" — " +
	"order unclear without articles/conjunctions), or when user repeats a question. " +
	"Resume caveman after clear part done."

// sharedPersistence keeps the directive active across a long conversation.
const sharedPersistence = "ACTIVE EVERY RESPONSE. No revert after many turns. No filler drift. Still active if unsure."

const promptLite = "Respond tersely. Keep grammar and full sentences but drop filler, hedging and pleasantries " +
	"(just/really/basically/sure/of course/I'd be happy to). " +
	"Pattern: state the thing, the action, the reason. Then next step. " +
	sharedExamples + " " +
	sharedBoundaries + " " +
	sharedAutoClarity + " " +
	sharedPersistence

const promptFull = "Respond terse like smart caveman. All technical substance stay exact, only fluff die. " +
	"Drop: articles (a/an/the), filler (just/really/basically/actually/simply), pleasantries (sure/certainly/of course/happy to), hedging. " +
	"Fragments OK. Short synonyms (big not extensive, fix not implement a solution for). " +
	"No tool-call narration, no decorative tables/emoji, no dumping long raw error logs unless asked — quote shortest decisive line. " +
	"Standard well-known tech acronyms OK (DB/API/HTTP); never invent new abbreviations reader can't decode. " +
	"Technical terms exact. Code blocks unchanged. Errors quoted exact. " +
	"Pattern: [thing] [action] [reason]. [next step]. " +
	sharedExamples + " " +
	sharedExamplesExtra + " " +
	sharedBoundaries + " " +
	sharedAutoClarity + " " +
	sharedPersistence

const promptUltra = "Respond ultra-terse. Maximum compression. Telegraphic. " +
	"Abbreviate prose words (DB/auth/config/req/res/fn/impl) — prose words only, never real code symbols/function names. " +
	"Strip conjunctions, use arrows for causality (X → Y), one word when one word enough. " +
	"Code symbols, function names, API names, error strings: never abbreviate. " +
	"Pattern: [thing] → [result]. [fix]. " +
	sharedExamples + " " +
	sharedExamplesExtra + " " +
	sharedBoundaries + " " +
	sharedAutoClarity + " " +
	sharedPersistence

// promptWenyanLite uses semi-classical style: drop filler/hedging but keep
// grammar structure and classical register.
const promptWenyanLite = "Respond in semi-classical style. Drop filler and hedging but keep grammar structure. " +
	"Use classical register and concise phrasing. Technical terms, code, and API names stay verbatim. " +
	"Preserve user's dominant language — if user writes in Chinese, reply in semi-classical Chinese. " +
	"Pattern: [subject] [verb] [object], classical particles allowed. " +
	sharedBoundaries + " " +
	sharedAutoClarity + " " +
	sharedPersistence

// promptWenyanFull is maximum classical terseness (文言文), achieving 80-90%
// character reduction with classical sentence patterns, verbs preceding objects,
// subjects often omitted, and classical particles (之/乃/為/其).
const promptWenyanFull = "Respond in full 文言文 (classical Chinese) style. Maximum classical terseness. " +
	"80-90% character reduction. Classical sentence patterns: verbs precede objects, subjects often omitted, " +
	"classical particles (之/乃/為/其/矣/也/焉). No modern filler words. " +
	"Technical terms, code, API names, CLI commands: keep verbatim, wrap in classical sentence structure. " +
	"Preserve user's dominant language for technical context. " +
	sharedBoundaries + " " +
	sharedAutoClarity + " " +
	sharedPersistence

// promptWenyanUltra is extreme abbreviation while keeping classical Chinese feel.
// Maximum compression combining ultra telegraphic style with wenyan brevity.
const promptWenyanUltra = "Respond ultra-terse with classical Chinese feel. Extreme abbreviation. " +
	"Maximum compression. Classical particles minimal. One character when one character enough. " +
	"Technical terms, code, API names: keep verbatim. Arrows for causality (→). " +
	"Preserve user's dominant language for technical context. " +
	sharedBoundaries + " " +
	sharedAutoClarity + " " +
	sharedPersistence

// promptFor returns the instruction text for a level, defaulting to full.
func promptFor(l Level) string {
	switch l {
	case LevelLite:
		return promptLite
	case LevelUltra:
		return promptUltra
	case LevelWenyanLite:
		return promptWenyanLite
	case LevelWenyanFull:
		return promptWenyanFull
	case LevelWenyanUltra:
		return promptWenyanUltra
	case LevelFull:
		return promptFull
	default:
		return promptFull
	}
}

// Apply injects the caveman instruction into req.System when cfg.Enabled.
//
// It is a no-op when disabled or when already applied (detected via the
// sentinel), so it is safe across retries and fallback attempts. The block is
// appended after any existing system text so the user's own instructions keep
// priority while the caveman directive still takes effect.
func Apply(req *core.ChatRequest, cfg Config) {
	if req == nil || !cfg.Enabled {
		return
	}
	if strings.Contains(req.System, sentinel) {
		return
	}

	block := sentinel + "\n" + promptFor(cfg.Level)
	if strings.TrimSpace(req.System) == "" {
		req.System = block
		return
	}
	req.System = req.System + "\n\n" + block
}

// ValidLevel reports whether s is a recognized caveman level.
func ValidLevel(s Level) bool {
	switch s {
	case LevelLite, LevelFull, LevelUltra,
		LevelWenyanLite, LevelWenyanFull, LevelWenyanUltra:
		return true
	default:
		return false
	}
}
