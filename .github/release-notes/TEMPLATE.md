# credential-priority v1.1.5

## 更新内容

### 中文

- 用简洁、面向使用者的条目说明「改了什么、对我有什么影响」。
- 避免内部函数名、结构体字段名、源码路径等技术细节。
- 配置相关变更写清：字段名、默认行为、是否需要改配置。
- 行为变更写清：谁受益、谁不受影响（例如某提供商 / 免费或付费）。
- 不写「升级说明」小节；安装/替换动态库并重启宿主的通用步骤由商店或 README 覆盖即可。

**本次更新 (v1.1.5) 包含：**
- 生效基于 24 小时内实际额度刷新时间的调用偏好，优先调度即将重置的凭据。
- 在同一模型提供商内，重置时间更近的凭据将获得更高的调度优先级。
- 已启用且大于 0 的优先级数值保持唯一，并严格限制上限为 999。
- 各提供商的优先级规则相互独立计算，互不影响。
- xAI 免费账户排除规则维持现状。
- 无需修改任何配置，更新后新调度策略将自动生效。

### English

- Write short, user-facing bullets: what changed and how it affects day-to-day use.
- Avoid internal symbol names, struct fields, or source paths.
- For config changes: name the setting, default behavior, and whether users must edit config.
- For behavior changes: who is affected (provider / free vs paid) and who is not.
- Do not add a separate “Upgrade notes” section; replace the plugin binary and restart the host as usual.

**This update (v1.1.5) includes:**
- Effective quota-reset-time-based 24-hour near-reset preference, prioritizing credentials that are expiring soon.
- Nearer reset wins within the same provider.
- Positive enabled priorities remain unique and are capped at 999.
- Provider priority rules remain independent.
- The xAI free account exclusion rule remains unchanged.
- No configuration change is required; the update takes effect automatically.
