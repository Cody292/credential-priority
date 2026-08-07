# credential-priority v1.1.6

## 更新内容

### 中文

- 用简洁、面向使用者的条目说明「改了什么、对我有什么影响」。
- 避免内部函数名、结构体字段名、源码路径等技术细节。
- 配置相关变更写清：字段名、默认行为、是否需要改配置。
- 行为变更写清：谁受益、谁不受影响（例如某提供商 / 免费或付费）。
- 不写「升级说明」小节；安装/替换动态库并重启宿主的通用步骤由商店或 README 覆盖即可。

**本次更新 (v1.1.6) 包含：**
- 5 小时短窗口绝不进入 999 临近重置提权，避免短周期频繁抢占高优先级。
- 适用于 Antigravity 与 Codex 付费及免费路径的周或月长窗口，在重置前 24 小时内可正常参与 999 临近重置提权。
- 各提供商优先级规则保持相互独立，互不影响。
- OAuth 付费 xAI 凭据现可使用官方周度长窗口重置时间，在 24 小时临近重置窗口内参与提权，而 xAI 免费账户依然不参与 999 临近重置提权。
- 无需修改任何配置，更新后自动生效。

### English

- Write short, user-facing bullets: what changed and how it affects day-to-day use.
- Avoid internal symbol names, struct fields, or source paths.
- For config changes: name the setting, default behavior, and whether users must edit config.
- For behavior changes: who is affected (provider / free vs paid) and who is not.
- Do not add a separate “Upgrade notes” section; replace the plugin binary and restart the host as usual.

**This update (v1.1.6) includes:**
- 5-hour short windows never enter the 999 near-reset boost, preventing short cycles from dominating high priority.
- Weekly and monthly long windows for applicable Antigravity and Codex paid or free tiers may participate in the 999 boost within 24 hours of reset.
- Provider priority rules remain independent and do not affect one another.
- OAuth paid xAI credentials can now use the official weekly long-window reset within the 24-hour near-reset boost window, while xAI Free still never receives the 999 near-reset boost.
- No configuration change is required; the update takes effect automatically.
