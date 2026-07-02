package management

import "html/template"

// StatusHTML 是渲染侧边栏资源页的 HTML 模版文本。
// 页面只在浏览器内暂存管理密钥，并通过 fetch 调用 CPA management API。
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
        .version-badge { display:inline-flex; align-items:center; min-height:24px; border-radius:999px; padding:3px 9px; background:#eff6ff; color:#1d4ed8; font-size:12px; font-weight:750; letter-spacing:0; }
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
        .switch-line { display:flex; align-items:center; gap:8px; }
        #configPanel { width:100%; max-width:100%; }
        #providerModeSelect { width:180px; padding-right:32px; }
        #manualProviderModeSelect { border-radius:12px; }
        #configSummary { font-size:14px; white-space:pre-wrap; }
        .switch { display:inline-flex; align-items:center; gap:12px; }
        .switch input { position:absolute; opacity:0; width:0; height:0; }
        .slider { position:relative; width:54px; height:30px; border-radius:999px; background:#d1d5db; transition:.2s; }
        .slider:before { content:""; position:absolute; width:24px; height:24px; left:3px; top:3px; border-radius:50%; background:#fff; box-shadow:0 1px 4px rgba(0,0,0,.2); transition:.2s; }
        .switch input:checked + .slider { background:#22c55e; }
        .switch input:checked + .slider:before { transform:translateX(24px); }
        .toast-root { position:relative; z-index:80; display:grid; gap:10px; width:320px; max-width:calc(100vw - 120px); }
        .toast-alert { border-left:4px solid; border-radius:12px; padding:10px 12px; display:flex; align-items:center; gap:10px; box-shadow:0 18px 48px rgba(15,23,42,.12); transition:opacity .3s ease, transform .3s ease; }
        .bg-green-100 { background:#dcfce7; border-color:#22c55e; color:#14532d; }
        .bg-blue-100 { background:#dbeafe; border-color:#3b82f6; color:#1e3a8a; }
        .bg-yellow-100 { background:#fef9c3; border-color:#eab308; color:#713f12; }
        .bg-red-100 { background:#fee2e2; border-color:#ef4444; color:#7f1d1d; }
        .modal-backdrop { position:fixed; inset:0; display:grid; place-items:center; background:rgba(15,23,42,.42); padding:20px; }
        .modal { width:min(560px,100%); background:#fff; border-radius:18px; padding:24px; box-shadow:0 24px 80px rgba(15,23,42,.24); }
        .language-shell { position:relative; }
        .language-menu-button { min-width:44px; padding:10px; display:inline-flex; align-items:center; justify-content:center; background:#fff; border-color:#d1d5db; color:#374151; }
        .config-detail-grid { display:grid; grid-template-columns:repeat(2,minmax(0,1fr)); gap:16px; margin-top:16px; }
        .auto-priority-compact-row { display:grid; grid-template-columns:repeat(3,minmax(0,1fr)); gap:12px; align-items:start; margin-top:16px; }
        .auto-priority-compact-row label { margin:0; }
        .auto-priority-compact-row input { width:100%; }
        .auto-priority-compact-row .hint { margin-top:6px; }
        .auto-priority-field-hint { white-space:nowrap; }
        .config-control-row { display:flex; align-items:flex-end; justify-content:flex-end; gap:16px; }
        .config-control-row label { margin-bottom:8px; }
        .config-control-row select { width:260px; }
        .form-actions { display:flex; justify-content:flex-end; margin-top:16px; }
        .checkbox-list { display:grid; gap:10px; margin-top:12px; }
        .checkbox-wrapper-46 input[type="checkbox"] { display:none; visibility:hidden; }
        .checkbox-wrapper-46 .cbx { margin:auto; user-select:none; cursor:pointer; display:inline-flex; align-items:center; justify-content:space-between; width:100%; gap:10px; }
        .checkbox-wrapper-46 .cbx .cbx-box { position:relative; width:18px; height:18px; border-radius:3px; transform:scale(1); border:1px solid #9098a9; transition:all .2s ease; flex-shrink:0; }
        .checkbox-wrapper-46 .cbx .cbx-box svg { position:absolute; top:3px; left:2px; fill:none; stroke:#fff; stroke-width:2; stroke-linecap:round; stroke-linejoin:round; stroke-dasharray:16px; stroke-dashoffset:16px; transition:all .3s ease; }
        .checkbox-wrapper-46 .inp-cbx:checked + .cbx .cbx-box { background:#506eec; border-color:#506eec; animation:wave-46 .4s ease; }
        .checkbox-wrapper-46 .inp-cbx:checked + .cbx .cbx-box svg { stroke-dashoffset:0; }
        @keyframes wave-46 { 50% { transform:scale(.9); } }
        button:disabled { opacity: 0.6; cursor: not-allowed; background: #94a3b8 !important; color: #f1f5f9 !important; border-color: #cbd5e1 !important; }
        .custom-select-container { position: relative; width: 100%; }
        .custom-select-trigger {
            display: flex;
            align-items: center;
            justify-content: space-between;
            min-height: 44px;
            border: 1px solid #d1d5db;
            border-radius: 12px;
            padding: 10px 12px;
            background: #fff;
            color: var(--text);
            cursor: pointer;
            user-select: none;
            font-size: 14px;
            width: 100%;
        }
        .custom-select-trigger:hover { border-color: #9ca3af; }
        .custom-select-arrow { transition: transform 0.2s; color: var(--muted); display: flex; align-items: center; }
        .custom-select-container.open .custom-select-arrow { transform: rotate(180deg); }
        .custom-select-options {
            position: absolute;
            top: calc(100% + 4px);
            left: 0;
            width: 100%;
            background: #fff;
            border: 1px solid #e5e7eb;
            border-radius: 12px;
            box-shadow: 0 10px 25px -5px rgba(0, 0, 0, 0.1), 0 8px 10px -6px rgba(0, 0, 0, 0.1);
            z-index: 99;
            overflow: hidden;
            padding: 4px;
        }
        .custom-select-option {
            padding: 10px 12px;
            cursor: pointer;
            user-select: none;
            font-size: 14px;
            border-radius: 8px;
            color: #374151;
            transition: background 0.15s, color 0.15s;
        }
        .custom-select-option:hover { background: #f3f4f6; color: var(--text); }
        .custom-select-option.active { background: #e0f2fe; color: #0369a1; font-weight: 600; }
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
        .provider-selection-grid { display:grid; grid-template-columns:minmax(0,.5fr) minmax(0,1.5fr); gap:20px; align-items:start; }
        .provider-selector-column,.selected-provider-column { min-width:0; }
        .config-layout-grid { display:grid; grid-template-columns:minmax(0,1fr) minmax(360px,.9fr); gap:20px; align-items:start; }
        .config-main-card { display:flex; flex-direction:column; gap:20px; }
        .priority-rules-card { display:grid; gap:14px; background:var(--soft); }
        .priority-rules-header { display:flex; align-items:flex-start; justify-content:space-between; gap:14px; border-bottom:1px solid var(--line); padding-bottom:14px; }
        .priority-rules-title { display:grid; gap:6px; }
        .priority-rules-summary { display:flex; align-items:center; gap:8px; flex-wrap:wrap; }
        .priority-rules-actions { display:flex; justify-content:flex-end; gap:10px; flex-wrap:wrap; }
        .priority-rules-summary-card { grid-column:1/-1; }
        .priority-rule-summary-list { display:grid; gap:10px; margin-top:10px; font-size:13px; }
        .priority-rule-summary-provider { display:grid; gap:4px; padding:10px 12px; border:1px solid rgba(17,24,39,.05); border-radius:12px; background:#fff; }
        .priority-rule-summary-provider strong { font-size:14px; }
        .priority-rule-summary-provider span { color:var(--muted); line-height:1.55; }
        .priority-rules-note { font-size:12px; color:var(--muted); font-weight:500; }
        .priority-rules-expand-hint { margin-left:auto; display:inline-flex; align-items:center; gap:5px; color:var(--muted); font-size:12px; font-weight:650; }
        .priority-rules-arrow { transition:transform .2s ease; }
        details[open] .priority-rules-arrow { transform:rotate(180deg); }
        .priority-rules-provider { display:grid; gap:8px; margin-top:12px; padding:12px; border-radius:12px; background:#fff; border:1px solid rgba(17,24,39,.05); }
        .priority-rules-provider strong { font-size:14px; }
        .priority-rules-provider ul { margin:0; padding-left:18px; color:var(--muted); line-height:1.6; }
        .priority-rule-grid { display:grid; grid-template-columns:repeat(2,minmax(0,1fr)); gap:12px; }
        .priority-rule-grid label { margin:0; }
        .priority-rule-toggle { display:flex; align-items:center; justify-content:space-between; gap:10px; min-height:44px; padding:10px 12px; border:1px solid var(--line); border-radius:12px; background:#f8fafc; }
        .priority-rule-switch { flex-shrink:0; }
        .priority-rules-card.rules-disabled .priority-rule-field { opacity:.55; }
        [hidden] { display:none !important; }
        @media (max-width:900px) { .config-layout-grid{grid-template-columns:1fr} }
        @media (max-width:720px) { body{padding:12px}.container{padding:20px}.grid,.config-detail-grid,.auto-priority-compact-row,.provider-selection-grid,.priority-rule-grid{grid-template-columns:1fr}.auto-priority-compact-row input{width:100%}.topbar{align-items:center}.topbar-actions{flex-wrap:wrap}.toast-root{width:100%; max-width:100%; order:2}.tabs{display:grid;grid-template-columns:1fr 1fr}.switch-line{display:block}.config-control-row{display:block}.config-control-row select{width:100%}.config-selection-row{flex-direction:column}.modal-selection-row{flex-direction:column} #configPanel{max-width:100%} #providerModeSelect{width:100%} }
    </style>
</head>
<body>
    <div class="container">
        <div class="topbar">
            <h1><span data-i18n="pageTitle">凭证优先级管理</span><span class="version-badge">v1.0.2</span></h1>
            <div class="topbar-actions">
                <div id="toastRoot" class="toast-root" aria-live="polite"></div>
                <div class="language-shell">
                    <button id="languageMenuButton" type="button" class="language-menu-button btn-secondary" onclick="switchLanguage()" aria-label="Switch language">
                        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true" focusable="false"><circle cx="12" cy="12" r="10"></circle><path d="M2 12h20"></path><path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"></path></svg>
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
                <button type="button" class="tab" data-tab="config" onclick="showTab('config')" data-i18n="configTab">配置</button>
            </nav>

            <section id="overviewPanel">
                <div class="grid">
                    <div class="card"><p data-i18n="totalCredentials">总凭证数</p><div id="totalCredentialValue" class="metric">{{.TotalCredentials}}</div></div>
                    <div class="card"><p data-i18n="providerCredentialCount">提供商凭证数量</p><div id="providerCounts" class="provider-counts"></div></div>
                </div>
                <div class="section card">
                    <h2 data-i18n="configDetails">配置详情</h2>
                    <div class="config-detail-grid">
                        <div class="card"><p data-i18n="autoPriorityEnabled">自动优先级排序</p><div id="autoApplySummary" class="metric">-</div></div>
                        <div class="card"><p data-i18n="sortingProviderSelection">排序提供商选择</p><div id="providerScopeSummary" class="metric">-</div></div>
                        <div class="card priority-rules-summary-card"><p data-i18n="priorityRulesSummaryTitle">优先级排序规则</p><div id="priorityRulesSummary" class="priority-rule-summary-list">-</div></div>
                    </div>
                    <div style="display:flex; justify-content:flex-end; margin-top:16px">
                        <button id="openProviderModalButton" type="button" class="btn-primary" onclick="openProviderModal()" data-i18n="runPrioritySort">执行优先级排序</button>
                    </div>
                </div>
            </section>

            <section id="configPanel" hidden>
                <div class="config-layout-grid">
                <form id="configForm" class="card config-main-card">
                    <div class="switch-line" style="display:flex; justify-content:space-between; align-items:center; border-bottom:1px solid var(--line); padding-bottom:16px;">
                        <span data-i18n="autoPriorityEnabled" style="font-weight:600;">自动优先级排序</span>
                        <label class="switch"><input id="autoPriorityEnabled" type="checkbox" onchange="syncAutoPriorityVisibility()"><span class="slider"></span></label>
                    </div>
                    <div id="providerSection" class="section" style="margin-top:0; display:flex; flex-direction:column; gap:12px;">
                        <div class="hint future-providers-hint" data-i18n="futureProvidersHint" style="margin-top:4px; margin-bottom:8px; font-size:12px; color:var(--muted);">目前仅支持 Antigravity 和 Codex 凭证，排序规则已按提供商独立分开。后续可能支持：Anthropic、Kimi、xAI、Vertex JSON。</div>
                        <label for="providerModeSelect" data-i18n="sortingProviderSelection" style="margin-bottom:0; font-weight:600;">排序提供商选择</label>
                        <div class="config-selection-row provider-selection-grid">
                            <div class="provider-selector-column">
                                <select id="providerModeSelect" onchange="syncProviderModeVisibility()" style="display:none;"><option value="all" data-i18n="providerAll">全部</option><option value="antigravity">Antigravity</option><option value="codex">Codex</option></select>
                                <div id="customProviderControls" class="checkbox-list" hidden style="margin-top:0;"></div>
                                <div id="configProviderMultiSelect" class="provider-multi-select" data-provider-multi-select="config"></div>
                            </div>
                            <div class="selected-provider-column">
                                <div id="configSelectedProviderTags" class="selected-provider-tags"></div>
                            </div>
                        </div>
                        <div id="configAntigravityModelGroupRow" class="config-selection-row" style="display:flex; gap:20px; align-items:flex-start;">
                            <div style="flex:1;">
                                <label for="configAntigravityModelGroupSelect" data-i18n="antigravityModelGroup" style="margin-bottom:8px; font-weight:600;">Antigravity 模型组</label>
                                <select id="configAntigravityModelGroupSelect"><option value="gemini" data-i18n="geminiModelGroup">Gemini 模型</option><option value="claude_gpt" data-i18n="claudeGPTModelGroup">Claude 和 GPT 模型</option></select>
                            </div>
                        </div>
                        <div class="auto-priority-compact-row">
                            <label><span data-i18n="autoPriorityIntervalLabel">自动排序间隔</span><input id="autoPriorityInterval" type="number" min="1" step="1" value="15"></label>
                            <label><span data-i18n="autoPriorityImmediateProbeLimitLabel">凭证上限</span><input id="autoPriorityImmediateProbeLimit" type="number" min="1" value="30"><span class="hint auto-priority-field-hint" data-i18n="autoPriorityImmediateProbeLimitHint">超过该值的凭证将分批探测</span></label>
                            <label><span data-i18n="autoPriorityActiveGroupSizeLabel">分批凭证数</span><input id="autoPriorityActiveGroupSize" type="number" min="1" value="10"></label>
                        </div>
                        <template id="providerCheckboxTemplate"><div class="checkbox-wrapper-46"><input type="checkbox" id="cbx-46" class="inp-cbx" data-provider="template" data-manual-provider="template" /><label for="cbx-46" class="cbx"><span class="cbx-text">Checkbox</span><span class="cbx-box"><svg viewBox="0 0 12 10" height="10px" width="12px"><polyline points="1.5 6 4.5 9 10.5 1"></polyline></svg></span></label></div></template>
                    </div>
                    <div class="form-actions" style="display:flex; justify-content:flex-end; border-top:1px solid var(--line); padding-top:16px; margin-top:8px;"><button id="saveConfigButton" type="button" class="btn-primary" onclick="saveConfig()" data-i18n="saveConfig">保存配置</button></div>
                </form>
                    <div id="configPriorityRulesCard" class="card priority-rules-card rules-disabled" data-priority-rules="provider-separated">
                        <div class="priority-rules-header">
                            <div class="priority-rules-title"><span data-i18n="priorityRulesTitle" style="font-weight:700;">优先级排序规则</span><span class="priority-rules-note" data-i18n="providerIndependentRulesNote">不同提供商的排序规则是独立的</span></div>
                            <label class="switch"><input id="priorityRulesEnabled" name="priority_rules.enabled" type="checkbox" onchange="syncPriorityRulesEnabled(true)"><span class="slider"></span></label>
                        </div>
                        <div class="priority-rules-actions"><button id="resetPriorityRulesButton" type="button" class="btn-secondary" onclick="restoreDefaultPriorityRules()" data-i18n="restoreDefaultRules">恢复默认规则</button><button id="savePriorityRulesButton" type="button" class="btn-primary" onclick="savePriorityRules()" data-i18n="savePriorityRules">保存规则</button></div>
                        <details class="priority-rules-provider" data-rule-provider="antigravity">
                            <summary class="priority-rules-summary"><strong>Antigravity</strong><span class="priority-rules-expand-hint"><span data-i18n="clickToExpand">点击展开</span><svg class="priority-rules-arrow" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M6 9l6 6 6-6"></path></svg></span></summary>
                            <ul><li data-i18n="antigravityRuleFresh">只排序本轮成功获取到所选模型组配额的 Antigravity 凭证。</li><li data-i18n="antigravityRuleWindow">优先级从 100 开始递减；重置时间更早的可用凭证排在前面。</li><li data-i18n="antigravityRuleDepleted">配额获取失败或剩余额度不可用时保留原优先级，不自动禁用。</li></ul>
                            <div class="priority-rule-grid">
                                <label class="priority-rule-field"><span data-i18n="priorityRuleStartPriority">可用凭证起始优先级</span><input name="priority_rules.antigravity.start_priority" type="number" min="1" value="100"></label>
                            </div>
                        </details>
                        <details class="priority-rules-provider" data-rule-provider="codex">
                            <summary class="priority-rules-summary"><strong>Codex</strong><span class="priority-rules-expand-hint"><span data-i18n="clickToExpand">点击展开</span><svg class="priority-rules-arrow" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M6 9l6 6 6-6"></path></svg></span></summary>
                            <ul><li data-i18n="codexRuleFresh">只排序本轮成功获取到 Codex 用量数据的凭证。</li><li data-i18n="codexRuleFree">Free 凭证额度耗尽时写入 priority=-1 并禁用；仍有额度时按重置时间排序。</li><li data-i18n="codexRulePaid">Plus、Pro、Team 凭证有可用额度时优先于 Free，并按付费窗口重置时间排序；额度耗尽不自动禁用。</li></ul>
                            <div class="priority-rule-grid">
                                <label class="priority-rule-field"><span data-i18n="priorityRuleStartPriority">可用凭证起始优先级</span><input name="priority_rules.codex.start_priority" type="number" min="1" value="100"></label>
                                <label class="priority-rule-field"><span data-i18n="priorityRuleFreeDepletedPriority">免费额度为 0 时优先级</span><input name="priority_rules.codex.free_depleted_priority" type="number" value="-1"></label>
                                <label class="priority-rule-toggle priority-rule-field"><span data-i18n="priorityRuleFreeDepletedDisabled">免费额度为 0 时禁用</span><span class="switch priority-rule-switch"><input name="priority_rules.codex.free_depleted_disabled" type="checkbox" checked><span class="slider"></span></span></label>
                                <label class="priority-rule-toggle priority-rule-field"><span data-i18n="priorityRulePaidKeepsEnabled">Plus、Pro、Team 耗尽不自动禁用</span><span class="switch priority-rule-switch"><input name="priority_rules.codex.paid_depleted_keeps_enabled" type="checkbox" checked><span class="slider"></span></span></label>
                            </div>
                        </details>
                    </div>
                </div>
            </section>
            <div id="banCountdown" class="error message-box" hidden></div>
        </main>
    </div>

    <div id="providerModal" class="modal-backdrop" hidden>
        <div class="modal" role="dialog" aria-modal="true" aria-labelledby="providerModalTitle">
            <h2 id="providerModalTitle" data-i18n="selectProviderTitle">选择要排序的提供商</h2>
            <p data-i18n="manualRunHint" style="font-size:13px; color:var(--muted); margin-bottom:16px;">该按钮为手动执行，如需自动排序请在配置页中开启自动排序。</p>
            <p class="hint future-providers-hint" data-i18n="futureProvidersHint" style="margin-bottom:16px; font-size:12px; color:var(--muted);">目前仅支持 Antigravity 和 Codex 凭证，排序规则已按提供商独立分开。后续可能支持：Anthropic、Kimi、xAI、Vertex JSON。</p>
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
                <button id="executePriorityButton" type="button" class="btn-primary" onclick='runCredentialPriority("apply", selectedManualProviders(), this)' data-i18n="applyRun">执行</button>
                <button type="button" class="btn-danger" onclick="closeProviderModal()" data-i18n="cancel">关闭</button>
            </div>
        </div>
    </div>

    <div id="resultModal" class="modal-backdrop" hidden>
        <div class="modal" role="dialog" aria-modal="true" aria-labelledby="resultModalTitle" style="width: min(680px, 95%); max-width: 680px;">
            <h2 id="resultModalTitle" data-i18n="runCompleted" style="margin-bottom: 16px;">手动任务已完成</h2>
            <div id="resultDetailsContainer" style="max-height: 320px; overflow-y: auto; background: #f8fafc; border: 1px solid #e5e7eb; border-radius: 12px; padding: 16px; display: flex; flex-direction: column; gap: 10px;">
            </div>
            <div style="display:flex; justify-content:flex-end; margin-top:20px">
                <button type="button" class="btn-secondary" onclick="closeResultModal()" data-i18n="cancel" style="min-width: 100px;">关闭</button>
            </div>
        </div>
    </div>

    <div id="confirmModal" class="modal-backdrop" hidden>
        <div class="modal" role="dialog" aria-modal="true" aria-labelledby="confirmModalTitle">
            <h2 id="confirmModalTitle" data-i18n="confirmActionTitle">请确认操作</h2>
            <p id="confirmModalMessage" style="font-size:14px; color:var(--muted); margin-bottom:18px;"></p>
            <div style="display:flex; justify-content:flex-end; gap:10px; flex-wrap:wrap;">
                <button type="button" class="btn-secondary" onclick="closeConfirmModal(false)" data-i18n="cancel">关闭</button>
                <button type="button" class="btn-primary" onclick="closeConfirmModal(true)" data-i18n="confirm">确认</button>
            </div>
        </div>
    </div>

    <script>
        const CONFIG_PATH="/v0/management/plugins/credential-priority/config";
        const AUTH_FILES_PATH="/v0/management/auth-files";
        const translations={
            "zh-CN":{pageTitle:"凭证优先级管理",managementKeySection:"CPA管理密钥验证",verifyKey:"验证",overviewTab:"概览",configTab:"配置",totalCredentials:"总凭证数",providerCredentialCount:"提供商凭证数量",configDetails:"配置详情",manualRunTitle:"手动优先级排序",manualRunHint:"该按钮为手动执行，如需自动排序请在配置页中开启自动排序。",futureProvidersHint:"目前仅支持 Antigravity 和 Codex 凭证，排序规则已按提供商独立分开。后续可能支持：Anthropic、Kimi、xAI、Vertex JSON。",priorityRulesTitle:"优先级排序规则",priorityRulesSummaryTitle:"优先级排序规则",providerIndependentRulesNote:"不同提供商的排序规则是独立的",clickToExpand:"点击展开",savePriorityRules:"保存规则",restoreDefaultRules:"恢复默认规则",restoreDefaultRulesConfirm:"是否恢复默认规则？",priorityRulesEnableConfirm:"是否需要启用优先级排序规则？自定义存在风险，无特殊需求建议不启用",confirmActionTitle:"请确认操作",confirm:"确认",priorityRulesRestored:"已恢复默认规则",priorityRuleStartPriority:"可用凭证起始优先级",priorityRuleFreshReadyOnly:"只使用本轮最新且可用的探测证据",priorityRuleProviderIndependent:"提供商独立排序",priorityRuleAntigravityFailureKeepsState:"配额获取失败时保留当前状态",priorityRuleFreeDepletedPriority:"免费额度为 0 时优先级",priorityRuleFreeDepletedDisabled:"免费额度为 0 时禁用",priorityRulePaidKeepsEnabled:"Plus、Pro、Team 耗尽不自动禁用",antigravityRuleFresh:"只排序本轮成功获取到所选模型组配额的 Antigravity 凭证。",antigravityRuleWindow:"优先级从 100 开始递减；重置时间更早的可用凭证排在前面。",antigravityRuleDepleted:"配额获取失败或剩余额度不可用时保留原优先级，不自动禁用。",codexRuleFresh:"只排序本轮成功获取到 Codex 用量数据的凭证。",codexRuleFree:"免费凭证额度耗尽时写入优先级=-1 并禁用；仍有额度时按重置时间排序。",codexRulePaid:"Plus、Pro、Team 凭证有可用额度时优先于免费凭证，并按付费窗口重置时间排序；额度耗尽不自动禁用。",failedQuotaFetch:"获取配额失败",manualRetry:"重试",runPrioritySort:"执行优先级排序",overviewLoadingText:"加载凭证统计中...",autoPriorityEnabled:"自动优先级排序",autoPriorityIntervalLabel:"自动排序间隔",autoPriorityImmediateProbeLimitLabel:"凭证上限",autoPriorityImmediateProbeLimitHint:"超过该值的凭证将分批探测",autoPriorityActiveGroupSizeLabel:"分批凭证数",sortingProviderSelection:"排序提供商选择",selectedProviders:"已选提供商",antigravityModelGroup:"Antigravity 模型组",geminiModelGroup:"Gemini 模型",claudeGPTModelGroup:"Claude 和 GPT 模型",providerAll:"全部",providerCustom:"自定义",saveConfig:"保存配置",selectProviderTitle:"选择要排序的提供商",allProviders:"全部提供商",autoStatusOn:"已开启",autoStatusOff:"已关闭",applyRun:"执行",cancel:"关闭",running:"执行中...",priorityUnset:"未设置",managementKeyRequired:"请先填写管理密钥。",configLoaded:"配置已读取。",configSaved:"配置已保存。",runCompleted:"手动任务已完成",banHint:"管理密钥多次错误已触发风控，可等待倒计时结束，或可重启 CPA / CLIProxyAPI 后再试。",banExample:"IP banned due to too many failed attempts. Try again in 29m43s",noChanges:"本次没有优先级变化"},
            "en-US":{pageTitle:"Credential Priority",managementKeySection:"CPA Management Key Verification",verifyKey:"Verify",overviewTab:"Overview",configTab:"Config",totalCredentials:"Total Credentials",providerCredentialCount:"Provider Credential Count",configDetails:"Config Details",manualRunTitle:"Manual Priority Sort",manualRunHint:"This button runs manually. Enable automatic sorting in the config tab if needed.",futureProvidersHint:"Only Antigravity and Codex credentials are currently supported. Sorting rules are separated by provider. Future support may include Anthropic, Kimi, xAI, and Vertex JSON.",priorityRulesTitle:"Priority sorting rules",priorityRulesSummaryTitle:"Priority sorting rules",providerIndependentRulesNote:"Provider sorting rules are independent",clickToExpand:"Click to expand",savePriorityRules:"Save Rules",restoreDefaultRules:"Restore default rules",restoreDefaultRulesConfirm:"Restore default rules?",priorityRulesEnableConfirm:"Enable priority sorting rules? Custom rules are risky and are not recommended unless you have a specific need.",confirmActionTitle:"Confirm action",confirm:"Confirm",priorityRulesRestored:"Default rules restored",priorityRuleStartPriority:"Available credential start priority",priorityRuleFreshReadyOnly:"Use only fresh and ready evidence from this run",priorityRuleProviderIndependent:"Provider-independent sorting",priorityRuleAntigravityFailureKeepsState:"Keep current state when quota fetch fails",priorityRuleFreeDepletedPriority:"Free depleted priority",priorityRuleFreeDepletedDisabled:"Disable Free when quota is 0",priorityRulePaidKeepsEnabled:"Plus, Pro, and Team depletion does not disable",antigravityRuleFresh:"Sorts only Antigravity credentials whose selected model-group quota was fetched in this run.",antigravityRuleWindow:"Priorities start at 100 and decrease; available credentials with earlier reset time rank first.",antigravityRuleDepleted:"Failed quota fetches or unavailable remaining quota keep the current priority and are not disabled automatically.",codexRuleFresh:"Sorts only credentials whose Codex usage data was fetched in this run.",codexRuleFree:"Depleted Free credentials are written as priority=-1 and disabled; available Free credentials sort by reset time.",codexRulePaid:"Plus, Pro, and Team credentials with quota rank ahead of Free and sort by paid-window reset time; depletion does not disable them automatically.",failedQuotaFetch:"Failed to fetch quota",manualRetry:"Retry",runPrioritySort:"Run Priority Sort",overviewLoadingText:"Loading credential summary...",autoPriorityEnabled:"Automatic Priority Sorting",autoPriorityIntervalLabel:"Auto sorting interval",autoPriorityImmediateProbeLimitLabel:"Credential limit",autoPriorityImmediateProbeLimitHint:"Credentials above this value are probed in batches",autoPriorityActiveGroupSizeLabel:"Batch credential count",sortingProviderSelection:"Sorting provider selection",selectedProviders:"Selected providers",antigravityModelGroup:"Antigravity model group",geminiModelGroup:"Gemini models",claudeGPTModelGroup:"Claude and GPT models",providerAll:"All",providerCustom:"Custom",saveConfig:"Save Config",selectProviderTitle:"Select Providers",allProviders:"All Providers",autoStatusOn:"ON",autoStatusOff:"OFF",applyRun:"Execute",cancel:"Close",running:"Running...",priorityUnset:"Unset",managementKeyRequired:"Please enter the management key first.",configLoaded:"Config loaded.",configSaved:"Config saved.",runCompleted:"Manual task completed",banHint:"Too many wrong keys triggered risk control. Wait for the countdown, or restart CPA / CLIProxyAPI and retry.",banExample:"IP banned due to too many failed attempts. Try again in 29m43s",noChanges:"No priority changes this time"}
        };
        let activeLanguage="zh-CN";
        let providerOptions=[];
        let currentConfig={provider_scope:"all",selected_providers:[]};
        let currentResult=null;
        let credentialSummaryLoading=false;
        const saveConfigCooldownMs=3000;
        let saveConfigCoolingDown=false;
        const defaultPriorityRuleConfig={enabled:false,antigravity:{start_priority:100},codex:{start_priority:100,free_depleted_priority:-1,free_depleted_disabled:true,paid_depleted_keeps_enabled:true}};
        const priorityRulesRestoreCooldownMs=3000;
        let priorityRulesRestoreCoolingDown=false;
        let confirmModalCallback=null;
        function getProviderDisplayName(provider) {
            const lower = String(provider || "").trim().toLowerCase();
            const names = {
                "antigravity": "Antigravity",
                "codex": "Codex",
                "anthropic": "Anthropic",
                "kimi": "Kimi",
                "xai": "xAI",
                "x-ai": "xAI",
                "vertex": "Vertex JSON",
                "vertex-json": "Vertex JSON",
                "vertex_json": "Vertex JSON"
            };
            return names[lower] || provider;
        }
        function textFor(key){return (translations[activeLanguage]&&translations[activeLanguage][key])||translations["zh-CN"][key]||key;}
        function setMessage(value){if(value){showToast(value,"info");}}
        function setLoginMessage(value){document.getElementById("loginMessage").textContent=value;}
        function requireManagementKey(){const key=document.getElementById("managementKey").value.trim();if(!key){setLoginMessage(textFor("managementKeyRequired"));showToast(textFor("managementKeyRequired"),"warning");return "";}return key;}
        function authHeaders(key){return {"Authorization":"Bearer "+key,"Content-Type":"application/json"};}
        async function managementFetch(path, options){const key=requireManagementKey();if(!key){return null;}const response=await fetch(path,{...(options||{}),headers:{...authHeaders(key),...((options&&options.headers)||{})}});const text=await response.text();if(!response.ok){throw new Error(text||response.statusText);}return text?JSON.parse(text):{};}
        async function loadConfig(options){const config=await managementFetch(CONFIG_PATH,{method:"GET"});if(!config){return null;}currentConfig=config;fillConfigForm(config);updateConfigSummary(config);if(!options||!options.silent){showToast(textFor("configLoaded"),"success");}return config;}
        async function verifyManagementKey(){try{const config=await loadConfig({silent:true});if(!config){return;}document.getElementById("loginGate").hidden=true;document.getElementById("appShell").hidden=false;showToast(textFor("configLoaded"),"success");await loadCredentialSummary();}catch(err){handleManagementError(err,true);}}
        function readSelectedProviders(selector){return Array.from(document.querySelectorAll(selector)).filter(function(item){return item.checked;}).map(function(item){return item.dataset.provider||item.dataset.manualProvider;});}
        function readProviderSelection(selectId, selector){const mode=document.getElementById(selectId).value;if(mode==="all"){return [];}return readSelectedProviders(selector);}
        function displayIntervalMinutes(value){const text=String(value||"15m").trim();if(/^\d+$/.test(text)){return text;}const match=text.match(/^(\d+)m$/);return match?match[1]:text;}
        function configIntervalValue(value){const text=String(value||"15").trim();return /^\d+$/.test(text)?text+"m":text;}
        function readConfigForm(){const selected=readProviderSelection("providerModeSelect","[data-provider]");return {auto_apply:document.getElementById("autoPriorityEnabled").checked,interval:configIntervalValue(document.getElementById("autoPriorityInterval").value),immediate_probe_limit:Number(document.getElementById("autoPriorityImmediateProbeLimit").value),active_group_size:Number(document.getElementById("autoPriorityActiveGroupSize").value),provider_scope:selected.length>0?"selected":"all",selected_providers:selected,antigravity_model_group:document.getElementById("configAntigravityModelGroupSelect").value,priority_rules:readPriorityRuleConfig()};}
        function fillConfigForm(config){document.getElementById("autoPriorityEnabled").checked=config.auto_apply===true;document.getElementById("autoPriorityInterval").value=displayIntervalMinutes(config.interval||"15m");document.getElementById("autoPriorityImmediateProbeLimit").value=String(config.immediate_probe_limit||30);document.getElementById("autoPriorityActiveGroupSize").value=String(config.active_group_size||10);const selected=config.provider_scope==="selected";document.getElementById("providerModeSelect").value=selected?"antigravity":"all";document.getElementById("configAntigravityModelGroupSelect").value=config.antigravity_model_group||"gemini";document.getElementById("manualAntigravityModelGroupSelect").value=config.antigravity_model_group||"gemini";syncProviderModeVisibility();syncAutoPriorityVisibility();for(const input of document.querySelectorAll("[data-provider]")){input.checked=Array.isArray(config.selected_providers)&&config.selected_providers.includes(input.dataset.provider);}fillPriorityRuleForm(config.priority_rules||defaultPriorityRuleConfig);syncAntigravityModelGroupVisibility();updateCustomSelects();refreshProviderLocalizedControls();}
        function syncAutoPriorityVisibility(){const enabled=document.getElementById("autoPriorityEnabled").checked;document.getElementById("providerSection").hidden=!enabled;}
        function updateConfigSummary(config){const scope=config.provider_scope==="selected"?(config.selected_providers||[]).map(getProviderDisplayName).join(", "):textFor("providerAll");document.getElementById("autoApplySummary").textContent=config.auto_apply===true?textFor("autoStatusOn"):textFor("autoStatusOff");document.getElementById("providerScopeSummary").textContent=scope;updatePriorityRulesSummary(config);}
        function summarySeparator(){return activeLanguage==="zh-CN"?"：":": ";}
        function priorityRuleLine(label,value){return label+summarySeparator()+value;}
        function priorityRuleSummaryProvider(title,lines){const section=document.createElement("div");section.className="priority-rule-summary-provider";const heading=document.createElement("strong");heading.textContent=title;section.appendChild(heading);for(const line of lines){const item=document.createElement("span");item.textContent=line;section.appendChild(item);}return section;}
        function updatePriorityRulesSummary(config){const root=document.getElementById("priorityRulesSummary");if(!root){return;}const rules=config&&config.priority_rules?config.priority_rules:defaultPriorityRuleConfig;const merged={enabled:rules&&typeof rules.enabled==="boolean"?rules.enabled:defaultPriorityRuleConfig.enabled,antigravity:{...defaultPriorityRuleConfig.antigravity,...((rules&&rules.antigravity)||{})},codex:{...defaultPriorityRuleConfig.codex,...((rules&&rules.codex)||{})}};root.textContent="";root.append(priorityRuleSummaryProvider("Antigravity",[priorityRuleLine(textFor("priorityRuleStartPriority"),merged.antigravity.start_priority)]),priorityRuleSummaryProvider("Codex",[priorityRuleLine(textFor("priorityRuleStartPriority"),merged.codex.start_priority),priorityRuleLine(textFor("priorityRuleFreeDepletedPriority"),merged.codex.free_depleted_priority),priorityRuleLine(textFor("priorityRuleFreeDepletedDisabled"),merged.codex.free_depleted_disabled?textFor("autoStatusOn"):textFor("autoStatusOff")),priorityRuleLine(textFor("priorityRulePaidKeepsEnabled"),merged.codex.paid_depleted_keeps_enabled?textFor("autoStatusOn"):textFor("autoStatusOff"))]));}
        async function saveConfig(){if(saveConfigCoolingDown){return;}const saveButton=document.getElementById("saveConfigButton");try{const config=readConfigForm();await managementFetch(CONFIG_PATH,{method:"PATCH",body:JSON.stringify(config)});const refreshedConfig=await loadConfig({silent:true});fillConfigForm(refreshedConfig);updateConfigSummary(refreshedConfig);showToast(textFor("configSaved"), "success");startSaveConfigCooldown(saveButton);}catch(err){handleManagementError(err,false);}}
        function readRulesOnlyConfig(){return {priority_rules:readPriorityRuleConfig()};}
        async function savePriorityRules(){if(saveConfigCoolingDown){return;}const saveButton=document.getElementById("savePriorityRulesButton");try{await managementFetch(CONFIG_PATH,{method:"PATCH",body:JSON.stringify(readRulesOnlyConfig())});const refreshedConfig=await loadConfig({silent:true});fillConfigForm(refreshedConfig);updateConfigSummary(refreshedConfig);showToast(textFor("configSaved"), "success");startSaveConfigCooldown(saveButton);}catch(err){handleManagementError(err,false);}}
        function startSaveConfigCooldown(saveButton){if(!saveButton){return;}saveConfigCoolingDown=true;saveButton.disabled=true;window.setTimeout(function(){saveConfigCoolingDown=false;saveButton.disabled=false;},saveConfigCooldownMs);}
        function deriveProviderOptions(files){const seen=new Map();for(const item of files){const value=String(item.provider||"").trim().toLowerCase();if(!value||seen.has(value)){continue;}seen.set(value,{value:value,label:getProviderDisplayName(item.provider||value)});}return Array.from(seen.values()).sort(function(a,b){return a.label.localeCompare(b.label);});}
        function renderProviderOptions(options, config){providerOptions=supportedProviderOptions(options);renderProviderCounts(options);renderCheckboxes("customProviderControls",providerOptions,"config",config&&config.selected_providers);renderCheckboxes("manualProviderControls",providerOptions,"manual",[]);renderProviderMultiSelect("config");renderProviderMultiSelect("manual");syncProviderModeVisibility();syncManualProviderModeVisibility();syncAntigravityModelGroupVisibility();renderSelectedProviderTags("config");renderSelectedProviderTags("manual");}
        function supportedProviderOptions(options){const labels=new Map(options.map(function(item){return [item.value,item.label];}));return ["antigravity","codex"].map(function(value){return {value:value,label:labels.get(value)||getProviderDisplayName(value)};});}
        function renderProviderCounts(options){const root=document.getElementById("providerCounts");root.textContent="";for(const provider of options){const badge=document.createElement("span");badge.className="badge";badge.textContent=provider.label+": "+provider.count;root.appendChild(badge);}if(options.length===0){const empty=document.createElement("span");empty.className="badge";empty.textContent="0";root.appendChild(empty);}}
        function renderCheckboxes(rootId, options, kind, selected){const root=document.getElementById(rootId);root.textContent="";for(const provider of options){const val=provider.value.toLowerCase();if(val==="antigravity"||val==="codex"){root.appendChild(createProviderCheckbox(provider,kind,selected||[]));}}}
        function createProviderCheckbox(provider, kind, selected){const wrapper=document.createElement("div");wrapper.className="checkbox-wrapper-46";const input=document.createElement("input");input.type="checkbox";input.className="inp-cbx";input.id=kind+"-provider-"+provider.value.replace(/[^a-z0-9_-]/g,"-");if(kind==="manual"){input.dataset.manualProvider=provider.value;}else{input.dataset.provider=provider.value;}input.checked=selected.includes(provider.value);input.addEventListener("change", function(){syncAntigravityModelGroupVisibility();renderSelectedProviderTags(kind);});const label=document.createElement("label");label.className="cbx";label.htmlFor=input.id;const box=document.createElement("span");box.className="cbx-box";box.innerHTML='<svg viewBox="0 0 12 10" height="10px" width="12px"><polyline points="1.5 6 4.5 9 10.5 1"></polyline></svg>';const text=document.createElement("span");text.className="cbx-text";text.textContent=provider.label;label.append(text,box);wrapper.append(input,label);return wrapper;}
        function renderProviderMultiSelect(kind){const root=document.getElementById(kind==="manual"?"manualProviderMultiSelect":"configProviderMultiSelect");if(!root){return;}const selectId=kind==="manual"?"manualProviderModeSelect":"providerModeSelect";root.textContent="";const trigger=document.createElement("button");trigger.type="button";trigger.className="provider-dropdown-trigger";trigger.onclick=function(event){toggleProviderDropdown(kind,event);};const label=document.createElement("span");label.className="provider-dropdown-label";label.textContent=providerSelectionLabel(kind);const arrow=document.createElement("span");arrow.className="provider-dropdown-arrow";arrow.innerHTML='<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M6 9l6 6 6-6"></path></svg>';trigger.append(label,arrow);const menu=document.createElement("div");menu.className="provider-dropdown-menu";menu.hidden=true;root.append(trigger,menu);addProviderDropdownItem(menu,kind,"all",textFor("providerAll"));for(const provider of providerOptions){addProviderDropdownItem(menu,kind,provider.value,provider.label);}document.getElementById(selectId).value=providerSelectionValue(kind);}
        function addProviderDropdownItem(menu,kind,value,label){const item=document.createElement("button");item.type="button";item.className="provider-dropdown-item";item.setAttribute("data-provider-option",value);if(providerOptionSelected(kind,value)){item.classList.add("active");}const text=document.createElement("span");text.textContent=label;item.append(text);item.onclick=function(event){event.stopPropagation();selectProviderOption(kind,value);};menu.appendChild(item);}
        function providerOptionSelected(kind,value){const selector=kind==="manual"?"[data-manual-provider]":"[data-provider]";const selected=readSelectedProviders(selector);if(value==="all"){return selected.length===0;}return selected.includes(value);}
        function toggleProviderDropdown(kind,event){if(event){event.stopPropagation();}const root=document.getElementById(kind==="manual"?"manualProviderMultiSelect":"configProviderMultiSelect");const menu=root&&root.querySelector(".provider-dropdown-menu");if(!menu){return;}const next=menu.hidden;closeProviderDropdowns();menu.hidden=!next;root.classList.toggle("open",next);}
        function closeProviderDropdowns(){document.querySelectorAll(".provider-dropdown-menu").forEach(function(item){item.hidden=true;item.parentElement.classList.remove("open");});}
        function selectProviderOption(kind,value){const selector=kind==="manual"?"[data-manual-provider]":"[data-provider]";const select=document.getElementById(kind==="manual"?"manualProviderModeSelect":"providerModeSelect");if(value==="all"){select.value="all";for(const input of document.querySelectorAll(selector)){input.checked=false;}}else{select.value=value;for(const input of document.querySelectorAll(selector)){const inputValue=input.dataset.provider||input.dataset.manualProvider;if(inputValue===value){input.checked=!input.checked;}}if(readSelectedProviders(selector).length===0){select.value="all";}}syncProviderModeVisibility();syncManualProviderModeVisibility();syncAntigravityModelGroupVisibility();renderSelectedProviderTags(kind);refreshProviderDropdownState(kind);}
        function providerSelectionValue(kind){const selector=kind==="manual"?"[data-manual-provider]":"[data-provider]";const selected=readSelectedProviders(selector);return selected.length===0?"all":selected[0];}
        function providerSelectionLabel(kind){const selector=kind==="manual"?"[data-manual-provider]":"[data-provider]";const selected=readSelectedProviders(selector);return selected.length===0?textFor("providerAll"):textFor("providerCustom");}
        function refreshProviderDropdownState(kind){const root=document.getElementById(kind==="manual"?"manualProviderMultiSelect":"configProviderMultiSelect");if(!root){return;}const label=root.querySelector(".provider-dropdown-label");if(label){label.textContent=providerSelectionLabel(kind);}for(const item of root.querySelectorAll("[data-provider-option]")){item.classList.toggle("active",providerOptionSelected(kind,item.getAttribute("data-provider-option")));}document.getElementById(kind==="manual"?"manualProviderModeSelect":"providerModeSelect").value=providerSelectionValue(kind);}
        function renderSelectedProviderTags(kind){const root=document.getElementById(kind==="manual"?"manualSelectedProviderTags":"configSelectedProviderTags");if(!root){return;}const selector=kind==="manual"?"[data-manual-provider]":"[data-provider]";const selected=readSelectedProviders(selector);root.textContent="";root.hidden=selected.length===0;root.setAttribute("aria-label",textFor("sortingProviderSelection"));for(const value of selected){const tag=document.createElement("span");tag.className="provider-tag";tag.textContent=getProviderDisplayName(value);const remove=document.createElement("button");remove.type="button";remove.className="provider-tag-remove";remove.textContent="×";remove.onclick=function(){selectProviderOption(kind,value);};tag.appendChild(remove);root.appendChild(tag);}}
        function priorityRuleInput(name){return document.querySelector('[name="priority_rules.'+name+'"]');}
        function readRuleNumber(name){const input=priorityRuleInput(name);return input?Number(input.value):0;}
        function readRuleChecked(name){const input=priorityRuleInput(name);return !!(input&&input.checked);}
        function writeRuleValue(name,value){const input=priorityRuleInput(name);if(!input){return;}if(input.type==="checkbox"){input.checked=!!value;}else{input.value=String(value);}}
        function readPriorityRuleConfig(){return {enabled:readRuleChecked("enabled"),antigravity:{start_priority:readRuleNumber("antigravity.start_priority")},codex:{start_priority:readRuleNumber("codex.start_priority"),free_depleted_priority:readRuleNumber("codex.free_depleted_priority"),free_depleted_disabled:readRuleChecked("codex.free_depleted_disabled"),paid_depleted_keeps_enabled:readRuleChecked("codex.paid_depleted_keeps_enabled")}};}
        function fillPriorityRuleForm(rules){const merged={enabled:rules&&typeof rules.enabled==="boolean"?rules.enabled:defaultPriorityRuleConfig.enabled,antigravity:{...defaultPriorityRuleConfig.antigravity,...((rules&&rules.antigravity)||{})},codex:{...defaultPriorityRuleConfig.codex,...((rules&&rules.codex)||{})}};writeRuleValue("enabled",merged.enabled);writeRuleValue("antigravity.start_priority",merged.antigravity.start_priority);writeRuleValue("codex.start_priority",merged.codex.start_priority);writeRuleValue("codex.free_depleted_priority",merged.codex.free_depleted_priority);writeRuleValue("codex.free_depleted_disabled",merged.codex.free_depleted_disabled);writeRuleValue("codex.paid_depleted_keeps_enabled",merged.codex.paid_depleted_keeps_enabled);syncPriorityRulesEnabled(false);}
        function applyPriorityRulesEnabled(enabled){const card=document.getElementById("configPriorityRulesCard");if(card){card.classList.toggle("rules-disabled",!enabled);}document.querySelectorAll(".priority-rule-field input").forEach(function(input){input.disabled=!enabled;});}
        function syncPriorityRulesEnabled(confirmEnable){const enabled=readRuleChecked("enabled");if(enabled&&confirmEnable){writeRuleValue("enabled",false);applyPriorityRulesEnabled(false);confirmPriorityRulesEnable();return;}applyPriorityRulesEnabled(enabled);}
        function confirmPriorityRulesEnable(){showConfirmModal(textFor("priorityRulesEnableConfirm"),function(){writeRuleValue("enabled",true);applyPriorityRulesEnabled(true);});}
        function restoreDefaultPriorityRules(){if(priorityRulesRestoreCoolingDown){return;}showConfirmModal(textFor("restoreDefaultRulesConfirm"),function(){fillPriorityRuleForm(defaultPriorityRuleConfig);showToast(textFor("priorityRulesRestored"),"success");startPriorityRulesRestoreCooldown();});}
        function startPriorityRulesRestoreCooldown(){const resetButton=document.getElementById("resetPriorityRulesButton");if(!resetButton){return;}priorityRulesRestoreCoolingDown=true;resetButton.disabled=true;window.setTimeout(function(){priorityRulesRestoreCoolingDown=false;resetButton.disabled=false;},priorityRulesRestoreCooldownMs);}
        function showConfirmModal(message,onConfirm){confirmModalCallback=onConfirm;document.getElementById("confirmModalMessage").textContent=message;document.getElementById("confirmModal").hidden=false;}
        function closeConfirmModal(confirmed){const callback=confirmModalCallback;confirmModalCallback=null;document.getElementById("confirmModal").hidden=true;if(confirmed&&callback){callback();}}
        function setOverviewLoading(loading){credentialSummaryLoading=loading;const button=document.getElementById("openProviderModalButton");if(button){button.disabled=loading;}setTabDisabled("config", loading);if(loading){document.getElementById("totalCredentialValue").textContent=textFor("overviewLoadingText");const root=document.getElementById("providerCounts");root.textContent="";const badge=document.createElement("span");badge.className="badge";badge.textContent=textFor("overviewLoadingText");root.appendChild(badge);}}
        function setTabDisabled(name, disabled){const tab=document.querySelector('[data-tab="'+name+'"]');if(tab){tab.disabled=disabled;}}
        async function loadCredentialSummary(){setOverviewLoading(true);try{const result=await managementFetch(AUTH_FILES_PATH,{method:"GET"});const files=Array.isArray(result.files)?result.files:[];document.getElementById("totalCredentialValue").textContent=String(files.length);const counts=new Map();for(const item of files){const value=String(item.provider||"").trim().toLowerCase();if(!value){continue;}const current=counts.get(value)||{value:value,label:getProviderDisplayName(item.provider||value),count:0};current.count++;counts.set(value,current);}renderProviderOptions(Array.from(counts.values()).sort(function(a,b){return a.label.localeCompare(b.label);}),currentConfig);}catch(err){handleManagementError(err,false);}finally{setOverviewLoading(false);}}
        function selectedManualProviders(){return readProviderSelection("manualProviderModeSelect","[data-manual-provider]");}
        function providerQuery(providers, authIndex){const group="antigravity_model_group="+encodeURIComponent(document.getElementById("manualAntigravityModelGroupSelect").value);const auth=authIndex?"&auth_index="+encodeURIComponent(authIndex):"";if(!providers||providers.length===0){return "provider_scope=all&"+group+auth;}return providers.map(function(provider){return "provider="+encodeURIComponent(provider);}).join("&")+"&"+group+auth;}
        function credentialDisplayName(c) {
            return c.account || c.email || c.name || c.auth_index || "";
        }
        function priorityChangeText(c) {
            const from = c.priority_missing ? textFor("priorityUnset") : c.priority_from;
            return from + " -> " + c.priority_to;
        }
        function formatResult(result) {
            if (!result || !Array.isArray(result.changes)) {
                return textFor("noChanges");
            }
            const changes = result.changes.filter(c => c.status === "success" && c.priority_attempted);
            if (changes.length === 0) {
                return textFor("noChanges");
            }
            const groups = {};
            for (const c of changes) {
                const pName = getProviderDisplayName(c.provider || "unknown");
                if (!groups[pName]) {
                    groups[pName] = [];
                }
                groups[pName].push(c);
            }
            let text = "";
            for (const providerName in groups) {
                text += "【" + providerName + "】\n";
                for (const c of groups[providerName]) {
                    text += "  - " + credentialDisplayName(c) + ": " + priorityChangeText(c) + "\n";
                }
                text += "\n";
            }
            return text.trim();
        }
        async function runCredentialPriority(mode, providers, button, authIndex, mergeExisting){const control=button||document.getElementById("executePriorityButton");let oldText="";setManualRunControlsDisabled(true);if(control){control.disabled=true;oldText=control.textContent;control.textContent=textFor("running");}try{const query=providerQuery(providers||[],authIndex);const path="/v0/management/plugins/credential-priority/run?mode="+encodeURIComponent(mode)+"&"+query;const result=await managementFetch(path,{method:"POST"});if(result){if(mergeExisting){closeProviderModal();showResult(mergeResults(currentResult,result));}else{closeProviderModal();showResult(result);}showToast(textFor("runCompleted"),"success");await loadCredentialSummary();}}catch(err){handleManagementError(err,false);}finally{setManualRunControlsDisabled(false);if(control){control.disabled=false;control.textContent=oldText;}}}
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
            const changes=result.changes.filter(function(c){return (c.status==="success"&&c.priority_attempted)||c.status === "failed";});
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
                row.style.boxShadow="0 1px 2px 0 rgba(0,0,0,0.05)";
                const pName=getProviderDisplayName(c.provider||"unknown");
                const badge=document.createElement("span");
                badge.className="badge";
                badge.style.margin="0";
                badge.style.flexShrink="0";
                badge.textContent=pName;
                const nameSpan=document.createElement("span");
                nameSpan.style.fontFamily="SFMono-Regular,Consolas,Menlo,monospace";
				nameSpan.style.fontSize="13px";
				nameSpan.style.color="var(--text)";
				nameSpan.style.whiteSpace="nowrap";
				nameSpan.style.overflow="hidden";
				nameSpan.style.textOverflow="ellipsis";
				nameSpan.style.flex="1";
				nameSpan.style.minWidth="0";
				nameSpan.textContent=credentialDisplayName(c);
                const changeSpan=document.createElement("span");
                changeSpan.style.fontSize="13px";
                changeSpan.style.fontWeight="600";
                changeSpan.style.color="var(--blue)";
                changeSpan.style.flexShrink="0";
                changeSpan.style.whiteSpace="nowrap";
                changeSpan.textContent=c.status === "failed" ? textFor("failedQuotaFetch") : priorityChangeText(c);
                row.append(badge,nameSpan,changeSpan);
                if(c.status === "failed"){
                    row.setAttribute("data-auth-index", c.retry_auth_index||"");
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
        function retryCredentialQuota(c, button){const provider=String(c.provider||"").toLowerCase();runCredentialPriority("apply",provider?[provider]:[],button,c.retry_auth_index||"",true);}
        function openProviderModal(){if(credentialSummaryLoading){return;}document.getElementById("providerModal").hidden=false;document.getElementById("modalNotice").textContent="";document.getElementById("modalNotice").className="message-box";}
        function closeProviderModal(){document.getElementById("providerModal").hidden=true;document.getElementById("modalNotice").textContent="";document.getElementById("modalNotice").className="message-box";}
        function syncProviderModeVisibility(){document.getElementById("customProviderControls").hidden=true;syncAntigravityModelGroupVisibility();}
        function syncManualProviderModeVisibility(){document.getElementById("manualProviderControls").hidden=true;syncAntigravityModelGroupVisibility();}
        function providerSelectionIncludesAntigravity(modeSelectId, selector){const mode=document.getElementById(modeSelectId).value;if(mode==="all"){return providerOptions.some(function(provider){return provider.value==="antigravity";});}return Array.from(document.querySelectorAll(selector)).some(function(input){return input.checked&&(input.dataset.provider==="antigravity"||input.dataset.manualProvider==="antigravity");});}
        function syncAntigravityModelGroupVisibility(){document.getElementById("configAntigravityModelGroupRow").hidden=!providerSelectionIncludesAntigravity("providerModeSelect","[data-provider]");document.getElementById("manualAntigravityModelGroupRow").hidden=!providerSelectionIncludesAntigravity("manualProviderModeSelect","[data-manual-provider]");}
        function showTab(name){if(credentialSummaryLoading&&name!=="overview"){return;}document.getElementById("overviewPanel").hidden=name!=="overview";document.getElementById("configPanel").hidden=name!=="config";for(const tab of document.querySelectorAll(".tab")){tab.classList.toggle("active",tab.dataset.tab===name);}}
        function parseRetryDelaySeconds(message){const match=String(message).match(/Try again in (?:(\d+)m)?(?:(\d+)s)?/i);if(!match){return 0;}return Number(match[1]||0)*60+Number(match[2]||0);}
        function handleManagementError(err, login){const message=String(err&&err.message?err.message:err);const seconds=parseRetryDelaySeconds(message);if(seconds>0||message.includes("IP banned due to too many failed attempts")){startBanCountdown(seconds||1800,message);}if(login){setLoginMessage(message);}else{setMessage(message);if(!document.getElementById("providerModal").hidden){showModalNotice(message,"error");}}showToast(message,"error");}
        function startBanCountdown(seconds, raw){const box=document.getElementById("banCountdown");box.hidden=false;let remaining=seconds;const render=function(){const minutes=Math.floor(remaining/60);const rest=remaining%60;box.textContent=raw+"\n"+textFor("banHint")+"\n"+minutes+"m"+rest+"s";remaining=Math.max(0,remaining-1);};render();window.clearInterval(window.credentialPriorityBanTimer);window.credentialPriorityBanTimer=window.setInterval(render,1000);}
        function showToast(message, type){const toast=document.createElement("div");toast.setAttribute("role","alert");const cls=type==="success"?"bg-green-100":type==="error"?"bg-red-100":type==="warning"?"bg-yellow-100":"bg-blue-100";toast.className="toast-alert "+cls;toast.textContent=message;document.getElementById("toastRoot").appendChild(toast);window.setTimeout(function(){toast.remove();},2500);}
        function showModalNotice(message, type){const box=document.getElementById("modalNotice");box.className="message-box "+(type==="error"?"error":"warn");box.textContent=message;}
        function switchLanguage(){setLanguage(activeLanguage==="zh-CN"?"en-US":"zh-CN");}
        function setLanguage(language){activeLanguage=language;applyLanguage();updateConfigSummary(currentConfig);updateCustomSelects();refreshProviderLocalizedControls();}
        function refreshProviderLocalizedControls(){renderProviderMultiSelect("config");renderProviderMultiSelect("manual");renderSelectedProviderTags("config");renderSelectedProviderTags("manual");}
        function applyLanguage(){const messages=translations[activeLanguage];for(const element of document.querySelectorAll("[data-i18n]")){const key=element.getAttribute("data-i18n");if(messages[key]){element.textContent=messages[key];}}document.documentElement.lang=activeLanguage;}
        function initCustomSelect(selectId) {
            const select = document.getElementById(selectId);
            if (!select) return;
            let wrapper = document.getElementById(selectId + "-custom-wrapper");
            if (wrapper) {
                wrapper.remove();
            }
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
                if (opt.value === select.value) {
                    optionEl.classList.add("active");
                }
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
                    const event = new Event("change");
                    select.dispatchEvent(event);
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
                if (wasHidden) {
                    wrapper.classList.add("open");
                } else {
                    wrapper.classList.remove("open");
                }
            });
        }
        function updateCustomSelects() {
            initCustomSelect("configAntigravityModelGroupSelect");
            initCustomSelect("manualAntigravityModelGroupSelect");
        }
        document.addEventListener("click", function() {
            closeProviderDropdowns();
            document.querySelectorAll(".custom-select-options").forEach(function(cont) {
                cont.hidden = true;
                cont.parentElement.classList.remove("open");
            });
        });

        applyLanguage();
    </script>
</body>
</html>
`

var parsedStatusTemplate = template.Must(template.New("status").Parse(StatusHTML))
