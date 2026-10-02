# credential-priority vX.Y.Z

## 更新内容

### 中文

- 用简洁、面向使用者的条目说明「改了什么、对我有什么影响」。
- 避免内部函数名、结构体字段名、源码路径等技术细节。
- 配置相关变更写清：字段名、默认行为、是否需要改配置。
- 行为变更写清：谁受益、谁不受影响（例如某提供商 / 免费或付费）。
- 不写「升级说明」小节；安装/替换动态库并重启宿主的通用步骤由商店或 README 覆盖即可。

### English

- Write short, user-facing bullets: what changed and how it affects day-to-day use.
- Avoid internal symbol names, struct fields, or source paths.
- For config changes: name the setting, default behavior, and whether users must edit config.
- For behavior changes: who is affected (provider / free vs paid) and who is not.
- Do not add a separate “Upgrade notes” section; replace the plugin binary and restart the host as usual.
