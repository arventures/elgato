# elgato

A small, fast command-line tool to control [Elgato Key Lights](https://www.elgato.com/us/en/p/key-light) from your Mac (or Linux) terminal.

```console
$ elgato on
$ elgato brightness 70
$ elgato temp 4500
$ elgato off
```

Lights are discovered automatically on your network — no IP addresses to memorize.

## Install

With [Homebrew](https://brew.sh):

```console
brew install arventures/elgato/elgato
```

That's shorthand for tapping `arventures/homebrew-elgato` and installing the `elgato` cask (macOS). To update later: `brew upgrade elgato`.

Or build from source (Go 1.26+):

```console
go install github.com/arventures/elgato@latest
```

## Usage

| Command | What it does |
| --- | --- |
| `elgato on` | Turn lights on |
| `elgato off` | Turn lights off |
| `elgato toggle` | Toggle power |
| `elgato brightness 50` | Set brightness to 50% |
| `elgato brightness +10` | Raise brightness by 10% (also `-10`) |
| `elgato temp 4500` | Set color temperature to 4500K |
| `elgato status` | Show current state |
| `elgato list` | Discover lights and refresh the cache |

Global flags (work on any command):

| Flag | Purpose |
| --- | --- |
| `--host 10.0.0.11` | Target a light directly by IP, skipping discovery |
| `-l, --light <name>` | Act on a single light (by name or serial) |
| `--refresh` | Force mDNS discovery instead of using cached addresses |
| `--json` | Machine-readable output |
| `--timeout 2s` | How long to browse for lights |

By default a command acts on **all** lights it finds. Use `--light` to narrow to one.

Shell completion is available via `elgato completion <bash|zsh|fish>` (Homebrew installs it automatically).

## How it finds your lights

1. **Cache first.** Known lights are remembered in `~/.config/elgato/config.yaml`, keyed by their **serial number** (stable across reboots and DHCP changes). Cached addresses are tried first, so everyday commands are instant.
2. **mDNS discovery.** If nothing is cached or reachable, the tool browses the network for the `_elg._tcp` service Elgato lights advertise, then refreshes the cache. Run `elgato list` (or add `--refresh`) any time to re-scan.
3. **Direct.** `--host <ip>` bypasses all of the above.

### Config file

`~/.config/elgato/config.yaml` is created automatically. You can edit it to give lights friendlier names:

```yaml
lights:
  key-left:
    serial: CW30K1A07261   # stable identity
    host: 10.0.0.11        # last known address (auto-refreshed)
    port: 9123
discovery_timeout: 2s
```

Then: `elgato --light key-left on`.

## About color temperature

Elgato's API takes temperature as a "mired" value in the range `143`–`344`. This tool lets you use Kelvin instead and converts for you (`mireds = 1,000,000 ÷ Kelvin`). The usable range is roughly **2900K (warm)** to **7000K (cool)**; values outside are clamped.

## Releasing (maintainers)

Releases are automated with [GoReleaser](https://goreleaser.com) via GitHub Actions. One-time setup:

1. Create a second GitHub repo named **`homebrew-elgato`** (this is the tap).
2. Create a Personal Access Token with `contents:write` on that tap repo and add it to **this** repo as an Actions secret named **`TAP_GITHUB_TOKEN`**.

Then cut a release by pushing a tag:

```console
git tag v0.1.0
git push origin v0.1.0
```

The workflow builds macOS/Linux binaries (Intel + Apple Silicon), publishes a GitHub Release, and pushes an updated Homebrew **cask** to the tap (Homebrew now prefers casks for pre-built binaries; casks are macOS-only). Validate config locally with `goreleaser check` and dry-run with `goreleaser release --snapshot --clean`.

## Development

```console
go build ./...      # compile
go test ./...       # run unit tests (pure logic; no device needed)
go vet ./...
```

The pure logic (Kelvin↔mireds conversion, brightness parsing, request shape) lives in `internal/elgato` and is unit-tested without hardware. Network code (HTTP client, mDNS) is isolated in `internal/elgato` and `internal/discovery`.

## Disclaimer

This is an unofficial, community project and is not affiliated with or endorsed by Corsair. "Elgato" and "Key Light" are trademarks of their respective owners.

## License

[MIT](./LICENSE)
