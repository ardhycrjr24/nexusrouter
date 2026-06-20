---
name: NexusRouter-image
description: Generate images via NexusRouter /v1/images/generations using OpenAI DALL-E / Gemini Imagen / FLUX / MiniMax / Stability AI / Fal.ai models. Use when the user wants to create, generate, draw, or render an image, picture, or text-to-image (txt2img).
---

# NexusRouter — Image Generation

Requires `NexusRouter_URL` (and `NexusRouter_KEY` if auth enabled). See https://raw.githubusercontent.com/ardhycrjr24/NexusRouter/main/skills/NexusRouter/SKILL.md for setup.

## Discover

```bash
curl $NexusRouter_URL/v1/models/image | jq '.data[].id'
# Per-model params/options (size enum, quality enum)
curl "$NexusRouter_URL/v1/models/info?id=openai/dall-e-3"
```

## Endpoint

`POST $NexusRouter_URL/v1/images/generations`

| Field | Required | Notes |
|---|---|---|
| `model` | yes | from `/v1/models/image` |
| `prompt` | yes | image description |
| `n` | no | count (provider-dependent) |
| `size` | no | `1024x1024`, `1792x1024`, ... |
| `quality` | no | `standard` / `hd` (OpenAI) |
| `response_format` | no | `url` (default) or `b64_json` |

Add query `?response_format=binary` to receive raw image bytes (handy for saving file).

## Examples

Save to file (binary):

```bash
curl -X POST "$NexusRouter_URL/v1/images/generations?response_format=binary" \
  -H "Authorization: Bearer $NexusRouter_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"gemini/gemini-3-pro-image-preview","prompt":"watercolor mountains at sunrise","size":"1024x1024"}' \
  --output out.png
```

JS (URL response):

```js
const r = await fetch(`${process.env.NexusRouter_URL}/v1/images/generations`, {
  method: "POST",
  headers: { "Authorization": `Bearer ${process.env.NexusRouter_KEY}`, "Content-Type": "application/json" },
  body: JSON.stringify({ model: "gemini/gemini-3-pro-image-preview", prompt: "neon city", size: "1024x1024" }),
});
const { data } = await r.json();
console.log(data[0].url || data[0].b64_json.slice(0, 40));
```

## Response shape

JSON (default `response_format=url`):
```json
{ "created": 1735000000, "data": [{ "url": "https://..." }] }
```

`response_format=b64_json`:
```json
{ "created": 1735000000, "data": [{ "b64_json": "iVBORw0KGgo..." }] }
```

Query `?response_format=binary` returns raw image bytes (Content-Type `image/png` or `image/jpeg`).

## Provider quick reference

| Provider | ID | Notes |
|---|---|---|
| OpenAI | `openai` | DALL-E 3, standard/hd quality |
| Gemini | `gemini` | Imagen, only `prompt` required |
| MiniMax | `minimax` | Standard OpenAI shape |
| Stability AI | `stability-ai` | `style` preset, `output_format` |
| Fal.ai | `fal-ai` | Async polling, img2img via `image` |
| NanoBanana | `nanobanana` | Edit mode via `image`/`images[]` |
| OpenRouter | `openrouter` | Routes to underlying image models |
