# pfui

pfui is a terminal-first coding assistant that merges the best ideas from Codex CLI and Claude Code. It provides a scroll-safe chat interface, configuration wizards for setting up OpenAI/Anthropic/custom providers, MCP scope management, and a `/model` explorer that pulls live metadata from every configured backend.

This repository contains the Go implementation, documentation, and CI workflows needed to ship production builds for Linux, macOS, Windows, and the major BSD variants (via cross-compilation).

## Highlights

- **Scroll-safe chat UI** – Bubble Tea–based TUI keeps the input dock pinned to the bottom while history streams upward, preserving terminal scrollback.
- **Configuration wizard** – `pfui --configuration` (or `/config` inside the chat) opens a first-launch experience that handles subscriptions, API keys, custom providers, and MCP servers.
- **Slash-command parity** – Stubs for `/model`, `/plan`, `/approvals`, `/resume`, `/config`, `/mcp`, `/provider`, `/jobs`, and `/usage` mirror Codex/Claude ergonomics. Implementation will be expanded incrementally.
- **Dual-mode shell exec** – pfui exposes a tool to the agent (not users) that can run shell commands in the foreground (ESC-cancelable) or background (tracked via `/jobs` indicators) just like Claude Code’s background runners.
- **Custom providers & MCP** – `pfui provider init` and `pfui mcp add` scaffold manifests in `~/.pfui`, making it easy to plug in connectors like z.ai through OpenAI- or Anthropic-compatible adapters.
- **Model whitelists** – Administrators can optionally limit the `/model` picker per provider (OpenAI, Claude, custom adapters). Leave defaults open for built-ins and whitelist just the custom connectors that need it.
- **Provider-aware chats** – Enable/disable OpenAI and Claude independently in `config.toml` and use `/provider` or the startup prompt to pick GPT-5.1-Codex, Claude 4.5 Sonnet, or any future connector before each chat.
- **One-click OAuth** – The configuration wizard reproduces Claude Code and Codex CLI’s OAuth flows: sign in with ChatGPT Plus to mint a fresh API key or link a Claude Pro/Max subscription (including 1M-token context) without leaving the terminal.
- **Built for terminal purists** – “Pfui” literally means “eww” in German. It’s a nod to how other AI CLIs trash scrollback, hijack alternate screens, or ignore sysadmin workflows. pfui exists so hardcore UNIX operators finally get an agent that respects their terminals, stays compatible with Claude Code habits, and still speaks GPT-5-Codex fluently.

## Agent contract

`internal/systemprompt` builds the composite instruction block we send to the active provider. It merges Codex CLI approvals with Claude Code plan/auto/off semantics, enumerates slash commands so the model can mention them, and advertises the exec tool schema:

```
exec {
  background?: bool = false,
  command: string,
  args?: string[],
  workdir?: string
}
```

Foreground execs stream inline and can be canceled with ESC; background runs keep going and show up in the `/jobs` overlay. The system prompt also reminds the model to avoid breaking scrollback, announce risky operations, and honor MCP scopes.

Search guidance lives in the same prompt: pfui probes `$PATH` for `ast-grep`, `rg`, and `grep`, then tells the model to prefer them in that order whenever it needs to scan code or text. If none are available it instructs the agent to ask before reaching for something slower or less structured.

### Plan mode + PLAN.md

`/plan` already mirrors Codex CLI’s checklist UX inside the TUI; now you can manage the Markdown copy Claude Code likes to keep in `PLAN.md` as well. Set `[plan] storage = "file"` in `~/.pfui/config.toml` (or pick "Plan Storage" inside the wizard) to mirror your steps to disk. Enable `auto_write = true` to sync the file after every edit, or leave it off and run `/plan save [path]` whenever you want a fresh export. Plans always stay in memory for the drawer—even when you write them to disk—so you get the best of both worlds.

## Getting started

```bash
git clone https://github.com/fbettag/pfui
cd pfui
go build ./cmd/pfui
./pfui --configuration   # run the wizard (may clear scrollback)
./pfui                   # launch the scroll-safe chat TUI
```

### Configuration

pfui looks for `~/.pfui/config.toml` by default. Use `config/example_config.toml` as a template:

```toml
# [models]
# whitelist = ["global-model"]
#
# [models.provider_whitelist]
# openai = ["gpt-5.1-codex"]
# claude = ["claude-4.5-sonnet"]
# my-custom-proxy = ["zai-ultra"]
```

Provider-specific entries take precedence. If you only want to constrain custom adapters, leave the global list commented out and populate `provider_whitelist` with the connector names you care about.

Set provider toggles to choose which built-in connectors are active (e.g., disable Anthropic on hosts that only run GPT-5 Codex, or vice versa).

For OAuth-based sign-ins, pfui defaults to the official Claude Code and Codex CLI client IDs. If you have enterprise-specific credentials, export `PFUI_ANTHROPIC_CLIENT_ID` and/or `PFUI_OPENAI_CLIENT_ID` before running `pfui --configuration` so the wizard uses your custom IDs.

### Managing credentials

- `pfui auth status` — list which providers have API keys or OAuth refresh tokens on disk and when they expire.
- `pfui auth refresh [--provider openai|anthropic]` — rotate Claude or ChatGPT credentials (refresh tokens and mint new API keys) without re-running the wizard.

### Provider & MCP helpers

- `pfui provider init NAME --adapter openai-chat --host https://api.example.com --token sk-...`
- `pfui mcp add search --scope project --url http://localhost:8000/mcp`

Both commands persist manifests under `~/.pfui` (or `.pfui` inside the project for `--scope project`).

## Development

- `go test ./...` – run unit tests (currently focused on configuration utilities).
- `go fmt ./...` – format code.
- `golangci-lint run` – optional linting (not bundled; install separately).

### GitHub workflows

`.github/workflows/ci.yml` runs on push/PR:

- **test** job executes `go test ./...` on ubuntu-latest, macos-latest, and windows-latest.
- **build-matrix** cross-compiles `pfui` for Linux, macOS, Windows, FreeBSD, OpenBSD, and NetBSD (amd64) to ensure every supported platform stays healthy.

## Roadmap

See `plan.md` for the detailed implementation plan covering the startup wizard, model explorer, MCP tooling, and provider abstractions. Contributions should align with that plan; open an issue before deviating significantly.
