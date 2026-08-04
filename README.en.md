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
- Currently supports Antigravity, Codex, and xAI credentials; additional providers may be added later.
- Provider rules are independent: Antigravity, Codex, and xAI do not share start priorities or depletion behavior.
- Status pages, diagnostics, snapshots, and logs expose only redacted credential information.
- **Automatic priority and rules** are edited via CPA **Plugin Manager visual ConfigFields** (recommended), or host `config.yaml` / `plugins.configs.credential-priority`.
- **Plugin page** supports Management Key verification, overview (read-only effective config), run history (last 5), help, and manual apply (management routes). **It does not save config on the plugin page.**

## Workflow

```text
Load plugin
  -> Read plugins.configs.credential-priority config
  -> Fetch CPA credential list through host.auth.list
  -> Filter currently supported providers by provider_scope (all or antigravity|codex|xai)
       - Antigravity: probe remaining quota for the selected model group
       - Codex: probe availability by account plan and quota state
        - xAI: FetchPlan (settings / billing / JWT) classifies free/paid; auto no longer multi-model chat probes
             Business quota via usage.handle; 2×429 within 30 minutes → free soft-disable; 401 AuthInvalid → hard-disable
             Free does not join priority sorting by default (`free_participates_priority` default false)
  -> Build a sorting plan only from fresh and ready evidence in this run
  -> Decide whether to write back by run mode
       - apply: write priority and enabled state through host.auth.save
       - preview: update status, diagnostics, snapshot, and logs only
  -> Show redacted statistics, audit summary, and sorting result on the management page
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
      provider_scope: "all"   # or "antigravity|codex|xai"
      antigravity_model_group: "gemini"
      priority_rules:
        enabled: false
        antigravity:
          start_priority: 100
        codex:
          start_priority: 100
          free_depleted_priority: -1
          free_depleted_disabled: true
          paid_depleted_disabled: false
        xai:
          start_priority: 100
          free_depleted_priority: -1
          free_depleted_disabled: false
          weekly_depleted_priority: -1
          monthly_and_weekly_depleted_priority: -1
          monthly_and_weekly_depleted_disabled: true
```

| Field | Description |
| :--- | :--- |
| `enabled` | Per-plugin switch. Global `plugins.enabled: true` and successful dynamic library registration are also required. Independent of `priority_rules.enabled`. |
| `priority` | CPA plugin loading and execution order. Higher values run earlier. |
| `auto_apply` | Enables scheduled execution and write-back. Default: `false`. |
| `provider_scope` | `all` handles every currently supported provider; or list one or more providers separated by `\|`, e.g. `antigravity\|codex\|xai`. Legacy `selected` + `selected_providers` remains supported. |
| `antigravity_model_group` | Antigravity quota group: `gemini` or `claude_gpt`. |
| `priority_rules.enabled` | Enables custom priority rules. When disabled, built-in sorting is used. Independent of top-level `enabled`. |
| `interval` | Auto sort / probe batch step (default 15m). Disabled credentials also batch with this interval (no fixed 1h freeze). |
| `immediate_probe_limit` / `active_group_size` | Immediate probe count and active batch size; disabled batches share `active_group_size` with active. |

> **v1.1.4 config notes** (user-facing): Flat `priority_rules.*` keys are supported. Turn on `priority_rules.enabled` so each provider’s `start_priority` is applied. Disabled/depleted accounts no longer wait a fixed 1 hour—pacing follows `interval` and batch settings. Within ~24h of a quota reset, accounts with remaining quota are preferred (Antigravity/Codex free included; xAI free excluded). `cache_path` / `cache_ttl` are not user settings.

### Provider-Independent Rules

Antigravity rules

- `priority_rules.antigravity.start_priority`: start priority for available credentials. Default: `100`.
- Only credentials with fresh quota evidence for the selected Antigravity model group are sorted.
- Failed quota fetches and unavailable remaining quota keep the current priority and enabled state.

Codex rules

