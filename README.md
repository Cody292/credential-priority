# credential-priority

CPA 凭证优先级自动调整插件。插件 ID、目录名、动态库 basename、CPA 配置键均为 `credential-priority`。

## 1. 导入与启用

构建当前平台动态库后，将产物放入 CPA 的插件发现目录。Linux 示例：

```bash
go build -buildmode=c-shared -o credential-priority.so .
cp credential-priority.so /path/to/cpa/plugins/linux/amd64/credential-priority.so
```

CPA 会从动态库文件名去扩展名得到插件 ID，因此文件名必须保持为 `credential-priority.<ext>`。

## 2. CPA 配置片段

在 CPA 配置文件中启用插件系统，并在 `plugins.configs.credential-priority` 下写入插件自有配置：

```yaml
plugins:
  enabled: true
  dir: "plugins"
  configs:
    credential-priority:
      enabled: true
      auto_apply: true # 首次部署建议先改为 false，通过资源页或 dry-run 确认后再启用。
      interval: 10m
      max_concurrency: 5
      min_change: 0.05
      top_priority_probe_count: 10
      active_group_size: 10
      active_group_jitter: 10m
      disabled_group_size: 5
      disabled_probe_interval: 30m
      cache_ttl: 24h
      cache_path: credential-priority/refresh-cache.json # 默认值；可改为绝对路径。
      provider_overrides:
        codex:
          enabled: true
          auto_apply: true
          interval: 5m
          max_concurrency: 2
```

## 3. 动态链接库命名与多平台扩展名

构建的动态库基准名称必须为 `credential-priority`。在不同的操作系统平台上，其文件扩展名如下：
* **Linux**: `credential-priority.so`
* **macOS**: `credential-priority.dylib`
* **Windows**: `credential-priority.dll`

## 4. 插件资源页面与管理路由

插件通过 `management.register` 同时注册 `resources` 与 `routes`。状态页作为插件资源页面访问：

```text
GET /v0/resource/plugins/credential-priority/status
```

插件还提供以下管理处理路径，宿主会按 Management API 规则路由到插件：

* `GET /v0/resource/plugins/credential-priority/status`：返回 HTML 状态页，包含总凭证数、fresh/unknown/probe_failed 计数、下一次探测时间、最近审计摘要等。
* `POST /v0/management/plugins/credential-priority/run?mode=dry-run`：手动触发探测和优先级规划计算，但不写入变更。
* `POST /v0/management/plugins/credential-priority/run?mode=apply`：手动触发探测、优先级规划并写入变更。
* `GET /v0/management/plugins/credential-priority/diagnostics`：导出排查诊断信息（脱敏处理，无敏感凭证内容）。
* `GET /v0/management/plugins/credential-priority/snapshot/latest`：获取最近一次运行生成的审计与决策快照（已脱敏）。

生产运行时会通过 CPA 宿主回调 `host.auth.list`、`host.http.do` 和管理 HTTP patch 接口完成凭证列举、fresh probe 与写入，不直接复制宿主凭证，也不绕过宿主代理策略。

## 5. 默认自动写入风险与 Dry-run 模式

插件支持的 `auto_apply` 参数默认为 `true`。在此模式下，系统会在取得 fresh 状态后自动更新凭证的优先级与禁用状态。
**风险提示**：在首次部署或大型凭证列表变更时，建议在配置文件中将 `auto_apply` 设为 `false`，先通过 `POST /run?mode=dry-run` 或 `/snapshot/latest` 查看规划效果，确认无误后再启用 `auto_apply`。

## 6. 卸载与热重载步骤

修改动态库或更新插件配置后，为确保旧实例安全退出且不发生 goroutine 泄漏，请按以下步骤操作：
1. **热重载**：使用 CPA 的插件管理接口向其发送热重载命令，CPA 宿主会触发旧插件实例的 `plugin.shutdown`，回收所有定时器与并发任务，然后载入新动态库并调用其 `cliproxy_plugin_init`。
2. **重启宿主**：在 CPA 宿主不支持热重载的平台上，建议直接重启整个 CPA 进程。

## 7. 敏感日志与脱敏约束

* 日志、状态页、诊断接口以及测试 evidence 严禁记录任何真实密钥、Token、Authorization 头部、管理 Key 或敏感请求体 Payload。
* 状态查询、诊断和快照输出中，涉及凭证敏感信息的字段均经过 SHA-256 混淆或脱敏（Masking）处理，确保数据隐私安全。
