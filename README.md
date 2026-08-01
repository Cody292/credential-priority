<div align="center">

# Credential-Priority

[中文](./README.md) | [English](./README.en.md)

</div>

CLIProxyAPI (CPA) 凭证优先级自动调整插件。插件 ID、动态库基础名与 CPA 配置键均为 `credential-priority`。

<div align="center">
  <img src="./picture/密钥验证页.png" alt="密钥验证页" width="32%" />
  <img src="./picture/概览页.png" alt="概览页" width="32%" />
  <img src="./picture/配置页.png" alt="配置页" width="32%" />
</div>

## 导航

- [功能概览](#功能概览)
- [工作流程](#工作流程)
- [构建与安装](#构建与安装)
- [插件商店来源](#插件商店来源)
- [配置说明](#配置说明)
- [管理页面与接口](#管理页面与接口)
- [许可证](#许可证)

## 功能概览

- 通过宿主回调 `host.auth.list`、`host.auth.get`、`host.auth.get_runtime`、`host.auth.save` 复用 CPA 的凭证、代理和写入链路。
- 只对本轮最新且可用的探测证据生成排序变更，避免用过期缓存调整凭证状态。
- 当前支持 Antigravity、Codex 与 xAI 凭证；后续可扩展其他提供商配置。
- 不同提供商的排序规则彼此独立，Antigravity、Codex 与 xAI 不共享起始优先级或额度耗尽策略。
- 状态页、诊断、快照与日志只输出脱敏后的凭证信息。
- **自动优先级与规则**通过 CPA **插件管理可视化配置字段**（`ConfigFields`）编辑；也可直接改 `config.yaml` / `plugins.configs.credential-priority`。
- **插件页**支持 Management Key 验证、概览（只读生效配置）、执行记录（近 5 次）、帮助，以及手动 apply（management 路径）；**不在插件页内保存配置**。

## 工作流程

```text
加载插件
  -> 读取 plugins.configs.credential-priority 配置
  -> 通过 host.auth.list 获取 CPA 凭证列表
  -> 按 provider_scope（all 或 antigravity|codex|xai）筛选当前支持的提供商
       - Antigravity：按所选模型组探测剩余额度
       - Codex：按账号计划与额度状态探测可用性
       - xAI：FetchPlan（settings / billing / JWT）识别 free/paid；auto 不再 chat 多模型探额度
            业务额度由 usage.handle 累计；连续 3 次额度类 429 → free 软禁用；401 AuthInvalid → 硬禁用
  -> 只使用本轮最新且可用的探测证据生成排序计划
  -> 根据运行模式决定是否写回
       - apply：通过 host.auth.save 写回优先级与启用状态
       - preview：仅更新状态、诊断、快照与日志
  -> 在管理页面展示脱敏后的统计、审计摘要与排序结果
```

## 构建与安装

插件以 CGO 动态库形式运行，宿主会从动态库文件名去掉扩展名得到插件 ID，因此文件名必须保持为 `credential-priority.<ext>`。

```bash
go build -buildmode=c-shared -o credential-priority.so .
```

把产物放入 CPA 插件发现目录之一：

- `plugins/<GOOS>/<GOARCH>/credential-priority.<ext>`
- `plugins/<GOOS>/<GOARCH>-<variant>/credential-priority.<ext>`
- `plugins/credential-priority.<ext>`

扩展名：Linux/FreeBSD 为 `.so`，macOS 为 `.dylib`，Windows 为 `.dll`。

## 插件商店来源

如需通过 CPA 插件商店安装本插件，第三方来源必须指向 `registry.json` 的原始 JSON 文本：

```yaml
plugins:
  enabled: true
  store-sources:
    - "https://raw.githubusercontent.com/Cody292/credential-priority/main/registry.json"
```

不要使用 `https://github.com/Cody292/credential-priority/blob/main/registry.json`。该地址返回 GitHub HTML 页面，CPA 无法按插件商店 registry 解析。修改 `store-sources` 后，重启 CPA 或通过管理端重新加载配置，再刷新插件商店列表。

## 配置说明

在 CPA `config.yaml` 中启用插件系统，并在 `plugins.configs.credential-priority` 下保留插件自有配置：

```yaml
plugins:
  enabled: true
  dir: "plugins"
  configs:
    credential-priority:
      enabled: true
      priority: 10
      auto_apply: false
      provider_scope: "all"   # 或 "antigravity|codex|xai"
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

字段说明：

| 字段 | 说明 |
| :--- | :--- |
| `enabled` | 单插件开关；还需要全局 `plugins.enabled: true` 且动态库注册成功。 |
| `priority` | CPA 宿主加载与执行插件的顺序，数值越大优先级越高。 |
| `auto_apply` | 是否由定时器自动执行并写回排序结果，默认 `false`。 |
| `provider_scope` | `all` 处理全部当前支持的提供商；也可填单个或多个提供商，多个用 `\|` 分隔，例如 `antigravity\|codex\|xai`。兼容旧配置 `selected` + `selected_providers`。 |
| `antigravity_model_group` | Antigravity 配额模型组，支持 `gemini` 与 `claude_gpt`。 |
| `priority_rules.enabled` | 是否启用自定义排序规则；关闭时使用内置排序策略。 |

### 提供商独立排序规则

Antigravity 规则

- `priority_rules.antigravity.start_priority`：可用凭证的起始优先级，默认 `100`。
- 只排序本轮成功获取到所选模型组配额的 Antigravity 凭证。
- 配额获取失败或剩余额度不可用时保留当前优先级与启用状态。

Codex 规则

- `priority_rules.codex.start_priority`：可用凭证的起始优先级，默认 `100`。
- `priority_rules.codex.free_depleted_priority`：Free 凭证额度为 0 时写入的优先级，默认 `-1`。
- `priority_rules.codex.free_depleted_disabled`：Free 凭证额度为 0 时是否禁用，默认 `true`。
- `priority_rules.codex.paid_depleted_disabled`：Plus、Pro、Team 额度耗尽时是否禁用；`true`=禁用，`false`=保持启用，默认 `false`。兼容旧键 `paid_depleted_keeps_enabled`（语义取反）。

xAI 规则（v1.1.1）

**套餐识别（FetchPlan）**

- 通过 settings / billing / JWT tier 轻量分类套餐为 `free` 或 `paid`，**auto 不再**对 chat 多模型探额度。
- 无法识别（网络失败、404、无套餐字段等）时**默认 `free`**；仅在明确付费 product/tier 时标 `paid`。
- 排序时 **free 高优**：可参与排序的 free 凭证优先于 paid（优先消耗 free）。

**free 额度与 24h 锚点**

- 业务额度由宿主 `usage.handle` 累计，不依赖 chat 探测。
- 连续 **3 次**额度类 429（如 `free-usage-exhausted` / rolling 24h tokens 等）→ `priority=-1` **软禁用**（仅降优先级）。
- `free_depleted_disabled` 默认 `false`（软禁用，不硬 `disabled`）；显式设为 `true` 时才硬禁用。
- 冷却锚点：`first_success_at + 24h`（若无成功记录则用第三次失败时刻 + 24h）；到期后可恢复高优。

**401 AuthInvalid（硬禁用）**

- OAuth force 刷新后仍 401 / 凭证失效文案 → `priority=-1` 且 `disabled=true` 硬禁用。
- 硬禁用后需用户**重新登录**恢复；不计入 free 额度失败次数。

**配置字段**

- `priority_rules.xai.start_priority`：可用凭证的起始优先级，默认 `100`。
- `priority_rules.xai.free_depleted_priority`：免费额度耗尽（软禁用）时写入的优先级，默认 `-1`。
- `priority_rules.xai.free_depleted_disabled`：免费额度耗尽时是否硬禁用，默认 `false`（软禁用：仅降 priority）。
- `priority_rules.xai.weekly_depleted_priority`：仅周限额耗尽时写入的优先级，默认 `-1`（不禁用）。
- `priority_rules.xai.monthly_and_weekly_depleted_priority`：周与月均耗尽时写入的优先级，默认 `-1`。
- `priority_rules.xai.monthly_and_weekly_depleted_disabled`：周与月均耗尽时是否禁用，默认 `true`。

## 管理页面与接口

插件通过 `management.register` 分别注册 **resources**（静态壳）与 **routes**（动态业务）。

### 产品边界

| 能力 | 入口 | 说明 |
| :--- | :--- | :--- |
| 自动优先级与规则配置 | CPA 插件管理可视化字段（推荐）或 `config.yaml` | `auto_apply`、`provider_scope`（all 或 a\|b\|c）、`interval`、`priority_rules.*` 等 |
| 插件资源页 | `/v0/resource/plugins/credential-priority/status` | 静态 HTML：Key 验证 + 概览/执行记录/帮助 + 手动排序 |
| 手动 apply | `/v0/management/plugins/credential-priority/run` | 需要 Management Key |
| 只读配置 | 宿主 `GET /v0/management/plugins/credential-priority/config` | 插件页只读展示，不在插件页 PATCH |

### 资源页面（静态）

- `GET /v0/resource/plugins/credential-priority/status`
  返回静态 HTML 壳。浏览器侧用 Management Key 拉取只读数据与执行记录，并调用 management 路径手动排序。**无插件页内配置保存控件。**

### 管理 API（动态，需密钥）

- `POST /v0/management/plugins/credential-priority/run?mode=apply&provider_scope=all&antigravity_model_group=gemini`
  手动触发探测、规划并写回凭证优先级。
- `POST /v0/management/plugins/credential-priority/run?mode=apply&provider=antigravity&antigravity_model_group=claude_gpt`
  只处理 Antigravity 凭证并使用 Claude/GPT 模型组。
- `POST /v0/management/plugins/credential-priority/run?mode=apply&provider=codex`
  只处理 Codex 凭证。
- `GET /v0/management/plugins/credential-priority/diagnostics`
  导出脱敏诊断信息。
- `GET /v0/management/plugins/credential-priority/snapshot/latest`
  获取最近一次运行的脱敏决策快照。

## 许可证

本项目使用 MIT License，详见 [LICENSE](./LICENSE)。