- `priority_rules.codex.start_priority`: start priority for available credentials. Default: `100`.
- `priority_rules.codex.free_depleted_priority`: priority for depleted Free credentials. Default: `-1`.
- `priority_rules.codex.free_depleted_disabled`: disables depleted Free credentials. Default: `true`.
- `priority_rules.codex.paid_depleted_disabled`: disable Plus/Pro/Team when depleted; `true`=disable, `false`=keep enabled. Default: `false`. Legacy `paid_depleted_keeps_enabled` is still accepted (inverted).

xAI rules (v1.1.4)

**Plan classification (FetchPlan)**

- Classifies the plan as `free` or `paid` via settings / billing / JWT tier — **auto no longer** multi-model chat probes for quota.
- Unfetchable results (network failure, 404, missing plan fields, etc.) default to **`free`**; only explicit paid product/tier signals mark **`paid`**.
- **Free sorting is off by default**: `free_participates_priority` defaults to `false` — Free does not join positive priority promotion / free-first / uniqueness reordering.
- Only with explicit `priority_rules.xai.free_participates_priority: true` do eligible free credentials sort above paid (consume free first).

**Free quota and 24h anchor (independent of the sorting switch)**

- Business quota is accumulated from host `usage.handle`, not from chat probes.
- **2×429 within 30 minutes** → `priority=-1` **soft-disable** (lower priority only); independent of whether Free joins sorting; still applies by default.
- `free_depleted_disabled` defaults to `false` (soft-disable, no hard `disabled`); set `true` only for hard disable.
- Cooldown anchor: `first_success_at + 24h` (or the depleting failure time + 24h if no success yet); after that, recovery can resume.

**401 AuthInvalid (hard disable)**

- Still 401 / credential-invalid text after OAuth force refresh → `priority=-1` and `disabled=true` hard disable.
- Requires user **re-login** to recover; does not count toward free quota failure streak.

**Config fields**

- `priority_rules.xai.start_priority`: start priority for available credentials. Default: `100`.
- `priority_rules.xai.free_depleted_priority`: priority when free usage is depleted (soft-disable). Default: `-1`.
- `priority_rules.xai.free_depleted_disabled`: hard-disables free usage depleted credentials. Default: `false` (soft-disable: lower priority only).
- `priority_rules.xai.free_participates_priority`: whether Free joins positive priority sorting / free-first. Default: `false`; set `true` to opt in. When false, 429 depletion, cooldown, and 401 are unchanged.
- `priority_rules.xai.weekly_depleted_priority`: priority when only weekly limit is depleted. Default: `-1` (not disabled).
- `priority_rules.xai.monthly_and_weekly_depleted_priority`: priority when monthly and weekly are depleted. Default: `-1`.
- `priority_rules.xai.monthly_and_weekly_depleted_disabled`: disables when monthly and weekly are depleted. Default: `true`.

## Management Page and API

The plugin registers **resources** (static shell) and **routes** (dynamic APIs) via `management.register`.

### Product boundary

| Capability | Entry | Notes |
| :--- | :--- | :--- |
| Automatic priority and rules | CPA Plugin Manager visual fields (recommended) or `config.yaml` | `auto_apply`, `provider_scope` (all or a\|b\|c), `interval`, `priority_rules.*`, etc. |
| Resource page | `/v0/resource/plugins/credential-priority/status` | Static HTML: key verify + overview / run history / help + manual sort |
| Manual apply | `/v0/management/plugins/credential-priority/run` | Requires Management Key |
| Read-only config | Host `GET /v0/management/plugins/credential-priority/config` | Display only; no plugin-page PATCH |

### Resource page (static)

- `GET /v0/resource/plugins/credential-priority/status`
  Returns a static HTML shell. The browser uses the Management Key for read-only data, run history, and management-path manual runs. **No in-page config save controls.**

### Management API (dynamic, key required)

- `POST /v0/management/plugins/credential-priority/run?mode=apply&provider_scope=all&antigravity_model_group=gemini`
  Manual probe, plan, and write-back of credential priorities.
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
