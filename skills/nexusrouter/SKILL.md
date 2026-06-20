---
name: NexusRouter
description: Entry point for NexusRouter — local/remote AI gateway with OpenAI-compatible REST for chat, image, TTS, embeddings, web search, web fetch. Use when the user mentions NexusRouter, NexusRouter_URL, or wants AI without writing provider boilerplate. This skill covers setup + indexes capability skills; fetch the relevant capability SKILL.md from the URLs below when needed.
---

# NexusRouter

Local/remote AI gateway exposing OpenAI-compatible REST. One key, many providers, auto-fallback.

## Setup

```bash
export NexusRouter_URL="http://localhost:20180"      # or VPS / tunnel URL
export NexusRouter_KEY="sk-..."                      # from Dashboard → Keys (only if auth enabled)
```

All requests: `${NexusRouter_URL}/v1/...` with header `Authorization: Bearer ${NexusRouter_KEY}` (omit if auth disabled).

Verify: `curl $NexusRouter_URL/healthz` → `{"ok":true}`

## Discover models

```bash
curl $NexusRouter_URL/v1/models                  # chat/LLM (default)
curl $NexusRouter_URL/v1/models/image            # image-gen
curl $NexusRouter_URL/v1/models/tts              # text-to-speech
curl $NexusRouter_URL/v1/models/embedding        # embeddings
curl $NexusRouter_URL/v1/models/web              # web search + fetch (entries have `kind` field)
curl $NexusRouter_URL/v1/models/stt              # speech-to-text
```

Use `data[].id` as `model` field in requests. Combos appear with `owned_by:"combo"`.

Response shape:
```json
{ "object": "list", "data": [
  { "id": "openai/gpt-5", "object": "model", "owned_by": "openai", "created": 1735000000 },
  { "id": "tavily/search", "object": "model", "kind": "webSearch", "owned_by": "tavily", "created": 1735000000 }
]}
```

## Capability skills

When the user needs a specific capability, fetch that skill's `SKILL.md` from its raw URL:

| Capability | Raw URL |
|---|---|
| Chat / code-gen | https://raw.githubusercontent.com/ardhycrjr24/NexusRouter/main/skills/NexusRouter-chat/SKILL.md |
| Image generation | https://raw.githubusercontent.com/ardhycrjr24/NexusRouter/main/skills/NexusRouter-image/SKILL.md |
| Text-to-speech | https://raw.githubusercontent.com/ardhycrjr24/NexusRouter/main/skills/NexusRouter-tts/SKILL.md |
| Speech-to-text | https://raw.githubusercontent.com/ardhycrjr24/NexusRouter/main/skills/NexusRouter-stt/SKILL.md |
| Embeddings | https://raw.githubusercontent.com/ardhycrjr24/NexusRouter/main/skills/NexusRouter-embeddings/SKILL.md |
| Web search | https://raw.githubusercontent.com/ardhycrjr24/NexusRouter/main/skills/NexusRouter-web-search/SKILL.md |
| Web fetch (URL → markdown) | https://raw.githubusercontent.com/ardhycrjr24/NexusRouter/main/skills/NexusRouter-web-fetch/SKILL.md |

## Supported providers (highlights)

| Provider | ID | Alias | Notes |
|---|---|---|---|
| OpenAI | `openai` | `openai` | GPT-5, GPT-4o, DALL-E, Whisper, TTS |
| Anthropic | `anthropic` | `anthropic` | Claude Opus, Sonnet, Haiku |
| Claude Code | `claude` | `cc` | Claude via Claude Code subscription |
| Google Gemini | `gemini` | `gemini` | Gemini 2.5, Imagen |
| Groq | `groq` | `groq` | Fast inference, Whisper |
| DeepSeek | `deepseek` | `ds` | DeepSeek V3, Coder |
| OpenRouter | `openrouter` | `openrouter` | 100+ models via single key |
| Mistral | `mistral` | `mistral` | Mistral Large, Medium |
| xAI | `xai` | `xai` | Grok models |
| NVIDIA NIM | `nvidia` | `nvidia` | Nemotron, Llama |
| Ollama Local | `ollama-local` | `ollama-local` | Local models, no auth |
| Custom OpenAI | `custom-openai` | `custom-openai` | Any OpenAI-compatible endpoint |
| Custom Anthropic | `custom-anthropic` | `custom-anthropic` | Any Anthropic-compatible endpoint |

Use `provider/model` format: `openai/gpt-5`, `anthropic/claude-opus-4-7`, `groq/whisper-large-v3`.

## Errors

- 401 → set/refresh `NexusRouter_KEY` (Dashboard → Keys)
- 400 `Invalid model format` → check `model` exists in `/v1/models/<kind>`
- 503 `All accounts unavailable` → wait `retry-after` or add another provider account
- 429 `Budget exhausted` → budget limit reached, check Dashboard → Budgets
