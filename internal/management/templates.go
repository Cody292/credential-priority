package management

import (
	"html/template"
)

// StatusHTML 是渲染管理状态页面的 HTML 模版文本。
// 使用了简约设计和宽松留白 (p-6, max-w-3xl)，并通过 CSS 提供现代的视觉呈现。
const StatusHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <title>凭证优先级管理状态页</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
            background-color: #f9f9fb;
            color: #1e1e2f;
            margin: 0;
            padding: 24px;
            display: flex;
            justify-content: center;
        }
        .container {
            width: 100%;
            max-w-3xl: 768px; /* max-w-3xl = 768px */
            max-width: 48rem; /* 768px */
            background: #ffffff;
            border-radius: 12px;
            box-shadow: 0 4px 6px -1px rgba(0, 0, 0, 0.05), 0 2px 4px -1px rgba(0, 0, 0, 0.03);
            border: 1px solid rgba(0, 0, 0, 0.05);
            padding: 2.5rem; /* p-10 */
        }
        h1 {
            font-size: 1.5rem;
            font-weight: 600;
            color: #111118;
            margin-top: 0;
            margin-bottom: 2rem;
            border-bottom: 1px solid #f0f0f4;
            padding-bottom: 1rem;
        }
        .section {
            margin-bottom: 2rem;
        }
        .section-title {
            font-size: 0.875rem;
            text-transform: uppercase;
            letter-spacing: 0.05em;
            color: #6b7280;
            margin-bottom: 1rem;
            font-weight: 600;
        }
        .grid {
            display: grid;
            grid-template-columns: repeat(2, minmax(0, 1fr));
            gap: 1.5rem;
        }
        .card {
            background-color: #fcfcfd;
            border: 1px solid #f3f4f6;
            border-radius: 8px;
            padding: 1.25rem;
        }
        .card-label {
            font-size: 0.75rem;
            color: #8b949e;
            margin-bottom: 0.25rem;
        }
        .card-value {
            font-size: 1.25rem;
            font-weight: 600;
            color: #111118;
        }
        .badge {
            display: inline-flex;
            align-items: center;
            border-radius: 9999px;
            padding: 0.25rem 0.75rem;
            font-size: 0.75rem;
            font-weight: 500;
        }
        .badge-success { background-color: #ecfdf5; color: #065f46; }
        .badge-warning { background-color: #fffbb5; color: #736700; }
        .badge-danger { background-color: #fef2f2; color: #991b1b; }
        .audit-box {
            background-color: #f8f9fa;
            border-left: 4px solid #e5e7eb;
            font-family: SFMono-Regular, Consolas, "Liberation Mono", Menlo, monospace;
            font-size: 0.875rem;
            padding: 1rem;
            border-radius: 4px;
            overflow-x: auto;
            white-space: pre-wrap;
            word-break: break-all;
        }
        .btn-group {
            margin-top: 2rem;
            display: flex;
            gap: 1rem;
        }
        .btn {
            display: inline-flex;
            align-items: center;
            justify-content: center;
            border-radius: 6px;
            font-size: 0.875rem;
            font-weight: 500;
            padding: 0.5rem 1rem;
            cursor: pointer;
            border: 1px solid transparent;
            transition: all 0.2s;
            text-decoration: none;
            min-height: 40px;
        }
        .btn-primary {
            background-color: #2563eb;
            color: #ffffff;
        }
        .btn-primary:hover {
            background-color: #1d4ed8;
        }
        .btn-secondary {
            background-color: #ffffff;
            color: #374151;
            border-color: #d1d5db;
        }
        .btn-secondary:hover {
            background-color: #f9fafb;
        }
        .text-right {
            text-align: right;
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>凭证优先级管理状态页</h1>

        <div class="section">
            <div class="section-title">凭证概览</div>
            <div class="grid">
                <div class="card">
                    <div class="card-label">总凭证数</div>
                    <div class="card-value">{{.TotalCredentials}}</div>
                </div>
                <div class="card">
                    <div class="card-label">探测结果分布</div>
                    <div style="margin-top: 0.5rem; display: flex; gap: 0.5rem; flex-wrap: wrap;">
                        <span class="badge badge-success">Fresh: {{.FreshCount}}</span>
                        <span class="badge badge-warning">Unknown: {{.UnknownCount}}</span>
                        <span class="badge badge-danger">Failed: {{.FailedCount}}</span>
                    </div>
                </div>
            </div>
        </div>

        <div class="section">
            <div class="section-title">时间信息</div>
            <div class="grid">
                <div class="card">
                    <div class="card-label">下一次自动探测时间</div>
                    <div class="card-value">{{if .NextProbeAt.IsZero}}暂无安排{{else}}{{.NextProbeAt.Format "2006-01-02 15:04:05 UTC"}}{{end}}</div>
                </div>
            </div>
        </div>

        <div class="section">
            <div class="section-title">最近审计摘要</div>
            <div class="audit-box">{{if .LatestAudit}}{{.LatestAudit}}{{else}}暂无审计记录{{end}}</div>
        </div>

        <div class="section">
            <div class="section-title">手动触发操作</div>
            <form action="/v0/management/plugins/credential-priority/run?mode=dry-run" method="POST" style="display: inline;">
                <button type="submit" class="btn btn-secondary">触发 Dry-Run</button>
            </form>
            <form action="/v0/management/plugins/credential-priority/run?mode=apply" method="POST" style="display: inline; margin-left: 0.5rem;">
                <button type="submit" class="btn btn-primary">触发 Apply 自动写入</button>
            </form>
        </div>

        <div class="section text-right" style="border-top: 1px solid #f0f0f4; padding-top: 1rem; margin-top: 3rem;">
            <a href="/v0/management/plugins/credential-priority/diagnostics" class="btn btn-secondary" style="font-size: 0.75rem;">导出诊断信息</a>
            <a href="/v0/management/plugins/credential-priority/snapshot/latest" class="btn btn-secondary" style="font-size: 0.75rem; margin-left: 0.5rem;">查看最新快照</a>
        </div>
    </div>
</body>
</html>
`

var parsedStatusTemplate = template.Must(template.New("status").Parse(StatusHTML))
