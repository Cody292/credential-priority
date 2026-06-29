# credential-priority

CLIProxyAPI（CPA）凭证优先级自动调整插件。插件 ID、动态库 basename 与配置键均为 `credential-priority`。

## 功能特性

- 通过宿主回调 `host.auth.list`、`host.http.do`、`host.auth.save` 复用 CPA 的凭证、代理和写入链路。
- 探测凭证 fresh 状态，并按可用性规划优先级与禁用状态。
- 支持 `dry-run` 预览和 `apply` 写入，首次部署建议先 dry-run。
- 状态页、诊断、快照与日志只输出脱敏后的凭证信息。

## 1. 构建与安装

插件以 CGO 动态库形式运行，宿主会从动态库文件名去掉扩展名得到插件 ID，因此文件名必须保持为 `credential-priority.<ext>`。

```bash
go build -buildmode=c-shared -o credential-priority.so .
```

把产物放入 CPA 插件发现目录之一：

- `plugins/<GOOS>/<GOARCH>/credential-priority.<ext>`
- `plugins/<GOOS>/<GOARCH>-<variant>/credential-priority.<ext>`
- `plugins/credential-priority.<ext>`

扩展名：Linux/FreeBSD 为 `.so`，macOS 为 `.dylib`，Windows 为 `.dll`。

## 2. 配置说明

在 CPA `config.yaml` 中启用插件系统，并在 `plugins.configs.credential-priority` 下保留插件自有配置：

```yaml
plugins:
  enabled: true
  dir: "plugins"
  configs:
    credential-priority:
      enabled: true
      priority: 10              # 插件加载与执行顺序，数值越大优先级越高。
      auto_apply: false         # 首次部署建议保持 false，确认 dry-run 后再改 true。
      interval: 10m
      max_concurrency: 5
      min_change: 0.05
      top_priority_probe_count: 10
      active_group_size: 10
      active_group_jitter: 10m
      disabled_group_size: 5
      disabled_probe_interval: 30m
      cache_ttl: 24h
      cache_path: "credential-priority/refresh-cache.json"
      provider_overrides:
        codex:
          enabled: true
          auto_apply: true
          interval: 5m
          max_concurrency: 2
```

CPA 只解析通用字段 `enabled` 与 `priority`，其他字段会原样传给插件并由插件校验。插件启用条件为：全局 `plugins.enabled: true`、单插件 `enabled: true`、动态库已注册成功。

## 3. 插件资源与管理路由

插件通过 `management.register` 注册资源页面和管理路由。

### 资源页面

- `GET /v0/resource/plugins/credential-priority/status`
  返回 HTML 看板，展示凭证总数、fresh/unknown/probe_failed 计数、下一次探测时间和最近审计摘要。

### 管理 API（需要宿主管理密钥）

- `POST /v0/management/plugins/credential-priority/run?mode=dry-run`
  手动触发探测与规划，不写入凭证。
- `POST /v0/management/plugins/credential-priority/run?mode=apply`
  手动触发探测、规划并写入凭证。
- `GET /v0/management/plugins/credential-priority/diagnostics`
  导出脱敏诊断信息。
- `GET /v0/management/plugins/credential-priority/snapshot/latest`
  获取最近一次运行的脱敏决策快照。

## 4. 本地开发与验证

1. 将编译好的动态库放入 CPA 插件发现目录。
2. 开启 `config.yaml` 对应配置并启动 CLIProxyAPI 宿主。
3. 请求 `GET /v0/management/plugins`，确认 `registered` 与 `effective_enabled` 均为 `true`。
4. 浏览器访问 `/v0/resource/plugins/credential-priority/status` 确认状态看板能正常加载。
5. 调用 `run?mode=dry-run`，确认规划结果符合预期。
6. 确认无误后再启用 `auto_apply` 或调用 `run?mode=apply`。

## 5. 更新与安全约束

- 修改动态库后，通过宿主管理接口卸载/重载插件；平台不支持热更新时重启 CPA。
- 插件卸载或宿主关闭时，宿主会调用 `plugin.shutdown`，插件会停止定时器与后台任务。
- 不要在日志、状态页、诊断或快照中输出密钥、Token、Authorization 头或原始凭证 JSON。
- 插件自己的 HTTP 请求优先走 `host.http.*`，避免绕过宿主代理、日志和传输策略。
