# btw

`btw` is a fast, IntelliJ-inspired "Search Everywhere" tool for the terminal. It provides a cohesive interface to find files, symbols, classes, and text across your project with instantaneous fuzzy matching and asynchronous execution.

## Features

- **Search Everywhere**: One interface for files, symbols, classes, and actions.
- **IntelliJ-style Matcher**: Fuzzy, CamelCase (`CCN` -> `CamelCaseName`), and path fragments.
- **Async Grep**: Non-blocking content search with streaming results.
- **History-aware**: Results are ranked based on recency and frequency.
- **Project Detection**: Automatic root detection; respects `.gitignore` and `.btw.yaml`.
- **Developer First**: Adaptive themes, `:N` line-jumping, and clipboard integration.

## Installation

### From Source
Requires Go 1.25+ and `task` (optional).

```bash
git clone https://github.com/yampug/btw.git
cd btw
task install
```

## Usage

```bash
# Launch in the current directory
btw

# Pre-fill a search query
btw main.go

# Start on a specific tab
btw -t symbols MyFunction

# Filter by extension
btw -f go,rs Matcher
```

### Navigation & Shortcuts

| Key | Action |
|---|---|
| `↑` / `↓` | Select result |
| `Tab` / `S-Tab` | Cycle tabs |
| `1` - `6` | Jump to tab |
| `Enter` | Open in `$EDITOR` |
| `Ctrl+P` | Toggle Project / All Places |
| `Ctrl+F` | Toggle Extension Filters |
| `Ctrl+R` | Refresh Index |
| `?` | Show Help Overlay |

## Configuration

`btw` looks for a configuration file at `~/.config/btw/config.yaml`.

```yaml
theme: auto        # dark | light | auto
editor: nvim       # override $EDITOR
max_results: 200
show_hidden: false
default_scope: project
```

## Testing

`btw` uses both in-process integration tests and actual SSH-based tests.

### In-Process Tests (Fast)
To run tests for the remote agent server without SSH dependencies:
```bash
go test ./internal/remote/... -v
```

### SSH Integration Tests (Real Transport)
To verify it works over a real SSH connection to `localhost`:
```bash
# Requires SSH access to localhost without interactive password prompts
./test/remote_ssh_test.sh
```

## License

MIT
