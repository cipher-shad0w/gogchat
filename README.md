# gogchat

**Google Chat from your terminal.**

[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![Python](https://img.shields.io/badge/Python-3.10+-3776AB?logo=python&logoColor=white)](https://python.org)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![GitHub Release](https://img.shields.io/github/v/release/cipher-shad0w/gogchat)](https://github.com/cipher-shad0w/gogchat/releases)

gogchat is a fast, full-featured CLI and TUI for the Google Chat API. Manage spaces, send messages, handle reactions, upload media, and more — all without leaving your terminal. A companion TUI provides a rich, interactive chat experience built with Python and Textual.

## Features

- **Full Google Chat API coverage** — spaces, messages, members, reactions, attachments, emoji, media, events, read state, and notifications
- **Terminal UI** — interactive chat interface with space navigation, message editing, reactions, and desktop notifications
- **JSON output** — structured output for scripting and automation (`--json`)
- **OAuth2 authentication** — browser-based login with built-in credentials; no setup required
- **Admin operations** — manage spaces and members as a Workspace admin (`--admin`)
- **Media upload & download** — attach and retrieve files from messages
- **Custom emoji** — create, list, and manage custom emoji for your organization
- **Cross-platform** — macOS, Linux, and Windows; amd64 and arm64

## Installation

### Homebrew (macOS & Linux)

```bash
brew tap cipher-shad0w/tap
brew install gogchat
```

### Prebuilt binaries

Download the latest release for your platform from [GitHub Releases](https://github.com/cipher-shad0w/gogchat/releases).

### Build from source

Requires Go 1.25+:

```bash
go install github.com/cipher-shad0w/gogchat/cmd/gogchat@latest
```

> **Note:** When building from source, you must supply your own Google OAuth2 credentials.
> See [CLI.md](CLI.md) for details.

### TUI

The terminal UI is a separate Python package. Requires Python 3.10+:

```bash
pip install gogchat-tui
```

Or with [uv](https://github.com/astral-sh/uv):

```bash
uv tool install gogchat-tui
```

## Quick start

```bash
# Authenticate with your Google account
gogchat auth login

# List your spaces
gogchat spaces list

# Send a message
gogchat messages send spaces/SPACE_ID --text "Hello from the CLI!"

# List members of a space
gogchat members list spaces/SPACE_ID

# Add a reaction
gogchat reactions add spaces/SPACE_ID/messages/MSG_ID --emoji "👍"

# Upload a file
gogchat media upload spaces/SPACE_ID --file ./report.pdf
```

## Terminal UI

Launch the interactive chat interface:

```bash
gogchat-tui
```

The TUI provides a full chat experience — space navigation, message display with rich text, inline reactions, message editing and deletion, a reaction picker, space management, desktop notifications, and local caching.

## Configuration

Configuration is resolved in the following order (highest precedence first):

1. **CLI flags** — `--config`, `--json`, `--admin`, `--quiet`, `--verbose`
2. **Environment variables** — prefixed with `GOGCHAT_`
3. **Config file** — `~/.config/gogchat/config.yaml`

### Global flags

| Flag | Description |
|------|-------------|
| `--json`, `-j` | Output as JSON |
| `--admin` | Use admin/domain-wide privileges |
| `--quiet`, `-q` | Suppress non-essential output |
| `--verbose`, `-v` | Enable verbose logging |
| `--config` | Path to config file |

### Environment variables

| Variable | Description |
|----------|-------------|
| `GOGCHAT_CONFIG` | Path to config file |
| `GOGCHAT_OUTPUT` | Default output format |
| `GOGCHAT_CLIENT_ID` | Custom OAuth2 client ID |
| `GOGCHAT_CLIENT_SECRET` | Custom OAuth2 client secret |
| `GOGCHAT_CREDENTIALS` | Path to credentials JSON file |
| `NO_COLOR` | Disable colored output |

### Exit codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | General error |
| 2 | Authentication error |
| 3 | Permission denied |
| 4 | Not found |
| 5 | Rate limited |

## Documentation

See **[CLI.md](CLI.md)** for the full command reference covering all 11 command groups and their subcommands.

## Contributing

Contributions are welcome. Please open an [issue](https://github.com/cipher-shad0w/gogchat/issues) to report bugs or suggest features, or submit a pull request.

## License

MIT — see [LICENSE](LICENSE) for details.
