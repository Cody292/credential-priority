<div align="center">

# Credential-Priority

[中文](./README.md) | [English](./README.en.md)

</div>

Credential Priority is a CLIProxyAPI (CPA) plugin that automatically adjusts credential priority. The plugin ID, dynamic library basename, and CPA configuration key are all `credential-priority`.

## Navigation

- [Overview](#overview)
- [Workflow](#workflow)
- [Build and Installation](#build-and-installation)
- [Plugin Store Source](#plugin-store-source)
- [Configuration](#configuration)
- [Management Page and API](#management-page-and-api)
- [License](#license)

## Overview

- Reuses CPA credential, proxy, and write-back flows through `host.auth.list`, `host.auth.get`, `host.auth.get_runtime`, and `host.auth.save`.
- Generates priority changes only from fresh and ready evidence collected in the current run.
- Currently supports only Antigravity and Codex credentials; additional providers may be added later.
- Provider rules are independent: Antigravity and Codex do not share start priorities or depletion behavior.
- Status pages, diagnostics, snapshots, and logs expose only redacted credential information.

## Workflow

```mermaid
%%{init: {"flowchart": {"nodeSpacing": 24, "rankSpacing": 28}} }%%
flowchart LR
    A[Load plugin] --> B[Read config]
    B --> C[List credentials]
    C --> D[Filter providers]
    D --> E[Probe quota]
    E --> F[Build plan]
    F --> G{apply?}
    G -->|yes| H[Write result]
    G -->|no| I[Update status]
    H --> J[Show redacted result]
    I --> J
```

## Build and Installation

The plugin runs as a CGO dynamic library. CPA derives the plugin ID from the dynamic library filename, so the filename must stay `credential-priority.<ext>`.

```bash
go build -buildmode=c-shared -o credential-priority.so .
```

Place the artifact in one of the CPA plugin discovery directories:

- `plugins/<GOOS>/<GOARCH>/credential-priority.<ext>`
- `plugins/<GOOS>/<GOARCH>-<variant>/credential-priority.<ext>`
- `plugins/credential-priority.<ext>`

Extensions: `.so` on Linux and FreeBSD, `.dylib` on macOS, and `.dll` on Windows.

## Plugin Store Source

To install this plugin through the CPA plugin store, third-party sources must point to the raw JSON text of `registry.json`:

```yaml
plugins:
  enabled: true
  store-sources:
    - "https://raw.githubusercontent.com/Cody292/credential-priority/main/registry.json"
```

Do not use `https://github.com/Cody292/credential-priority/blob/main/registry.json`. That URL returns a GitHub HTML page, which CPA cannot parse as a plugin store registry. After changing `store-sources`, restart CPA or reload configuration through the management UI, then refresh the plugin store list.

## Configuration

Enable the CPA plugin system and keep plugin-owned fields under `plugins.configs.credential-priority`:

```yaml
plugins:
  enabled: true
  dir: "plugins"
  configs:
    credential-priority:
      enabled: true
      priority: 10
      auto_apply: false
      provider_scope: "all"
      selected_providers: []
      antigravity_model_group: "gemini"
      interval: 5m
      max_concurrency: 2
      min_change: 1
      top_priority_probe_count: 10
      active_group_size: 10
      active_group_jitter: 10m
      disabled_group_size: 5
      disabled_probe_interval: 30m
      priority_rules:
        enabled: false
        antigravity:
          start_priority: 100
        codex:
          start_priority: 100
          free_depleted_priority: -1
          free_depleted_disabled: true
          paid_depleted_keeps_enabled: true
```

| Field | Description |
| :--- | :--- |
| `enabled` | Per-plugin switch. Global `plugins.enabled: true` and successful dynamic library registration are also required. |
| `priority` | CPA plugin loading and execution order. Higher values run earlier. |
| `auto_apply` | Enables scheduled execution and write-back. Default: `false`. |
| `provider_scope` | `all` handles all currently supported providers; `selected` handles only `selected_providers`. |
| `selected_providers` | Supports only `antigravity` and `codex`. Empty selected scope falls back to `all`. |
| `antigravity_model_group` | Antigravity quota group: `gemini` or `claude_gpt`. |
| `interval` | Scheduled execution interval. Default: `5m`. |
| `max_concurrency` | Concurrent probe count. Default: `2`. |
| `min_change` | Priority changes below this threshold are skipped. Default: `1`. |
| `top_priority_probe_count` | Number of high-priority credentials probed immediately. Default: `10`. |
| `active_group_size` | Active credential probe group size. Default: `10`. |
| `active_group_jitter` | Active group probe jitter. Default: `10m`. |
| `disabled_group_size` | Disabled credential probe group size. Default: `5`. |
| `disabled_probe_interval` | Disabled credential re-probe interval. Default: `30m`. |
| `priority_rules.enabled` | Enables custom priority rules. When disabled, built-in sorting is used. |

### Provider-Independent Rules

Antigravity rules

- `priority_rules.antigravity.start_priority`: start priority for available credentials. Default: `100`.
- Only credentials with fresh quota evidence for the selected Antigravity model group are sorted.
- Failed quota fetches and unavailable remaining quota keep the current priority and enabled state.

Codex rules

- `priority_rules.codex.start_priority`: start priority for available credentials. Default: `100`.
- `priority_rules.codex.free_depleted_priority`: priority for depleted Free credentials. Default: `-1`.
- `priority_rules.codex.free_depleted_disabled`: disables depleted Free credentials. Default: `true`.
- `priority_rules.codex.paid_depleted_keeps_enabled`: keeps Plus, Pro, and Team credentials enabled when depleted. Default: `true`.

## Management Page and API

The plugin registers a resource page and management routes through `management.register`.

### Resource Page

- `GET /v0/resource/plugins/credential-priority/status`
  Returns an HTML dashboard with credential totals, provider counts, next probe time, recent audit summary, and redacted decisions.

### Management API

The following endpoints require the CPA management key:

- `POST /v0/management/plugins/credential-priority/run?mode=apply&provider_scope=all&antigravity_model_group=gemini`
  Manually probes, plans, and writes credential changes.
- `POST /v0/management/plugins/credential-priority/run?mode=apply&provider=antigravity&antigravity_model_group=claude_gpt`
  Handles only Antigravity credentials with the Claude/GPT model group.
- `POST /v0/management/plugins/credential-priority/run?mode=apply&provider=codex`
  Handles only Codex credentials.
- `GET /v0/management/plugins/credential-priority/diagnostics`
  Exports redacted diagnostics.
- `GET /v0/management/plugins/credential-priority/snapshot/latest`
  Returns the latest redacted decision snapshot.

## License

This project is licensed under the MIT License. See [LICENSE](./LICENSE).
