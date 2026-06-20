---
name: NexusRouter-stt
description: Speech-to-text via NexusRouter /v1/audio/transcriptions using OpenAI Whisper / Groq / Gemini / Deepgram / AssemblyAI models. Use when the user wants to transcribe audio, convert speech to text, or get subtitles from audio files.
---

# NexusRouter — Speech-to-Text

Requires `NexusRouter_URL` (and `NexusRouter_KEY` if auth enabled). See https://raw.githubusercontent.com/ardhycrjr24/NexusRouter/main/skills/NexusRouter/SKILL.md for setup.

## Discover

```bash
curl $NexusRouter_URL/v1/models/stt | jq '.data[].id'
# Per-model params (language, response_format, prompt, temperature support)
curl "$NexusRouter_URL/v1/models/info?id=openai/whisper-1"
```

`model` = STT model ID (e.g. `openai/whisper-1`, `groq/whisper-large-v3`, `deepgram/nova-3`, `gemini/gemini-2.5-flash`).

## Endpoint

`POST $NexusRouter_URL/v1/audio/transcriptions` (OpenAI Whisper compatible, `multipart/form-data`)

| Field | Required | Notes |
|---|---|---|
| `model` | yes | from `/v1/models/stt` |
| `file` | yes | audio file (mp3, wav, m4a, webm, ogg, flac) |
| `language` | no | ISO-639-1 (e.g. `en`, `vi`) |
| `prompt` | no | hint text to guide transcription |
| `response_format` | no | `json` (default) / `text` / `verbose_json` / `srt` / `vtt` |
| `temperature` | no | 0–1 |

## Examples

```bash
curl -X POST "$NexusRouter_URL/v1/audio/transcriptions" \
  -H "Authorization: Bearer $NexusRouter_KEY" \
  -F "model=openai/whisper-1" \
  -F "file=@audio.mp3" \
  -F "language=en"
```

JS (Node):

```js
import { createReadStream } from "node:fs";
const form = new FormData();
form.append("model", "groq/whisper-large-v3-turbo");
form.append("file", new Blob([await (await import("node:fs/promises")).readFile("audio.mp3")]), "audio.mp3");
const r = await fetch(`${process.env.NexusRouter_URL}/v1/audio/transcriptions`, {
  method: "POST",
  headers: { "Authorization": `Bearer ${process.env.NexusRouter_KEY}` },
  body: form,
});
const { text } = await r.json();
console.log(text);
```

## Response shape

Default (`response_format=json`):
```json
{ "text": "Hello, this is the transcription." }
```

`verbose_json` adds `language`, `duration`, `segments[]` with timestamps.
`srt` / `vtt` return subtitle text.

## Provider quick reference

| Provider | `model` format | Notes |
|---|---|---|
| OpenAI | `whisper-1`, `gpt-4o-transcribe`, `gpt-4o-mini-transcribe` | Native OpenAI shape |
| Groq | `whisper-large-v3`, `whisper-large-v3-turbo` | Fastest; OpenAI shape |
| Gemini | `gemini-2.5-flash`, `gemini-2.5-pro` | Server converts to `generateContent` with audio |
| Deepgram | `nova-3`, `nova-2` | Token auth; server adapts response |
| AssemblyAI | `universal-3-pro`, `universal-2` | Async upload+poll handled server-side |
