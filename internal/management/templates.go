package management

// StatusHTML 是 resource /status 的静态 HTML 壳。
// 浏览器侧仅：Management Key 验证、只读拉取配置/概览、调用 management 路径手动 dry-run/apply。
// 禁止配置表单与保存；自动规则仅通过宿主 config.yaml / plugins.configs 修改。
const StatusHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <title>凭证优先级管理</title>
    <style>
        :root { color-scheme: light; --text:#111827; --muted:#6b7280; --line:#e5e7eb; --panel:#fff; --soft:#f8fafc; --blue:#2563eb; --danger:#b91c1c; --green:#16a34a; }
        * { box-sizing:border-box; }
        body { margin:0; padding:24px; background:#f6f7fb; color:var(--text); font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,"Helvetica Neue",Arial,sans-serif; }
        .container { width:100%; max-width:1180px; margin:0 auto; background:var(--panel); border:1px solid rgba(17,24,39,.06); border-radius:18px; padding:32px; box-shadow:0 18px 48px rgba(15,23,42,.07); }
        .topbar { display:flex; align-items:flex-start; justify-content:space-between; gap:16px; margin-bottom:24px; }
        .topbar-actions { display:flex; align-items:flex-start; justify-content:flex-end; gap:12px; flex:1; }
        h1 { margin:0; font-size:28px; letter-spacing:-.03em; display:flex; align-items:center; gap:10px; flex-wrap:wrap; }
        .version-badge { display:inline-flex; align-items:center; min-height:24px; border-radius:999px; padding:3px 9px; background:#eff6ff; color:#1d4ed8; font-size:12px; font-weight:750; }
        h2 { margin:0 0 16px; font-size:18px; }
        p { margin:0; color:var(--muted); line-height:1.65; }
        label { display:block; margin:0 0 8px; font-size:13px; font-weight:650; color:#374151; }
        input, select { width:100%; min-height:44px; border:1px solid #d1d5db; border-radius:12px; padding:10px 12px; font:inherit; background:#fff; color:var(--text); }
        button { min-height:42px; border-radius:12px; border:1px solid transparent; padding:10px 16px; font:inherit; font-weight:650; cursor:pointer; }
        .btn-primary { background:var(--blue); color:#fff; }
        .btn-secondary { background:#fff; border-color:#d1d5db; color:#374151; }
        .btn-danger { background:#fef2f2; color:var(--danger); border-color:#fecaca; }
        .section { margin-top:28px; }
        .card { background:var(--soft); border:1px solid rgba(17,24,39,.05); border-radius:16px; padding:24px; }
        .grid { display:grid; grid-template-columns:repeat(2,minmax(0,1fr)); gap:16px; }
        .metric { font-size:30px; font-weight:750; letter-spacing:-.04em; }
        .hint { margin-top:8px; font-size:12px; color:var(--muted); }
        .warn { color:#92400e; background:#fffbeb; border:1px solid #fde68a; border-radius:12px; padding:12px; }
        .error { color:var(--danger); background:#fef2f2; border:1px solid #fecaca; border-radius:12px; padding:12px; }
        .message-box { margin-top:16px; white-space:pre-wrap; word-break:break-word; font-family:SFMono-Regular,Consolas,"Liberation Mono",Menlo,monospace; font-size:13px; }
        .tabs { display:flex; gap:8px; padding:4px; background:#f3f4f6; border-radius:14px; margin:20px 0 24px; }
        .tab { flex:1; background:transparent; color:#4b5563; }
        .tab.active { background:#fff; color:var(--text); box-shadow:0 1px 3px rgba(15,23,42,.08); }
        .provider-counts { display:flex; flex-wrap:wrap; gap:8px; margin-top:10px; }
        .badge { display:inline-flex; align-items:center; border-radius:999px; padding:6px 10px; background:#eef2ff; color:#3730a3; font-size:12px; font-weight:650; }
        .toast-root { position:relative; z-index:80; display:grid; gap:10px; width:320px; max-width:calc(100vw - 120px); }
        .toast-alert { border-left:4px solid; border-radius:12px; padding:10px 12px; display:flex; align-items:center; gap:10px; box-shadow:0 18px 48px rgba(15,23,42,.12); }
        .bg-green-100 { background:#dcfce7; border-color:#22c55e; color:#14532d; }
        .bg-blue-100 { background:#dbeafe; border-color:#3b82f6; color:#1e3a8a; }
        .bg-yellow-100 { background:#fef9c3; border-color:#eab308; color:#713f12; }
        .bg-red-100 { background:#fee2e2; border-color:#ef4444; color:#7f1d1d; }
        .modal-backdrop { position:fixed; inset:0; display:grid; place-items:center; background:rgba(15,23,42,.42); padding:20px; }
        .modal { width:min(560px,100%); background:#fff; border-radius:18px; padding:24px; box-shadow:0 24px 80px rgba(15,23,42,.24); }
        .language-shell { position:relative; }
        .language-menu-button { min-width:44px; padding:10px; display:inline-flex; align-items:center; justify-content:center; background:#fff; border-color:#d1d5db; color:#374151; }
        .config-detail-grid { display:grid; grid-template-columns:repeat(2,minmax(0,1fr)); gap:16px; margin-top:16px; }
        .priority-rules-summary-card { grid-column:1/-1; }
        .priority-rule-summary-list { display:grid; gap:10px; margin-top:10px; font-size:13px; }
        .priority-rule-summary-provider { display:grid; gap:4px; padding:10px 12px; border:1px solid rgba(17,24,39,.05); border-radius:12px; background:#fff; }
        .priority-rule-summary-provider strong { font-size:14px; }
        .priority-rule-summary-provider span { color:var(--muted); line-height:1.55; }
        .readonly-config-card { display:grid; gap:12px; }
        .readonly-config-pre { margin:0; padding:14px 16px; border-radius:12px; background:#fff; border:1px solid rgba(17,24,39,.06); font-family:SFMono-Regular,Consolas,Menlo,monospace; font-size:12px; line-height:1.55; white-space:pre-wrap; word-break:break-word; color:#374151; max-height:420px; overflow:auto; }
        .yaml-hint { margin:0 0 12px; font-size:13px; color:var(--muted); }
        .help-content { line-height:1.65; color:#374151; }
        .help-content h3 { margin:20px 0 8px; color:#111827; font-size:16px; }
        .help-content ul { padding-left:20px; margin:0 0 16px; }
        .help-content li { margin-bottom:8px; }
        .help-content code { background:#f3f4f6; padding:1px 6px; border-radius:6px; font-size:13px; }
        .run-history-list { display:grid; gap:12px; }
        .run-history-card { background:#fff; border:1px solid rgba(17,24,39,.06); border-radius:14px; padding:14px 16px; display:grid; gap:10px; }
        .run-history-head { display:flex; flex-wrap:wrap; align-items:center; justify-content:space-between; gap:10px; }
        .run-history-meta { display:flex; flex-wrap:wrap; gap:8px; align-items:center; }
        .run-history-time { font-size:12px; color:var(--muted); font-weight:600; }
        .run-history-stats { display:flex; flex-wrap:wrap; gap:8px; }
        .run-history-stat { display:inline-flex; align-items:center; gap:4px; border-radius:999px; padding:4px 10px; background:#f8fafc; border:1px solid var(--line); font-size:12px; font-weight:650; color:#374151; }
        .run-history-providers { display:grid; grid-template-columns:repeat(3,minmax(0,1fr)); gap:8px; }
        .run-history-provider { display:grid; gap:4px; padding:10px 12px; border-radius:12px; background:var(--soft); border:1px solid rgba(17,24,39,.04); min-width:0; }
        .run-history-provider strong { font-size:13px; }
        .run-history-empty { text-align:center; color:var(--muted); padding:24px 12px; }
        .history-card-header { display:flex; align-items:flex-start; justify-content:space-between; gap:12px; flex-wrap:wrap; margin-bottom:4px; }
        .history-card-header h2 { margin:0; }
        .history-refresh-btn { min-height:40px; padding:8px 14px; }
        .provider-selection-grid { display:grid; grid-template-columns:minmax(0,.5fr) minmax(0,1.5fr); gap:20px; align-items:start; }
        .provider-selector-column,.selected-provider-column { min-width:0; }
        .provider-multi-select { position:relative; width:100%; }
        .provider-dropdown-trigger { width:100%; min-height:44px; display:flex; align-items:center; justify-content:space-between; border:1px solid #d1d5db; border-radius:12px; padding:10px 12px; background:#fff; color:var(--text); cursor:pointer; }
        .provider-dropdown-arrow { transition:transform .2s ease; color:var(--muted); display:inline-flex; align-items:center; }
        .provider-multi-select.open .provider-dropdown-arrow { transform:rotate(180deg); }
        .provider-dropdown-menu { position:absolute; top:calc(100% + 6px); left:0; width:100%; z-index:100; display:grid; gap:4px; padding:6px; background:#fff; border:1px solid var(--line); border-radius:12px; box-shadow:0 14px 28px rgba(15,23,42,.12); }
        .provider-dropdown-item { width:100%; min-height:38px; display:flex; align-items:center; gap:10px; padding:8px 10px; border:0; border-radius:9px; background:#fff; color:#374151; font-size:14px; font-weight:600; text-align:left; cursor:pointer; }
        .provider-dropdown-item:hover { background:#f3f4f6; }
        .provider-dropdown-item.active { background:#e0f2fe; color:#0369a1; }
        .selected-provider-tags { min-height:44px; padding:6px 10px; display:flex; align-items:center; flex-wrap:wrap; gap:8px; border:1px dashed #bae6fd; border-radius:12px; background:#f8fafc; }
        .provider-tag { display:inline-flex; align-items:center; gap:6px; border:1px solid #99f6e4; border-radius:999px; padding:5px 9px; background:#ccfbf1; color:#0f766e; font-size:12px; font-weight:700; }
        .provider-tag-remove { border:0; min-height:auto; padding:0; background:transparent; color:#64748b; font-size:14px; line-height:1; cursor:pointer; }
        .checkbox-list { display:grid; gap:10px; margin-top:12px; }
        .checkbox-wrapper-46 input[type="checkbox"] { display:none; visibility:hidden; }
        .checkbox-wrapper-46 .cbx { margin:auto; user-select:none; cursor:pointer; display:inline-flex; align-items:center; justify-content:space-between; width:100%; gap:10px; }
        .checkbox-wrapper-46 .cbx .cbx-box { position:relative; width:18px; height:18px; border-radius:3px; border:1px solid #9098a9; flex-shrink:0; }
        .checkbox-wrapper-46 .cbx .cbx-box svg { position:absolute; top:3px; left:2px; fill:none; stroke:#fff; stroke-width:2; stroke-linecap:round; stroke-linejoin:round; stroke-dasharray:16px; stroke-dashoffset:16px; }
        .checkbox-wrapper-46 .inp-cbx:checked + .cbx .cbx-box { background:#506eec; border-color:#506eec; }
        .checkbox-wrapper-46 .inp-cbx:checked + .cbx .cbx-box svg { stroke-dashoffset:0; }
        .custom-select-container { position: relative; width: 100%; }
        .custom-select-trigger { display:flex; align-items:center; justify-content:space-between; min-height:44px; border:1px solid #d1d5db; border-radius:12px; padding:10px 12px; background:#fff; cursor:pointer; user-select:none; font-size:14px; width:100%; }
        .custom-select-arrow { transition: transform 0.2s; color: var(--muted); display: flex; align-items: center; }
        .custom-select-container.open .custom-select-arrow { transform: rotate(180deg); }
        .custom-select-options { position:absolute; top:calc(100% + 4px); left:0; width:100%; background:#fff; border:1px solid #e5e7eb; border-radius:12px; box-shadow:0 10px 25px -5px rgba(0,0,0,.1); z-index:99; overflow:hidden; padding:4px; }
        .custom-select-option { padding:10px 12px; cursor:pointer; font-size:14px; border-radius:8px; color:#374151; }
        .custom-select-option:hover { background:#f3f4f6; }
        .custom-select-option.active { background:#e0f2fe; color:#0369a1; font-weight:600; }
        button:disabled { opacity:0.6; cursor:not-allowed; background:#94a3b8 !important; color:#f1f5f9 !important; border-color:#cbd5e1 !important; }
        [hidden] { display:none !important; }
        @media (max-width:720px) { body{padding:12px}.container{padding:20px}.grid,.config-detail-grid,.provider-selection-grid,.run-history-providers{grid-template-columns:1fr}.tabs{display:grid;grid-template-columns:1fr 1fr} }
    </style>
</head>
<body>
    <div class="container">
        <div class="topbar">
            <h1><span data-i18n="pageTitle">凭证优先级管理</span><span class="version-badge">v1.1.1</span></h1>
            <div class="topbar-actions">
                <div id="toastRoot" class="toast-root" aria-live="polite"></div>
                <div class="language-shell">
                    <button id="languageMenuButton" type="button" class="language-menu-button btn-secondary" onclick="switchLanguage()" aria-label="Switch language">
                        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><circle cx="12" cy="12" r="10"></circle><path d="M2 12h20"></path><path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"></path></svg>
                    </button>
                </div>
            </div>
        </div>

        <section id="loginGate" class="card">
            <h2 data-i18n="managementKeySection">CPA管理密钥验证</h2>
            <input id="managementKey" type="password" autocomplete="off" placeholder="Management key">
            <div style="margin-top:16px; display:flex; justify-content:flex-end">
                <button type="button" class="btn-primary" onclick="verifyManagementKey()" data-i18n="verifyKey">验证</button>
            </div>
            <div id="loginMessage" class="message-box" aria-live="polite"></div>
        </section>

        <main id="appShell" hidden>
            <nav class="tabs" aria-label="Plugin tabs">
                <button type="button" class="tab active" data-tab="overview" onclick="showTab('overview')" data-i18n="overviewTab">概览</button>
                <button type="button" class="tab" data-tab="history" onclick="showTab('history')" data-i18n="historyTab">执行记录</button>
                <button type="button" class="tab" data-tab="help" onclick="showTab('help')" data-i18n="helpTab">帮助</button>
            </nav>

            <section id="overviewPanel">
                <div class="grid">
                    <div class="card"><p data-i18n="totalCredentials">总凭证数</p><div id="totalCredentialValue" class="metric">-</div></div>
                    <div class="card"><p data-i18n="providerCredentialCount">提供商凭证数量</p><div id="providerCounts" class="provider-counts"></div></div>
                </div>
                <div class="section card">
                    <h2 data-i18n="configDetails">配置详情</h2>
                    <p class="yaml-hint" data-i18n="configVisualHint">自动优先级与排序规则请在 CPA「插件管理」可视化配置中修改；本页仅展示当前生效配置并支持手动排序。</p>
                    <div class="config-detail-grid">
                        <div class="card"><p data-i18n="autoPriorityEnabled">自动优先级排序</p><div id="autoApplySummary" class="metric">-</div></div>
                        <div class="card"><p data-i18n="sortingProviderSelection">排序提供商选择</p><div id="providerScopeSummary" class="metric">-</div></div>
                        <div class="card priority-rules-summary-card"><p data-i18n="priorityRulesSummaryTitle">优先级排序规则</p><div id="priorityRulesSummary" class="priority-rule-summary-list">-</div></div>
                    </div>
                    <div style="display:flex; justify-content:flex-end; gap:10px; flex-wrap:wrap; margin-top:16px">
                        <button id="openProviderModalButton" type="button" class="btn-primary" onclick="openProviderModal('apply')" data-i18n="runPrioritySort">执行优先级排序</button>
                    </div>
                </div>
            </section>

            <section id="historyPanel" hidden>
                <div class="card">
                    <div class="history-card-header">
                        <div>
                            <h2 data-i18n="historyTab">执行记录</h2>
                            <p class="yaml-hint" data-i18n="historyHint" style="margin-bottom:0;">展示最近 5 次自动或手动执行摘要（进程内保留，重启后清空）。</p>
                        </div>
                        <button id="refreshHistoryButton" type="button" class="btn-secondary history-refresh-btn" onclick="refreshRunHistory()" data-i18n="refreshHistory">刷新</button>
                    </div>
                    <div id="runHistoryList" class="run-history-list" style="margin-top:12px;"></div>
                </div>
            </section>

            <section id="helpPanel" hidden>
                <div class="card help-content">
                    <h2 data-i18n="helpTab">帮助</h2>
                    <h3 data-i18n="helpConfigTitle">可视化配置字段说明</h3>
                    <p data-i18n="helpConfigDesc">在 CPA 插件管理页打开 credential-priority，于「配置字段」中编辑并保存。字段说明如下：</p>
                    <ul>
                        <li><code>auto_apply</code>: <span data-i18n="helpAutoApply">是否开启定时自动优先级排序并写回。</span></li>
                        <li><code>provider_scope</code>: <span data-i18n="helpProviderScope">排序提供商：all 表示全部；或 antigravity|codex|xai（| 分隔）。</span></li>
                        <li><code>antigravity_model_group</code>: <span data-i18n="helpAntigravityGroup">Antigravity 配额模型组：gemini 或 claude_gpt。</span></li>
                        <li><code>interval</code>: <span data-i18n="helpInterval">自动排序间隔（分钟），填写纯数字，默认 15。</span></li>
                        <li><code>immediate_probe_limit</code>: <span data-i18n="helpImmediateProbe">单轮立即探测凭证上限，默认 30。</span></li>
                        <li><code>active_group_size</code>: <span data-i18n="helpActiveGroup">分批探测时每批凭证数，默认 10。</span></li>
                        <li><code>max_concurrency</code>: <span data-i18n="helpMaxConcurrency">探测并发上限，默认 6。</span></li>
                    </ul>
                    <h3 data-i18n="helpRulesTitle">优先级规则字段</h3>
                    <p data-i18n="helpRulesDesc">以下字段控制自定义排序规则；关闭 priority_rules.enabled 时使用内置策略。</p>
                    <ul>
                        <li><code>priority_rules.enabled</code>: <span data-i18n="helpRulesEnabled">是否启用自定义优先级规则。</span></li>
                        <li><code>priority_rules.antigravity.start_priority</code>: <span data-i18n="helpAgStart">Antigravity 可用凭证起始优先级。</span></li>
                        <li><code>priority_rules.codex.start_priority</code> / <code>free_depleted_priority</code> / <code>free_depleted_disabled</code> / <code>paid_depleted_disabled</code>: <span data-i18n="helpCodexRules">Codex 起始优先级与 Free/付费耗尽策略。</span></li>
                        <li><code>priority_rules.xai.*</code>: <span data-i18n="helpXaiRules">xAI 起始优先级与免费/周/月耗尽策略。</span></li>
                    </ul>
                    <h3 data-i18n="helpPageTitle">插件页能力</h3>
                    <ul>
                        <li data-i18n="helpPageOverview">概览：只读展示当前配置与凭证统计，并支持手动执行优先级排序。</li>
                        <li data-i18n="helpPageHistory">执行记录：查看最近 5 次自动/手动执行的结构化摘要。</li>
                        <li data-i18n="helpPageNoEdit">本页不编辑配置；配置请在 CPA 插件管理可视化字段中修改。</li>
                    </ul>
                </div>
            </section>
            <div id="banCountdown" class="error message-box" hidden></div>
        </main>
    </div>

    <div id="providerModal" class="modal-backdrop" hidden>
        <div class="modal" role="dialog" aria-modal="true" aria-labelledby="providerModalTitle">
            <h2 id="providerModalTitle" data-i18n="selectProviderTitle">选择要排序的提供商</h2>
            <p data-i18n="manualRunHint" style="font-size:13px; color:var(--muted); margin-bottom:16px;">该按钮为手动执行。自动排序请在 CPA 插件管理可视化配置中设置 auto_apply 等字段。</p>
            <p class="hint future-providers-hint" data-i18n="futureProvidersHint" style="margin-bottom:16px; font-size:12px; color:var(--muted);">目前支持 Antigravity、Codex 与 xAI 凭证。</p>
            <label for="manualProviderModeSelect" data-i18n="sortingProviderSelection">排序提供商选择</label>
            <div class="modal-selection-row provider-selection-grid" style="margin-bottom:16px;">
                <div class="provider-selector-column">
                    <select id="manualProviderModeSelect" onchange="syncManualProviderModeVisibility()" style="display:none;"><option value="all" data-i18n="providerAll">全部</option><option value="antigravity">Antigravity</option><option value="codex">Codex</option></select>
                    <div id="manualProviderControls" class="checkbox-list" hidden style="margin-top:0;"></div>
                    <div id="manualProviderMultiSelect" class="provider-multi-select" data-provider-multi-select="manual"></div>
                </div>
                <div class="selected-provider-column">
                    <div id="manualSelectedProviderTags" class="selected-provider-tags"></div>
                </div>
            </div>
            <div id="manualAntigravityModelGroupRow" class="modal-selection-row" style="display:flex; gap:20px; align-items:flex-start; margin-bottom:16px;">
                <div style="flex:1;">
                    <label for="manualAntigravityModelGroupSelect" data-i18n="antigravityModelGroup">Antigravity 模型组</label>
                    <select id="manualAntigravityModelGroupSelect"><option value="gemini" data-i18n="geminiModelGroup">Gemini 模型</option><option value="claude_gpt" data-i18n="claudeGPTModelGroup">Claude 和 GPT 模型</option></select>
                </div>
            </div>
            <div id="modalNotice" class="message-box" role="status"></div>
            <div style="display:flex; justify-content:flex-end; gap:10px; flex-wrap:wrap; margin-top:18px">
                <button id="executePriorityButton" type="button" class="btn-primary" onclick="executeFromModal()" data-i18n="applyRun">执行</button>
                <button type="button" class="btn-danger" onclick="closeProviderModal()" data-i18n="cancel">关闭</button>
            </div>
        </div>
    </div>

    <div id="resultModal" class="modal-backdrop" hidden>
        <div class="modal" role="dialog" aria-modal="true" aria-labelledby="resultModalTitle" style="width: min(680px, 95%); max-width: 680px;">
            <h2 id="resultModalTitle" data-i18n="runCompleted" style="margin-bottom: 16px;">手动任务已完成</h2>
            <div id="resultDetailsContainer" style="max-height: 320px; overflow-y: auto; background: #f8fafc; border: 1px solid #e5e7eb; border-radius: 12px; padding: 16px; display: flex; flex-direction: column; gap: 10px;"></div>
            <div style="display:flex; justify-content:flex-end; margin-top:20px">
                <button type="button" class="btn-secondary" onclick="closeResultModal()" data-i18n="cancel" style="min-width: 100px;">关闭</button>
            </div>
        </div>
    </div>

    <script>
        const CONFIG_PATH="/v0/management/plugins/credential-priority/config";
        const AUTH_FILES_PATH="/v0/management/auth-files";
        const DIAGNOSTICS_PATH="/v0/management/plugins/credential-priority/diagnostics";
        const RUN_PATH="/v0/management/plugins/credential-priority/run";
        const translations={
            "zh-CN":{pageTitle:"凭证优先级管理",managementKeySection:"CPA管理密钥验证",verifyKey:"验证",overviewTab:"概览",historyTab:"执行记录",helpTab:"帮助",totalCredentials:"总凭证数",providerCredentialCount:"提供商凭证数量",configDetails:"配置详情",configVisualHint:"自动优先级与排序规则请在 CPA「插件管理」可视化配置中修改；本页仅展示当前生效配置并支持手动排序。",historyHint:"展示最近 5 次自动或手动执行摘要（进程内保留，重启后清空）。",historyEmpty:"暂无执行记录。",refreshHistory:"刷新",historyRefreshed:"执行记录已刷新。",manualRunHint:"该按钮为手动执行。自动排序请在 CPA 插件管理可视化配置中设置 auto_apply 等字段。",futureProvidersHint:"目前支持 Antigravity、Codex 与 xAI 凭证。",priorityRulesSummaryTitle:"优先级排序规则",priorityRuleStartPriority:"可用凭证起始优先级",priorityRuleFreeDepletedPriority:"免费额度为 0 时优先级",priorityRuleFreeDepletedDisabled:"免费额度为 0 时禁用",priorityRulePaidDepletedDisabled:"Plus、Pro、Team 耗尽时禁用",priorityRuleWeeklyDepletedPriority:"仅周限额耗尽时优先级",priorityRuleMonthlyWeeklyDepletedPriority:"周与月均耗尽时优先级",priorityRuleMonthlyWeeklyDepletedDisabled:"周与月均耗尽时禁用",failedQuotaFetch:"获取配额失败",manualRetry:"重试",runPrioritySort:"执行优先级排序",overviewLoadingText:"加载凭证统计中...",autoPriorityEnabled:"自动优先级排序",sortingProviderSelection:"排序提供商选择",antigravityModelGroup:"Antigravity 模型组",geminiModelGroup:"Gemini 模型",claudeGPTModelGroup:"Claude 和 GPT 模型",providerAll:"全部",providerCustom:"自定义",selectProviderTitle:"选择要排序的提供商",autoStatusOn:"已开启",autoStatusOff:"已关闭",applyRun:"执行",cancel:"关闭",running:"执行中...",priorityUnset:"未设置",managementKeyRequired:"请先填写管理密钥。",configLoaded:"配置已读取。",runCompleted:"手动任务已完成",banHint:"管理密钥多次错误已触发风控，可等待倒计时结束，或可重启 CPA / CLIProxyAPI 后再试。",noChanges:"本次没有优先级变化",kindAuto:"自动排序",kindManual:"手动执行",kindDryRun:"试运行",kindApply:"写回",triggerLabel:"触发方式",statCredentials:"凭证数",statSucceeded:"成功",statFailed:"失败",statSkipped:"跳过",providerBreakdown:"按提供商",helpConfigTitle:"可视化配置字段说明",helpConfigDesc:"在 CPA 插件管理页打开 credential-priority，于「配置字段」中编辑并保存。字段说明如下：",helpAutoApply:"是否开启定时自动优先级排序并写回。",helpProviderScope:"排序提供商：all 表示全部；或 antigravity|codex|xai（多个用 | 分隔）。",helpAntigravityGroup:"Antigravity 配额模型组：gemini 或 claude_gpt。",helpInterval:"自动排序间隔（分钟），填写纯数字，默认 15。",helpImmediateProbe:"单轮立即探测凭证上限，默认 30。",helpActiveGroup:"分批探测时每批凭证数，默认 10。",helpMaxConcurrency:"探测并发上限，默认 6。",helpRulesTitle:"优先级规则字段",helpRulesDesc:"以下字段控制自定义排序规则；关闭 priority_rules.enabled 时使用内置策略。",helpRulesEnabled:"是否启用自定义优先级规则。",helpAgStart:"Antigravity 可用凭证起始优先级。",helpCodexRules:"Codex 起始优先级与 Free/付费耗尽策略。",helpXaiRules:"xAI 起始优先级与免费/周/月耗尽策略。",helpPageTitle:"插件页能力",helpPageOverview:"概览：只读展示当前配置与凭证统计，并支持手动执行优先级排序。",helpPageHistory:"执行记录：查看最近 5 次自动/手动执行的结构化摘要。",helpPageNoEdit:"本页不编辑配置；配置请在 CPA 插件管理可视化字段中修改。"},
            "en-US":{pageTitle:"Credential Priority",managementKeySection:"CPA Management Key Verification",verifyKey:"Verify",overviewTab:"Overview",historyTab:"Run History",helpTab:"Help",totalCredentials:"Total Credentials",providerCredentialCount:"Provider Credential Count",configDetails:"Config Details",configVisualHint:"Change auto priority and rules in CPA Plugin Manager visual config fields. This page shows the effective config and supports manual sorting only.",historyHint:"Shows the last 5 automatic or manual runs (in-memory; cleared after restart).",historyEmpty:"No run history yet.",refreshHistory:"Refresh",historyRefreshed:"Run history refreshed.",manualRunHint:"Manual run only. Configure auto_apply and related fields in CPA Plugin Manager visual config.",futureProvidersHint:"Antigravity, Codex, and xAI credentials are currently supported.",priorityRulesSummaryTitle:"Priority sorting rules",priorityRuleStartPriority:"Available credential start priority",priorityRuleFreeDepletedPriority:"Free depleted priority",priorityRuleFreeDepletedDisabled:"Disable Free when quota is 0",priorityRulePaidDepletedDisabled:"Disable Plus, Pro, and Team when depleted",priorityRuleWeeklyDepletedPriority:"Weekly-only depleted priority",priorityRuleMonthlyWeeklyDepletedPriority:"Monthly and weekly depleted priority",priorityRuleMonthlyWeeklyDepletedDisabled:"Disable when monthly and weekly are depleted",failedQuotaFetch:"Failed to fetch quota",manualRetry:"Retry",runPrioritySort:"Run Priority Sort",overviewLoadingText:"Loading credential summary...",autoPriorityEnabled:"Automatic Priority Sorting",sortingProviderSelection:"Sorting provider selection",antigravityModelGroup:"Antigravity model group",geminiModelGroup:"Gemini models",claudeGPTModelGroup:"Claude and GPT models",providerAll:"All",providerCustom:"Custom",selectProviderTitle:"Select Providers",autoStatusOn:"ON",autoStatusOff:"OFF",applyRun:"Execute",cancel:"Close",running:"Running...",priorityUnset:"Unset",managementKeyRequired:"Please enter the management key first.",configLoaded:"Config loaded.",runCompleted:"Manual task completed",banHint:"Too many wrong keys triggered risk control. Wait for the countdown, or restart CPA / CLIProxyAPI and retry.",noChanges:"No priority changes this time",kindAuto:"Auto sort",kindManual:"Manual run",kindDryRun:"Dry run",kindApply:"Apply",triggerLabel:"Trigger",statCredentials:"Credentials",statSucceeded:"Succeeded",statFailed:"Failed",statSkipped:"Skipped",providerBreakdown:"By provider",helpConfigTitle:"Visual config fields",helpConfigDesc:"Open credential-priority in CPA Plugin Manager and edit the Config fields. Field notes:",helpAutoApply:"Enable scheduled automatic priority sorting and write-back.",helpProviderScope:"Providers to sort: all, or antigravity|codex|xai separated by |.",helpAntigravityGroup:"Antigravity quota model group: gemini or claude_gpt.",helpInterval:"Auto-sort interval in minutes; enter a plain number (default 15).",helpImmediateProbe:"Max credentials probed immediately per round (default 30).",helpActiveGroup:"Credentials per batch when probing in batches (default 10).",helpMaxConcurrency:"Probe concurrency limit (default 6).",helpRulesTitle:"Priority rule fields",helpRulesDesc:"Custom sorting rules. When priority_rules.enabled is false, built-in strategy is used.",helpRulesEnabled:"Enable custom priority rules.",helpAgStart:"Antigravity start priority for available credentials.",helpCodexRules:"Codex start priority and Free/paid depletion policy.",helpXaiRules:"xAI start priority and free/weekly/monthly depletion policy.",helpPageTitle:"Plugin page capabilities",helpPageOverview:"Overview: read-only effective config and credential stats, plus manual priority sort.",helpPageHistory:"Run history: structured summaries of the last 5 automatic/manual runs.",helpPageNoEdit:"This page does not edit config; use CPA Plugin Manager visual fields."}
        };
        let activeLanguage="zh-CN";
        let providerOptions=[];
        let currentConfig={provider_scope:"all",selected_providers:[]};
        let currentResult=null;
        let credentialSummaryLoading=false;
        let pendingRunMode="apply";
        let runHistoryCache=[];
        const defaultPriorityRuleConfig={enabled:false,antigravity:{start_priority:100},codex:{start_priority:100,free_depleted_priority:-1,free_depleted_disabled:true,paid_depleted_disabled:false},xai:{start_priority:100,free_depleted_priority:-1,free_depleted_disabled:false,weekly_depleted_priority:-1,monthly_and_weekly_depleted_priority:-1,monthly_and_weekly_depleted_disabled:true}};
        function getProviderDisplayName(provider) {
            const lower = String(provider || "").trim().toLowerCase();
            const names = {"antigravity":"Antigravity","codex":"Codex","xai":"xAI","x-ai":"xAI"};
            return names[lower] || provider;
        }
        function textFor(key){return (translations[activeLanguage]&&translations[activeLanguage][key])||translations["zh-CN"][key]||key;}
        function setMessage(value){if(value){showToast(value,"info");}}
        function setLoginMessage(value){document.getElementById("loginMessage").textContent=value;}
        function requireManagementKey(){const key=document.getElementById("managementKey").value.trim();if(!key){setLoginMessage(textFor("managementKeyRequired"));showToast(textFor("managementKeyRequired"),"warning");return "";}return key;}
        function authHeaders(key){return {"Authorization":"Bearer "+key,"Content-Type":"application/json"};}
        async function managementFetch(path, options){const key=requireManagementKey();if(!key){return null;}const response=await fetch(path,{...(options||{}),headers:{...authHeaders(key),...((options&&options.headers)||{})}});const text=await response.text();if(!response.ok){throw new Error(text||response.statusText);}return text?JSON.parse(text):{};}
        async function loadConfig(options){const config=await managementFetch(CONFIG_PATH,{method:"GET"});if(!config){return null;}currentConfig=config;updateConfigSummary(config);if(config.antigravity_model_group){document.getElementById("manualAntigravityModelGroupSelect").value=config.antigravity_model_group;}if(!options||!options.silent){showToast(textFor("configLoaded"),"success");}return config;}
        async function loadDiagnostics(options){try{const diag=await managementFetch(DIAGNOSTICS_PATH,{method:"GET"});runHistoryCache=Array.isArray(diag&&diag.run_history)?diag.run_history:[];renderRunHistory();if(options&&options.toast){showToast(textFor("historyRefreshed"),"success");}}catch(err){runHistoryCache=[];renderRunHistory();if(options&&options.toast){handleManagementError(err,false);}}}
        async function refreshRunHistory(){const button=document.getElementById("refreshHistoryButton");let oldText="";if(button){button.disabled=true;oldText=button.textContent;button.textContent=textFor("running");}try{await loadDiagnostics({toast:true});}finally{if(button){button.disabled=false;button.textContent=oldText||textFor("refreshHistory");}}}
        async function verifyManagementKey(){try{const config=await loadConfig({silent:true});if(!config){return;}document.getElementById("loginGate").hidden=true;document.getElementById("appShell").hidden=false;showToast(textFor("configLoaded"),"success");await loadCredentialSummary();await loadDiagnostics();}catch(err){handleManagementError(err,true);}}
        function readSelectedProviders(selector){return Array.from(document.querySelectorAll(selector)).filter(function(item){return item.checked;}).map(function(item){return item.dataset.provider||item.dataset.manualProvider;});}
        function readProviderSelection(selectId, selector){const mode=document.getElementById(selectId).value;if(mode==="all"){return [];}return readSelectedProviders(selector);}
        function normalizeHostConfig(raw){const config=Object.assign({},raw||{});const flat={};for(const key of Object.keys(config)){if(key.indexOf("priority_rules.")===0){flat[key.slice("priority_rules.".length)]=config[key];}}if(!config.priority_rules||typeof config.priority_rules!=="object"){config.priority_rules={};}const rules=config.priority_rules;if(typeof flat.enabled==="boolean"){rules.enabled=flat.enabled;}rules.antigravity=Object.assign({},rules.antigravity||{});rules.codex=Object.assign({},rules.codex||{});rules.xai=Object.assign({},rules.xai||{});for(const key of Object.keys(flat)){if(key==="enabled"){continue;}const parts=key.split(".");if(parts.length!==2){continue;}const provider=parts[0];const field=parts[1];if(!rules[provider]||typeof rules[provider]!=="object"){rules[provider]={};}rules[provider][field]=flat[key];}if(rules.codex&&rules.codex.paid_depleted_disabled===undefined&&typeof rules.codex.paid_depleted_keeps_enabled==="boolean"){rules.codex.paid_depleted_disabled=!rules.codex.paid_depleted_keeps_enabled;}config.priority_rules=rules;return config;}
        function formatProviderScopeSummary(config){const scope=String((config&&config.provider_scope)||"all").trim().toLowerCase();if(!scope||scope==="all"){return textFor("providerAll");}if(scope==="selected"){const selected=(config.selected_providers||[]).map(getProviderDisplayName).filter(Boolean);return selected.length?selected.join(" · "):textFor("providerAll");}const parts=scope.split(/[|,;\s]+/).map(function(item){return item.trim();}).filter(Boolean);if(parts.length===0){return textFor("providerAll");}return parts.map(getProviderDisplayName).join(" · ");}
        function updateConfigSummary(config){const normalized=normalizeHostConfig(config||{});currentConfig=normalized;document.getElementById("autoApplySummary").textContent=normalized.auto_apply===true?textFor("autoStatusOn"):textFor("autoStatusOff");document.getElementById("providerScopeSummary").textContent=formatProviderScopeSummary(normalized);updatePriorityRulesSummary(normalized);}
        function kindLabel(kind,trigger){const k=String(kind||"").toLowerCase();if(k==="auto_apply"){return textFor("kindAuto");}if(k==="dry_run"){return textFor("kindDryRun");}if(k==="apply"){const t=String(trigger||"").toLowerCase();if(t.indexOf("auto")>=0){return textFor("kindAuto");}return textFor("kindManual");}return kind||textFor("kindApply");}
        function formatHistoryTime(value){if(!value){return "-";}try{const d=new Date(value);if(Number.isNaN(d.getTime())){return String(value);}return d.toLocaleString(activeLanguage==="zh-CN"?"zh-CN":"en-US");}catch(e){return String(value);}}
        function statBadge(label,value){const span=document.createElement("span");span.className="run-history-stat";span.textContent=label+summarySeparator()+String(value==null?0:value);return span;}
        function credentialCount(entry){const attempted=Number(entry&&entry.attempted||0);const skipped=Number(entry&&entry.skipped||0);const succeeded=Number(entry&&entry.succeeded||0);const failed=Number(entry&&entry.failed||0);const total=attempted+skipped;if(total>0){return total;}return succeeded+failed+skipped;}
        function providerCredentialCount(p){const attempted=Number(p&&p.attempted||0);const skipped=Number(p&&p.skipped||0);const total=attempted+skipped;if(total>0){return total;}return Number(p&&p.succeeded||0)+Number(p&&p.failed||0)+skipped;}
        function providerStatLine(p){return textFor("statCredentials")+summarySeparator()+providerCredentialCount(p)+" · "+textFor("statSucceeded")+summarySeparator()+(p.succeeded||0)+" · "+textFor("statFailed")+summarySeparator()+(p.failed||0)+" · "+textFor("statSkipped")+summarySeparator()+(p.skipped||0);}
        function renderRunHistory(){const root=document.getElementById("runHistoryList");if(!root){return;}root.textContent="";const items=Array.isArray(runHistoryCache)?runHistoryCache.slice(0,5):[];if(items.length===0){const empty=document.createElement("div");empty.className="run-history-empty";empty.textContent=textFor("historyEmpty");root.appendChild(empty);return;}for(const entry of items){const card=document.createElement("div");card.className="run-history-card";const head=document.createElement("div");head.className="run-history-head";const meta=document.createElement("div");meta.className="run-history-meta";const kind=document.createElement("span");kind.className="badge";kind.textContent=kindLabel(entry.kind,entry.trigger);const time=document.createElement("span");time.className="run-history-time";time.textContent=formatHistoryTime(entry.at);meta.append(kind,time);head.appendChild(meta);card.appendChild(head);const stats=document.createElement("div");stats.className="run-history-stats";stats.append(statBadge(textFor("statCredentials"),credentialCount(entry)),statBadge(textFor("statSucceeded"),entry.succeeded),statBadge(textFor("statFailed"),entry.failed),statBadge(textFor("statSkipped"),entry.skipped));card.appendChild(stats);if(Array.isArray(entry.providers)&&entry.providers.length>0){const title=document.createElement("div");title.style.fontSize="12px";title.style.color="var(--muted)";title.style.fontWeight="650";title.textContent=textFor("providerBreakdown");card.appendChild(title);const list=document.createElement("div");list.className="run-history-providers";for(const p of entry.providers){const row=document.createElement("div");row.className="run-history-provider";const name=document.createElement("strong");name.textContent=getProviderDisplayName(p.name||p.Name||"");row.appendChild(name);const line=document.createElement("span");line.style.fontSize="12px";line.style.color="var(--muted)";line.textContent=providerStatLine(p);row.appendChild(line);if(p.error){const err=document.createElement("span");err.style.fontSize="12px";err.style.color="var(--danger)";err.textContent=p.error;row.appendChild(err);}list.appendChild(row);}card.appendChild(list);}root.appendChild(card);}}
        function summarySeparator(){return activeLanguage==="zh-CN"?"：":": ";}
        function priorityRuleLine(label,value){return label+summarySeparator()+value;}
        function priorityRuleSummaryProvider(title,lines){const section=document.createElement("div");section.className="priority-rule-summary-provider";const heading=document.createElement("strong");heading.textContent=title;section.appendChild(heading);for(const line of lines){const item=document.createElement("span");item.textContent=line;section.appendChild(item);}return section;}
        function pickRuleValue(flat, nested, key, fallback){if(flat&&flat[key]!==undefined&&flat[key]!==null){return flat[key];}if(nested&&nested[key]!==undefined&&nested[key]!==null){return nested[key];}return fallback;}
        function updatePriorityRulesSummary(config){const root=document.getElementById("priorityRulesSummary");if(!root){return;}const rules=(config&&config.priority_rules)||{};const ag=rules.antigravity||{};const codex=rules.codex||{};const xai=rules.xai||{};const d=defaultPriorityRuleConfig;root.textContent="";root.append(priorityRuleSummaryProvider("Antigravity",[priorityRuleLine(textFor("priorityRuleStartPriority"),pickRuleValue(null,ag,"start_priority",d.antigravity.start_priority))]),priorityRuleSummaryProvider("Codex",[priorityRuleLine(textFor("priorityRuleStartPriority"),pickRuleValue(null,codex,"start_priority",d.codex.start_priority)),priorityRuleLine(textFor("priorityRuleFreeDepletedPriority"),pickRuleValue(null,codex,"free_depleted_priority",d.codex.free_depleted_priority)),priorityRuleLine(textFor("priorityRuleFreeDepletedDisabled"),pickRuleValue(null,codex,"free_depleted_disabled",d.codex.free_depleted_disabled)?textFor("autoStatusOn"):textFor("autoStatusOff")),priorityRuleLine(textFor("priorityRulePaidDepletedDisabled"),pickRuleValue(null,codex,"paid_depleted_disabled",d.codex.paid_depleted_disabled)?textFor("autoStatusOn"):textFor("autoStatusOff"))]),priorityRuleSummaryProvider("xAI",[priorityRuleLine(textFor("priorityRuleStartPriority"),pickRuleValue(null,xai,"start_priority",d.xai.start_priority)),priorityRuleLine(textFor("priorityRuleFreeDepletedPriority"),pickRuleValue(null,xai,"free_depleted_priority",d.xai.free_depleted_priority)),priorityRuleLine(textFor("priorityRuleFreeDepletedDisabled"),pickRuleValue(null,xai,"free_depleted_disabled",d.xai.free_depleted_disabled)?textFor("autoStatusOn"):textFor("autoStatusOff")),priorityRuleLine(textFor("priorityRuleWeeklyDepletedPriority"),pickRuleValue(null,xai,"weekly_depleted_priority",d.xai.weekly_depleted_priority)),priorityRuleLine(textFor("priorityRuleMonthlyWeeklyDepletedPriority"),pickRuleValue(null,xai,"monthly_and_weekly_depleted_priority",d.xai.monthly_and_weekly_depleted_priority)),priorityRuleLine(textFor("priorityRuleMonthlyWeeklyDepletedDisabled"),pickRuleValue(null,xai,"monthly_and_weekly_depleted_disabled",d.xai.monthly_and_weekly_depleted_disabled)?textFor("autoStatusOn"):textFor("autoStatusOff"))]));}
        function supportedProviderOptions(options){const labels=new Map(options.map(function(item){return [item.value,item.label];}));return ["antigravity","codex","xai"].map(function(value){return {value:value,label:labels.get(value)||getProviderDisplayName(value)};});}
        function renderProviderCounts(options){const root=document.getElementById("providerCounts");root.textContent="";for(const provider of options){const badge=document.createElement("span");badge.className="badge";badge.textContent=provider.label+": "+provider.count;root.appendChild(badge);}if(options.length===0){const empty=document.createElement("span");empty.className="badge";empty.textContent="0";root.appendChild(empty);}}
        function renderProviderOptions(options, config){providerOptions=supportedProviderOptions(options);renderProviderCounts(options);renderCheckboxes("manualProviderControls",providerOptions,"manual",[]);renderProviderMultiSelect("manual");syncManualProviderModeVisibility();syncAntigravityModelGroupVisibility();renderSelectedProviderTags("manual");}
        function renderCheckboxes(rootId, options, kind, selected){const root=document.getElementById(rootId);if(!root){return;}root.textContent="";for(const provider of options){const val=provider.value.toLowerCase();if(val==="antigravity"||val==="codex"||val==="xai"){root.appendChild(createProviderCheckbox(provider,kind,selected||[]));}}}
        function createProviderCheckbox(provider, kind, selected){const wrapper=document.createElement("div");wrapper.className="checkbox-wrapper-46";const input=document.createElement("input");input.type="checkbox";input.className="inp-cbx";input.id=kind+"-provider-"+provider.value.replace(/[^a-z0-9_-]/g,"-");if(kind==="manual"){input.dataset.manualProvider=provider.value;}else{input.dataset.provider=provider.value;}input.checked=selected.includes(provider.value);input.addEventListener("change", function(){syncAntigravityModelGroupVisibility();renderSelectedProviderTags(kind);});const label=document.createElement("label");label.className="cbx";label.htmlFor=input.id;const box=document.createElement("span");box.className="cbx-box";box.innerHTML='<svg viewBox="0 0 12 10" height="10px" width="12px"><polyline points="1.5 6 4.5 9 10.5 1"></polyline></svg>';const text=document.createElement("span");text.className="cbx-text";text.textContent=provider.label;label.append(text,box);wrapper.append(input,label);return wrapper;}
        function renderProviderMultiSelect(kind){const root=document.getElementById("manualProviderMultiSelect");if(!root){return;}const selectId="manualProviderModeSelect";root.textContent="";const trigger=document.createElement("button");trigger.type="button";trigger.className="provider-dropdown-trigger";trigger.onclick=function(event){toggleProviderDropdown(kind,event);};const label=document.createElement("span");label.className="provider-dropdown-label";label.textContent=providerSelectionLabel(kind);const arrow=document.createElement("span");arrow.className="provider-dropdown-arrow";arrow.innerHTML='<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M6 9l6 6 6-6"></path></svg>';trigger.append(label,arrow);const menu=document.createElement("div");menu.className="provider-dropdown-menu";menu.hidden=true;root.append(trigger,menu);addProviderDropdownItem(menu,kind,"all",textFor("providerAll"));for(const provider of providerOptions){addProviderDropdownItem(menu,kind,provider.value,provider.label);}document.getElementById(selectId).value=providerSelectionValue(kind);}
        function addProviderDropdownItem(menu,kind,value,label){const item=document.createElement("button");item.type="button";item.className="provider-dropdown-item";item.setAttribute("data-provider-option",value);if(providerOptionSelected(kind,value)){item.classList.add("active");}const text=document.createElement("span");text.textContent=label;item.append(text);item.onclick=function(event){event.stopPropagation();selectProviderOption(kind,value);};menu.appendChild(item);}
        function providerOptionSelected(kind,value){const selector="[data-manual-provider]";const selected=readSelectedProviders(selector);if(value==="all"){return selected.length===0;}return selected.includes(value);}
        function toggleProviderDropdown(kind,event){if(event){event.stopPropagation();}const root=document.getElementById("manualProviderMultiSelect");const menu=root&&root.querySelector(".provider-dropdown-menu");if(!menu){return;}const next=menu.hidden;closeProviderDropdowns();menu.hidden=!next;root.classList.toggle("open",next);}
        function closeProviderDropdowns(){document.querySelectorAll(".provider-dropdown-menu").forEach(function(item){item.hidden=true;item.parentElement.classList.remove("open");});}
        function selectProviderOption(kind,value){const selector="[data-manual-provider]";const select=document.getElementById("manualProviderModeSelect");if(value==="all"){select.value="all";for(const input of document.querySelectorAll(selector)){input.checked=false;}}else{select.value=value;for(const input of document.querySelectorAll(selector)){const inputValue=input.dataset.manualProvider;if(inputValue===value){input.checked=!input.checked;}}if(readSelectedProviders(selector).length===0){select.value="all";}}syncManualProviderModeVisibility();syncAntigravityModelGroupVisibility();renderSelectedProviderTags(kind);refreshProviderDropdownState(kind);}
        function providerSelectionValue(kind){const selected=readSelectedProviders("[data-manual-provider]");return selected.length===0?"all":selected[0];}
        function providerSelectionLabel(kind){const selected=readSelectedProviders("[data-manual-provider]");return selected.length===0?textFor("providerAll"):textFor("providerCustom");}
        function refreshProviderDropdownState(kind){const root=document.getElementById("manualProviderMultiSelect");if(!root){return;}const label=root.querySelector(".provider-dropdown-label");if(label){label.textContent=providerSelectionLabel(kind);}for(const item of root.querySelectorAll("[data-provider-option]")){item.classList.toggle("active",providerOptionSelected(kind,item.getAttribute("data-provider-option")));}document.getElementById("manualProviderModeSelect").value=providerSelectionValue(kind);}
        function renderSelectedProviderTags(kind){const root=document.getElementById("manualSelectedProviderTags");if(!root){return;}const selected=readSelectedProviders("[data-manual-provider]");root.textContent="";root.hidden=selected.length===0;for(const value of selected){const tag=document.createElement("span");tag.className="provider-tag";tag.textContent=getProviderDisplayName(value);const remove=document.createElement("button");remove.type="button";remove.className="provider-tag-remove";remove.textContent="×";remove.onclick=function(){selectProviderOption(kind,value);};tag.appendChild(remove);root.appendChild(tag);}}
        function setOverviewLoading(loading){credentialSummaryLoading=loading;const button=document.getElementById("openProviderModalButton");if(button){button.disabled=loading;}if(loading){document.getElementById("totalCredentialValue").textContent=textFor("overviewLoadingText");const root=document.getElementById("providerCounts");root.textContent="";const badge=document.createElement("span");badge.className="badge";badge.textContent=textFor("overviewLoadingText");root.appendChild(badge);}}
        async function loadCredentialSummary(){setOverviewLoading(true);try{const result=await managementFetch(AUTH_FILES_PATH,{method:"GET"});const files=Array.isArray(result.files)?result.files:[];document.getElementById("totalCredentialValue").textContent=String(files.length);const counts=new Map();for(const item of files){const value=String(item.provider||"").trim().toLowerCase();if(!value){continue;}const current=counts.get(value)||{value:value,label:getProviderDisplayName(item.provider||value),count:0};current.count++;counts.set(value,current);}renderProviderOptions(Array.from(counts.values()).sort(function(a,b){return a.label.localeCompare(b.label);}),currentConfig);}catch(err){handleManagementError(err,false);}finally{setOverviewLoading(false);}}
        function selectedManualProviders(){return readProviderSelection("manualProviderModeSelect","[data-manual-provider]");}
        function providerQuery(providers, authIndex){const group="antigravity_model_group="+encodeURIComponent(document.getElementById("manualAntigravityModelGroupSelect").value);const auth=authIndex?"&auth_index="+encodeURIComponent(authIndex):"";if(!providers||providers.length===0){return "provider_scope=all&"+group+auth;}return providers.map(function(provider){return "provider="+encodeURIComponent(provider);}).join("&")+"&"+group+auth;}
        function credentialDisplayName(c){return c.account || c.email || c.name || c.auth_index || "";}
        function priorityChangeText(c){const from = c.priority_missing ? textFor("priorityUnset") : c.priority_from;return from + " -> " + c.priority_to;}
        async function runCredentialPriority(mode, providers, button, authIndex, mergeExisting){const control=button||document.getElementById("executePriorityButton");let oldText="";setManualRunControlsDisabled(true);if(control){control.disabled=true;oldText=control.textContent;control.textContent=textFor("running");}try{const query=providerQuery(providers||[],authIndex);const path=RUN_PATH+"?mode="+encodeURIComponent(mode)+"&"+query;const result=await managementFetch(path,{method:"POST"});if(result){if(mergeExisting){closeProviderModal();showResult(mergeResults(currentResult,result));}else{closeProviderModal();showResult(result);}showToast(textFor("runCompleted"),"success");await loadCredentialSummary();await loadDiagnostics();}}catch(err){handleManagementError(err,false);}finally{setManualRunControlsDisabled(false);if(control){control.disabled=false;control.textContent=oldText;}}}
        function setManualRunControlsDisabled(disabled){if(disabled){closeProviderDropdowns();}for(const root of [document.getElementById("manualProviderMultiSelect"),document.getElementById("manualSelectedProviderTags"),document.getElementById("manualAntigravityModelGroupRow")]){if(!root){continue;}root.querySelectorAll("button,input,select").forEach(function(control){control.disabled=disabled;});}const modelGroup=document.getElementById("manualAntigravityModelGroupSelect");if(modelGroup){modelGroup.disabled=disabled;}}
        function closeResultModal(){document.getElementById("resultModal").hidden=true;}
        function resultKey(c){return c.retry_auth_index||c.auth_index||credentialDisplayName(c);}
        function removeMatchingChanges(changes,key){return changes.filter(function(item){return resultKey(item)!==key;});}
        function mergeResults(previous,next){if(!previous||!Array.isArray(previous.changes)){return next;}if(!next||!Array.isArray(next.changes)){return previous;}const merged={...previous,changes:previous.changes.slice()};for(const change of next.changes){const key=resultKey(change);merged.changes=removeMatchingChanges(merged.changes,key);merged.changes.push(change);}merged.attempted=next.attempted;merged.succeeded=next.succeeded;merged.failed=merged.changes.filter(function(item){return item.status==="failed";}).length;merged.skipped=merged.changes.filter(function(item){return item.status==="skipped";}).length;return merged;}
        function showResult(result){
            currentResult=result;
            const container=document.getElementById("resultDetailsContainer");
            container.innerHTML="";
            if(!result||!Array.isArray(result.changes)){
                container.innerHTML="<div style=\"text-align: center; padding: 12px; color: var(--muted);\">" + textFor("noChanges") + "</div>";
                document.getElementById("resultModal").hidden=false;
                return;
            }
            const changes=result.changes.filter(shouldShowChange);
            if(changes.length===0){
                container.innerHTML="<div style=\"text-align: center; padding: 12px; color: var(--muted);\">" + textFor("noChanges") + "</div>";
                document.getElementById("resultModal").hidden=false;
                return;
            }
            changes.forEach(function(c){
                const row=document.createElement("div");
                row.style.display="flex";
                row.style.alignItems="center";
                row.style.justifyContent="space-between";
                row.style.background="#fff";
                row.style.border="1px solid #f1f5f9";
                row.style.borderRadius="8px";
                row.style.padding="10px 14px";
                row.style.gap="12px";
                const pName=getProviderDisplayName(c.provider||"unknown");
                const badge=document.createElement("span");
                badge.className="badge";
                badge.style.margin="0";
                badge.style.flexShrink="0";
                badge.textContent=pName;
                const nameSpan=document.createElement("span");
                nameSpan.style.fontFamily="SFMono-Regular,Consolas,Menlo,monospace";
                nameSpan.style.fontSize="13px";
                nameSpan.style.flex="1";
                nameSpan.style.minWidth="0";
                nameSpan.style.overflow="hidden";
                nameSpan.style.textOverflow="ellipsis";
                nameSpan.style.whiteSpace="nowrap";
                nameSpan.textContent=credentialDisplayName(c);
                const changeSpan=document.createElement("span");
                changeSpan.style.fontSize="13px";
                changeSpan.style.fontWeight="600";
                changeSpan.style.color="var(--blue)";
                changeSpan.style.flexShrink="0";
                changeSpan.textContent=isFailedQuotaFetch(c) ? textFor("failedQuotaFetch") : priorityChangeText(c);
                row.append(badge,nameSpan,changeSpan);
                if(shouldShowRetry(c)){
                    const retry=document.createElement("button");
                    retry.type="button";
                    retry.className="btn-secondary";
                    retry.textContent=textFor("manualRetry");
                    retry.onclick=function(){retryCredentialQuota(c,retry);};
                    row.appendChild(retry);
                }
                container.appendChild(row);
            });
            document.getElementById("resultModal").hidden=false;
        }
        function isFailedQuotaFetch(c){return c.reason==="failedQuotaFetch";}
        function shouldShowChange(c){return (c.status==="success"&&c.priority_attempted)||(c.status==="success"&&c.disabled_attempted)||c.status === "failed";}
        function shouldShowRetry(c){return c.status === "failed"||isFailedQuotaFetch(c);}
        function retryCredentialQuota(c, button){const provider=String(c.provider||"").toLowerCase();runCredentialPriority("apply",provider?[provider]:[],button,c.retry_auth_index||"",true);}
        function openProviderModal(mode){if(credentialSummaryLoading){return;}pendingRunMode="apply";document.getElementById("providerModal").hidden=false;document.getElementById("modalNotice").textContent="";document.getElementById("modalNotice").className="message-box";const exec=document.getElementById("executePriorityButton");if(exec){exec.textContent=textFor("applyRun");}}
        function executeFromModal(){runCredentialPriority("apply", selectedManualProviders(), document.getElementById("executePriorityButton"));}
        function closeProviderModal(){document.getElementById("providerModal").hidden=true;document.getElementById("modalNotice").textContent="";document.getElementById("modalNotice").className="message-box";}
        function syncManualProviderModeVisibility(){document.getElementById("manualProviderControls").hidden=true;syncAntigravityModelGroupVisibility();}
        function providerSelectionIncludesAntigravity(modeSelectId, selector){const mode=document.getElementById(modeSelectId).value;if(mode==="all"){return providerOptions.some(function(provider){return provider.value==="antigravity";});}return Array.from(document.querySelectorAll(selector)).some(function(input){return input.checked&&(input.dataset.provider==="antigravity"||input.dataset.manualProvider==="antigravity");});}
        function syncAntigravityModelGroupVisibility(){document.getElementById("manualAntigravityModelGroupRow").hidden=!providerSelectionIncludesAntigravity("manualProviderModeSelect","[data-manual-provider]");}
        function showTab(name){if(credentialSummaryLoading&&name!=="overview"){return;}document.getElementById("overviewPanel").hidden=name!=="overview";document.getElementById("historyPanel").hidden=name!=="history";document.getElementById("helpPanel").hidden=name!=="help";for(const tab of document.querySelectorAll(".tab")){tab.classList.toggle("active",tab.dataset.tab===name);}if(name==="history"){renderRunHistory();}}
        function parseRetryDelaySeconds(message){const match=String(message).match(/Try again in (?:(\d+)m)?(?:(\d+)s)?/i);if(!match){return 0;}return Number(match[1]||0)*60+Number(match[2]||0);}
        function handleManagementError(err, login){const message=String(err&&err.message?err.message:err);const seconds=parseRetryDelaySeconds(message);if(seconds>0||message.includes("IP banned due to too many failed attempts")){startBanCountdown(seconds||1800,message);}if(login){setLoginMessage(message);}else{setMessage(message);if(!document.getElementById("providerModal").hidden){showModalNotice(message,"error");}}showToast(message,"error");}
        function startBanCountdown(seconds, raw){const box=document.getElementById("banCountdown");box.hidden=false;let remaining=seconds;const render=function(){const minutes=Math.floor(remaining/60);const rest=remaining%60;box.textContent=raw+"\n"+textFor("banHint")+"\n"+minutes+"m"+rest+"s";remaining=Math.max(0,remaining-1);};render();window.clearInterval(window.credentialPriorityBanTimer);window.credentialPriorityBanTimer=window.setInterval(render,1000);}
        function showToast(message, type){const toast=document.createElement("div");toast.setAttribute("role","alert");const cls=type==="success"?"bg-green-100":type==="error"?"bg-red-100":type==="warning"?"bg-yellow-100":"bg-blue-100";toast.className="toast-alert "+cls;toast.textContent=message;document.getElementById("toastRoot").appendChild(toast);window.setTimeout(function(){toast.remove();},2500);}
        function showModalNotice(message, type){const box=document.getElementById("modalNotice");box.className="message-box "+(type==="error"?"error":"warn");box.textContent=message;}
        function switchLanguage(){setLanguage(activeLanguage==="zh-CN"?"en-US":"zh-CN");}
        function setLanguage(language){activeLanguage=language;applyLanguage();updateConfigSummary(currentConfig);updateCustomSelects();refreshProviderLocalizedControls();renderRunHistory();}
        function refreshProviderLocalizedControls(){renderProviderMultiSelect("manual");renderSelectedProviderTags("manual");}
        function applyLanguage(){const messages=translations[activeLanguage];for(const element of document.querySelectorAll("[data-i18n]")){const key=element.getAttribute("data-i18n");if(messages[key]){element.textContent=messages[key];}}document.documentElement.lang=activeLanguage;}
        function initCustomSelect(selectId) {
            const select = document.getElementById(selectId);
            if (!select) return;
            let wrapper = document.getElementById(selectId + "-custom-wrapper");
            if (wrapper) { wrapper.remove(); }
            wrapper = document.createElement("div");
            wrapper.id = selectId + "-custom-wrapper";
            wrapper.className = "custom-select-container";
            const trigger = document.createElement("div");
            trigger.className = "custom-select-trigger";
            const valueSpan = document.createElement("span");
            valueSpan.className = "custom-select-value";
            const selectedOpt = select.options[select.selectedIndex];
            valueSpan.textContent = selectedOpt ? selectedOpt.textContent : "";
            const arrow = document.createElement("span");
            arrow.className = "custom-select-arrow";
            arrow.innerHTML = '<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M6 9l6 6 6-6"></path></svg>';
            trigger.appendChild(valueSpan);
            trigger.appendChild(arrow);
            wrapper.appendChild(trigger);
            const optionsContainer = document.createElement("div");
            optionsContainer.className = "custom-select-options";
            optionsContainer.hidden = true;
            Array.from(select.options).forEach(function(opt) {
                const optionEl = document.createElement("div");
                optionEl.className = "custom-select-option";
                if (opt.value === select.value) { optionEl.classList.add("active"); }
                optionEl.textContent = opt.textContent;
                optionEl.dataset.value = opt.value;
                optionEl.addEventListener("click", function(e) {
                    e.stopPropagation();
                    select.value = opt.value;
                    valueSpan.textContent = opt.textContent;
                    Array.from(optionsContainer.children).forEach(function(child) {
                        child.classList.toggle("active", child.dataset.value === opt.value);
                    });
                    optionsContainer.hidden = true;
                    wrapper.classList.remove("open");
                    select.dispatchEvent(new Event("change"));
                });
                optionsContainer.appendChild(optionEl);
            });
            wrapper.appendChild(optionsContainer);
            select.parentNode.insertBefore(wrapper, select.nextSibling);
            select.style.display = "none";
            trigger.addEventListener("click", function(e) {
                e.stopPropagation();
                const wasHidden = optionsContainer.hidden;
                document.querySelectorAll(".custom-select-options").forEach(function(cont) {
                    cont.hidden = true;
                    cont.parentElement.classList.remove("open");
                });
                optionsContainer.hidden = !wasHidden;
                if (wasHidden) { wrapper.classList.add("open"); } else { wrapper.classList.remove("open"); }
            });
        }
        function updateCustomSelects() { initCustomSelect("manualAntigravityModelGroupSelect"); }
        document.addEventListener("click", function() {
            closeProviderDropdowns();
            document.querySelectorAll(".custom-select-options").forEach(function(cont) {
                cont.hidden = true;
                cont.parentElement.classList.remove("open");
            });
        });
        applyLanguage();
        updateCustomSelects();
    </script>
</body>
</html>
`
