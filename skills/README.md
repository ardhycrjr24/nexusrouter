# NexusRouter — Agent Skills

Drop-in skills for any AI agent (Claude, Cursor, ChatGPT, custom SDK). Just **copy a link** below and paste it to your AI — it will fetch the skill and use NexusRouter for you.

> Tip: start with the **NexusRouter** entry skill — it covers setup and links to all capability skills.

## Skills

| Capability | Copy link below and paste to your AI |
|---|---|
| **Entry / Setup** (start here) | https://raw.githubusercontent.com/ardhycrjr24/NexusRouter/main/skills/NexusRouter/SKILL.md |
| Chat / code-gen | https://raw.githubusercontent.com/ardhycrjr24/NexusRouter/main/skills/NexusRouter-chat/SKILL.md |
| Image generation | https://raw.githubusercontent.com/ardhycrjr24/NexusRouter/main/skills/NexusRouter-image/SKILL.md |
| Text-to-speech | https://raw.githubusercontent.com/ardhycrjr24/NexusRouter/main/skills/NexusRouter-tts/SKILL.md |
| Speech-to-text | https://raw.githubusercontent.com/ardhycrjr24/NexusRouter/main/skills/NexusRouter-stt/SKILL.md |
| Embeddings | https://raw.githubusercontent.com/ardhycrjr24/NexusRouter/main/skills/NexusRouter-embeddings/SKILL.md |
| Web search | https://raw.githubusercontent.com/ardhycrjr24/NexusRouter/main/skills/NexusRouter-web-search/SKILL.md |
| Web fetch (URL → markdown) | https://raw.githubusercontent.com/ardhycrjr24/NexusRouter/main/skills/NexusRouter-web-fetch/SKILL.md |

## How to use

Paste to your AI (Claude, Cursor, ChatGPT, …):

```
Read this skill and use it: https://raw.githubusercontent.com/ardhycrjr24/NexusRouter/main/skills/NexusRouter/SKILL.md
```

Then ask normally — *"generate an image of a cat"*, *"transcribe this URL"*, etc.

## Configure your shell once

```bash
export NexusRouter_URL="http://localhost:20180"   # local default, or your VPS / tunnel URL
export NexusRouter_KEY="sk-..."                   # from Dashboard → Keys (only if auth enabled)
```

Verify: `curl $NexusRouter_URL/healthz` → `{"ok":true}`.

## Links

- Source: https://github.com/ardhycrjr24/NexusRouter
- Dashboard: http://localhost:20180 (local)
