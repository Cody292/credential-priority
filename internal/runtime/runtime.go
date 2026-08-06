package runtime

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"credential-priority/internal/apply"
	"credential-priority/internal/config"
	"credential-priority/internal/host"
	"credential-priority/internal/management"
)

const maxRunHistory = 5

// Runtime 持有 CPA 插件生命周期、配置、ticker 和 single-flight 状态。
type Runtime struct {
	mu              sync.Mutex
	runMu           sync.Mutex
	tickerFactory   TickerFactory
	runner          TaskRunner
	rootCtx         context.Context
	cancel          context.CancelFunc
	cfg             config.Config
	hostCallbacks   host.HostCallbacks
	clock           Clock
	sleeper         Sleeper
	management      *management.Handler
	latestResult    apply.Result
	latestAudit     string
	runHistory      []RunHistoryEntry
	lastAutoApplyAt time.Time
	worker          *tickerWorker
	shutdown        bool
}

// New 创建未注册的 runtime；ticker 会在 register/reconfigure 成功后启动。
func New(options Options) *Runtime {
	factory := options.TickerFactory
	if factory == nil {
		factory = timeTickerFactory{}
	}
	runner := options.Runner
	clock := options.Clock
	if clock == nil {
		clock = realRuntimeClock{}
	}
	sleeper := options.Sleeper
	if sleeper == nil {
		sleeper = realSleeper{}
	}
	ctx, cancel := context.WithCancel(context.Background())
	runtime := &Runtime{tickerFactory: factory, rootCtx: ctx, cancel: cancel, cfg: config.Default(), hostCallbacks: options.Host, clock: clock, sleeper: sleeper}
	if runner != nil {
		runtime.runner = runner
	} else {
		runtime.runner = runtime.runProductionTask
	}
	runtime.management = management.NewHandler(managementRunner{runtime: runtime})
	return runtime
}

// Handle 根据 CPA 方法名处理 JSON 请求并返回 JSON 信封字节。
func (r *Runtime) Handle(ctx context.Context, method string, request []byte) []byte {
	switch method {
	case "plugin.register":
		parsed, err := decodeRegisterRequest(request)
		if err != nil {
			return failure(err)
		}
		result, err := r.Register(ctx, parsed)
		return envelopeRegister(result, err)
	case "plugin.reconfigure":
		parsed, err := decodeReconfigureRequest(request)
		if err != nil {
			return failure(err)
		}
		result, err := r.Reconfigure(ctx, parsed)
		return envelopeRegister(result, err)
	case "plugin.shutdown":
		return envelopeStatus(r.Shutdown(ctx))
	case "management.register":
		return r.registerManagement()
	case "management.handle":
		return r.handleManagement(ctx, request)
	case "usage.handle":
		return envelopeStatus(r.HandleUsage(ctx, request))
	default:
		return failure(fmt.Errorf("%w: method %q", ErrInvalidRequest, method))
	}
}

func (r *Runtime) snapshotRun(result apply.Result, audit string) {
	r.snapshotRunEntry(result, audit, RunHistoryEntry{
		Kind:      "apply",
		Trigger:   "manual",
		Attempted: result.Attempted,
		Succeeded: result.Succeeded,
		Failed:    result.Failed,
		Skipped:   result.Skipped,
		Message:   audit,
	})
}

func (r *Runtime) snapshotRunEntry(result apply.Result, audit string, entry RunHistoryEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.latestResult = result
	r.latestAudit = audit
	if entry.At.IsZero() {
		entry.At = r.clock.Now().UTC()
	}
	if entry.Kind == "" {
		entry.Kind = "apply"
	}
	history := make([]RunHistoryEntry, 0, maxRunHistory)
	history = append(history, entry)
	for i := 0; i < len(r.runHistory) && len(history) < maxRunHistory; i++ {
		history = append(history, r.runHistory[i])
	}
	r.runHistory = history
}

func (r *Runtime) currentRunSnapshot() (apply.Result, string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.latestResult, r.latestAudit
}

func (r *Runtime) currentRunHistory() []RunHistoryEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]RunHistoryEntry, len(r.runHistory))
	copy(out, r.runHistory)
	return out
}

type realRuntimeClock struct{}

func (realRuntimeClock) Now() time.Time {
	return time.Now().UTC()
}

type realSleeper struct{}

func (realSleeper) Sleep(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// Register 解析首次配置，启动 ticker，并返回真实能力声明。
func (r *Runtime) Register(ctx context.Context, request RegisterRequest) (RegisterResult, error) {
	cfg, err := config.LoadBytes([]byte(request.ConfigYAML))
	if err != nil {
		return RegisterResult{}, fmt.Errorf("load register config: %w", err)
	}
	if err := r.replaceConfig(ctx, cfg); err != nil {
		return RegisterResult{}, err
	}
	return registrationResult(), nil
}

func registrationResult() RegisterResult {
	return RegisterResult{
		SchemaVersion: 1,
		Metadata: Metadata{
			Name:             config.PluginID,
			Version:          "1.1.5",
			Author:           "CPA Plugins",
			GitHubRepository: "https://github.com/Cody292/credential-priority",
			Description:      "Fresh evidence based credential priority management API.",
			ConfigFields:     configFields(),
		},
		Capabilities: map[string]bool{
			"management_api": true,
			"usage_plugin":   true,
		},
	}
}

func configFields() []ConfigField {
	defaults := config.Default()
	rules := defaults.PriorityRules
	return []ConfigField{
		{Name: "auto_apply", Type: "boolean", Description: localizedDescription("启用定时自动优先级排序并写回；默认 false，手动执行仍可用。", "Enable scheduled automatic priority sorting and write-back; default false so manual runs remain available."), DefaultValue: defaults.AutoApply},
		{Name: "provider_scope", Type: "string", Description: localizedDescription("排序提供商：填 all 表示全部；或填单个/多个提供商，多个用 | 分隔，例如 antigravity|codex|xai。", "Providers to sort: all for every supported provider, or a single/multiple list separated by |, e.g. antigravity|codex|xai."), DefaultValue: string(defaults.ProviderScope)},
		{Name: "antigravity_model_group", Type: "string", Description: localizedDescription("Antigravity 配额模型组：gemini 或 claude_gpt。", "Antigravity quota model group: gemini or claude_gpt."), EnumValues: []string{"gemini", "claude_gpt"}, DefaultValue: string(defaults.AntigravityModelGroup)},
		{Name: "interval", Type: "string", Description: localizedDescription("自动排序间隔（分钟）。填写纯数字即可，无需单位；默认 15。", "Auto-sort interval in minutes. Enter a plain number without a unit; default 15."), DefaultValue: formatDurationMinutes(defaults.Interval)},
		{Name: "immediate_probe_limit", Type: "integer", Description: localizedDescription("单轮立即探测凭证上限；超过后分批探测。默认 30。", "Max credentials probed immediately per round; excess are batched. Default 30."), DefaultValue: defaults.ImmediateProbeLimit},
		{Name: "active_group_size", Type: "integer", Description: localizedDescription("分批探测时每批凭证数。默认 10。", "Credentials per batch when probing in batches. Default 10."), DefaultValue: defaults.ActiveGroupSize},
		{Name: "max_concurrency", Type: "integer", Description: localizedDescription("探测并发上限。默认 6。", "Probe concurrency limit. Default 6."), DefaultValue: defaults.MaxConcurrency},
		{Name: "priority_rules.enabled", Type: "boolean", Description: localizedDescription("启用自定义优先级规则；关闭时使用内置策略。默认 false。", "Enable custom priority rules; when false, built-in strategy is used. Default false."), DefaultValue: rules.Enabled},
		{Name: "priority_rules.antigravity.start_priority", Type: "integer", Description: localizedDescription("Antigravity 可用凭证起始优先级。默认 100。", "Antigravity start priority for available credentials. Default 100."), DefaultValue: rules.Antigravity.StartPriority},
		{Name: "priority_rules.codex.start_priority", Type: "integer", Description: localizedDescription("Codex 可用凭证起始优先级。默认 100。", "Codex start priority for available credentials. Default 100."), DefaultValue: rules.Codex.StartPriority},
		{Name: "priority_rules.codex.free_depleted_priority", Type: "integer", Description: localizedDescription("Codex Free 额度为 0 时写入的优先级。默认 -1。", "Priority written when Codex Free quota is 0. Default -1."), DefaultValue: rules.Codex.FreeDepletedPriority},
		{Name: "priority_rules.codex.free_depleted_disabled", Type: "boolean", Description: localizedDescription("Codex Free 额度为 0 时是否禁用。默认 true。", "Disable Codex Free when quota is 0. Default true."), DefaultValue: rules.Codex.FreeDepletedDisabled},
		{Name: "priority_rules.codex.paid_depleted_disabled", Type: "boolean", Description: localizedDescription("Codex Plus/Pro/Team 耗尽时是否禁用。true=禁用，false=保持启用。默认 false。", "Disable Codex Plus/Pro/Team when depleted. true=disable, false=keep enabled. Default false."), DefaultValue: rules.Codex.PaidDepletedDisabled},
		{Name: "priority_rules.xai.start_priority", Type: "integer", Description: localizedDescription("xAI 可用凭证起始优先级。默认 100。", "xAI start priority for available credentials. Default 100."), DefaultValue: rules.XAI.StartPriority},
		{Name: "priority_rules.xai.free_depleted_priority", Type: "integer", Description: localizedDescription("xAI 免费额度耗尽时写入的优先级。默认 -1。", "Priority when xAI free usage is depleted. Default -1."), DefaultValue: rules.XAI.FreeDepletedPriority},
		{Name: "priority_rules.xai.free_depleted_disabled", Type: "boolean", Description: localizedDescription("xAI 免费额度耗尽时是否硬禁用。默认 false（软禁用：仅降 priority，不 PatchDisabled）。", "Hard-disable when xAI free usage is depleted. Default false (soft-disable: lower priority only, no PatchDisabled)."), DefaultValue: rules.XAI.FreeDepletedDisabled},
		{Name: "priority_rules.xai.free_participates_priority", Type: "boolean", Description: localizedDescription("Free 凭证是否参与优先级排序。默认 false（仅保留 429 耗尽/冷却/401）；显式 true 才 opt-in free-first。", "Whether Free credentials participate in priority sorting. Default false (keep 429 depletion/cooldown/401 only); set true to opt in free-first."), DefaultValue: rules.XAI.FreeParticipatesPriority},
		{Name: "priority_rules.xai.weekly_depleted_priority", Type: "integer", Description: localizedDescription("xAI 仅周限额耗尽时写入的优先级。默认 -1。", "Priority when only xAI weekly limit is depleted. Default -1."), DefaultValue: rules.XAI.WeeklyDepletedPriority},
		{Name: "priority_rules.xai.monthly_and_weekly_depleted_priority", Type: "integer", Description: localizedDescription("xAI 周与月均耗尽时写入的优先级。默认 -1。", "Priority when xAI weekly and monthly are depleted. Default -1."), DefaultValue: rules.XAI.MonthlyAndWeeklyDepletedPriority},
		{Name: "priority_rules.xai.monthly_and_weekly_depleted_disabled", Type: "boolean", Description: localizedDescription("xAI 周与月均耗尽时是否禁用。默认 true。", "Disable when xAI weekly and monthly are depleted. Default true."), DefaultValue: rules.XAI.MonthlyAndWeeklyDepletedDisabled},
	}
}

func localizedDescription(chinese string, english string) string {
	return chinese + "\n" + english
}

func formatDurationMinutes(value time.Duration) string {
	if value <= 0 {
		return "15"
	}
	minutes := int(value / time.Minute)
	if minutes < 1 {
		minutes = 1
	}
	return fmt.Sprintf("%d", minutes)
}

// Reconfigure 验证新配置并在成功后用新 interval 重启 ticker。
func (r *Runtime) Reconfigure(ctx context.Context, request ReconfigureRequest) (RegisterResult, error) {
	cfg, err := config.LoadBytes([]byte(request.ConfigYAML))
	if err != nil {
		return RegisterResult{}, fmt.Errorf("load reconfigure config: %w", err)
	}
	if err := r.replaceConfig(ctx, cfg); err != nil {
		return RegisterResult{}, err
	}
	return registrationResult(), nil
}

// Run 执行一轮手动任务；若已有任务运行则返回 ErrRunInProgress。
func (r *Runtime) Run(ctx context.Context) error {
	return r.run(ctx, TriggerManual, "", nil, "", nil)
}

// RunWithProviders 执行限定 provider 的一轮手动任务。
func (r *Runtime) RunWithProviders(ctx context.Context, providers []string) error {
	return r.RunWithProviderScope(ctx, config.ProviderScopeSelected, providers)
}

// RunWithProviderScope 执行限定或全部 provider 的一轮手动任务。
func (r *Runtime) RunWithProviderScope(ctx context.Context, scope config.ProviderScope, providers []string) error {
	return r.run(ctx, TriggerManual, scope, providers, "", nil)
}

// AutoApply 执行一轮自动任务；Codex/Antigravity/xAI 在轮次内并行；若已有任务运行则返回 ErrRunInProgress。
func (r *Runtime) AutoApply(ctx context.Context) error {
	return r.runAuto(ctx, "", nil)
}

// AutoApplyWithProviders 执行限定 provider 的一轮自动写入任务。
func (r *Runtime) AutoApplyWithProviders(ctx context.Context, providers []string) error {
	return r.AutoApplyWithProviderScope(ctx, config.ProviderScopeSelected, providers)
}

// AutoApplyWithProviderScope 执行限定或全部 provider 的一轮自动写入任务。
func (r *Runtime) AutoApplyWithProviderScope(ctx context.Context, scope config.ProviderScope, providers []string) error {
	return r.runAuto(ctx, scope, providers)
}

// runAuto 持有全局 runMu，并在 provider 维度并行执行探测与写回。
// 受 cfg.Interval 最小间隔保护：启动/reconfigure 立即触发与周期调度共用同一冷却，避免短时间连发。
func (r *Runtime) runAuto(ctx context.Context, scope config.ProviderScope, providers []string) error {
	if !r.runMu.TryLock() {
		return ErrRunInProgress
	}
	defer r.runMu.Unlock()
	taskCtx, cleanup, cfg, _, err := r.taskContext(ctx)
	if err != nil {
		return err
	}
	defer cleanup()
	now := r.clock.Now().UTC()
	r.mu.Lock()
	last := r.lastAutoApplyAt
	interval := cfg.Interval
	if interval > 0 && !last.IsZero() && now.Sub(last) < interval {
		r.mu.Unlock()
		return nil
	}
	r.mu.Unlock()
	if scope == config.ProviderScopeAll {
		cfg.ProviderScope = config.ProviderScopeAll
		cfg.SelectedProviders = nil
	} else if len(providers) > 0 {
		cfg.ProviderScope = config.ProviderScopeSelected
		cfg.SelectedProviders = append([]string(nil), providers...)
	}
	runErr := r.runAutoParallelProviders(taskCtx, TaskRequest{Config: cfg, Trigger: TriggerAutoApply})
	if runErr != nil {
		// 取消/失败都不推进 last：否则会出现「无 history 却进入 15m 冷却」。
		if !errors.Is(runErr, context.Canceled) && !errors.Is(runErr, context.DeadlineExceeded) {
			r.snapshotRunEntry(apply.Result{}, runErr.Error(), RunHistoryEntry{
				Kind:    "auto_apply",
				Trigger: string(TriggerAutoApply),
				Message: "auto_apply error: " + runErr.Error(),
			})
		}
		return fmt.Errorf("run %s: %w", TriggerAutoApply, runErr)
	}
	// 仅成功路径推进 last（snapshot 已在 runAutoParallelProviders 内完成）。
	r.mu.Lock()
	r.lastAutoApplyAt = r.clock.Now().UTC()
	r.mu.Unlock()
	return nil
}

// nextAutoApplyWait 返回距离下一次允许自动排序的等待时长（从 lastAutoApplyAt 起算 interval）。
func (r *Runtime) nextAutoApplyWait(interval time.Duration) time.Duration {
	if interval <= 0 {
		interval = 15 * time.Minute
	}
	r.mu.Lock()
	last := r.lastAutoApplyAt
	r.mu.Unlock()
	if last.IsZero() {
		// 尚未跑过：短等待后重试（死锁恢复 / 启动竞态），不干等整个 interval。
		return time.Second
	}
	remaining := interval - r.clock.Now().UTC().Sub(last)
	if remaining < time.Second {
		return time.Second
	}
	return remaining
}

// ManualApplyWithProviderScope 执行管理页手动触发的写入任务。
func (r *Runtime) ManualApplyWithProviderScope(ctx context.Context, scope config.ProviderScope, providers []string) error {
	return r.run(ctx, TriggerManualApply, scope, providers, "", nil)
}

// ManualApplyWithProviderScopeAndModelGroup 执行管理页手动触发的指定 Antigravity 模型组写入任务。
func (r *Runtime) ManualApplyWithProviderScopeAndModelGroup(ctx context.Context, scope config.ProviderScope, providers []string, modelGroup config.AntigravityModelGroup) error {
	return r.run(ctx, TriggerManualApply, scope, providers, modelGroup, nil)
}

// ManualApplyWithProviderScopeModelGroupAndAuthIndexes 执行管理页单凭证重试写入任务。
func (r *Runtime) ManualApplyWithProviderScopeModelGroupAndAuthIndexes(ctx context.Context, scope config.ProviderScope, providers []string, modelGroup config.AntigravityModelGroup, authIndexes []string) error {
	return r.run(ctx, TriggerManualApply, scope, providers, modelGroup, authIndexes)
}

// RunWithProviderScopeAndModelGroup 执行管理页手动触发的指定 Antigravity 模型组 dry-run 任务。
func (r *Runtime) RunWithProviderScopeAndModelGroup(ctx context.Context, scope config.ProviderScope, providers []string, modelGroup config.AntigravityModelGroup) error {
	return r.run(ctx, TriggerManual, scope, providers, modelGroup, nil)
}

// Shutdown 停止 ticker、取消 runtime context，并等待内部 worker 退出。
func (r *Runtime) Shutdown(ctx context.Context) error {
	r.mu.Lock()
	if r.shutdown {
		r.mu.Unlock()
		return nil
	}
	r.shutdown = true
	r.cancel()
	worker := r.worker
	r.worker = nil
	r.mu.Unlock()
	return stopWorker(ctx, worker)
}

// Config 返回当前已验证配置快照。
func (r *Runtime) Config() (config.Config, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.shutdown {
		return config.Config{}, ErrShutdown
	}
	return r.cfg, nil
}

func (r *Runtime) replaceConfig(ctx context.Context, cfg config.Config) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("runtime configure context: %w", err)
	}

	r.mu.Lock()
	if r.shutdown {
		r.mu.Unlock()
		return ErrShutdown
	}
	oldCfg := r.cfg
	oldWorker := r.worker
	// 仅当调度相关配置变化时才重启 worker，避免 reconfigure 取消进行中的 AutoApply。
	needRestartWorker := oldWorker == nil ||
		oldCfg.Enabled != cfg.Enabled ||
		oldCfg.AutoApply != cfg.AutoApply ||
		oldCfg.Interval != cfg.Interval
	r.cfg = cfg
	r.mu.Unlock()

	if !needRestartWorker {
		return nil
	}

	worker := r.newWorker(cfg)
	r.mu.Lock()
	if r.shutdown {
		r.mu.Unlock()
		return stopNewWorker(worker, ErrShutdown)
	}
	oldWorker = r.worker
	r.worker = worker
	// 必须先释放 mu 再 start：worker 内 AutoApply → taskContext 需要 r.mu。
	r.mu.Unlock()
	if worker != nil {
		worker.start(r.rootCtx, r)
	}
	return stopWorker(ctx, oldWorker)
}

func (r *Runtime) newWorker(cfg config.Config) *tickerWorker {
	if !cfg.Enabled || !cfg.AutoApply {
		return nil
	}
	return &tickerWorker{interval: cfg.Interval, ticker: r.tickerFactory.NewTicker(cfg.Interval), done: make(chan struct{})}
}

func (r *Runtime) run(ctx context.Context, trigger Trigger, scope config.ProviderScope, providers []string, modelGroup config.AntigravityModelGroup, authIndexes []string) error {
	if !r.runMu.TryLock() {
		return ErrRunInProgress
	}
	defer r.runMu.Unlock()
	taskCtx, cleanup, cfg, runner, err := r.taskContext(ctx)
	if err != nil {
		return err
	}
	defer cleanup()
	if scope == config.ProviderScopeAll {
		cfg.ProviderScope = config.ProviderScopeAll
		cfg.SelectedProviders = nil
	} else if len(providers) > 0 {
		cfg.ProviderScope = config.ProviderScopeSelected
		cfg.SelectedProviders = append([]string(nil), providers...)
	}
	if modelGroup != "" {
		cfg.AntigravityModelGroup = modelGroup
	}
	if err := runner(taskCtx, TaskRequest{Config: cfg, Trigger: trigger, AuthIndexes: append([]string(nil), authIndexes...)}); err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("run %s: %w", trigger, err)
	}
	return nil
}

func (r *Runtime) taskContext(ctx context.Context) (context.Context, func(), config.Config, TaskRunner, error) {
	r.mu.Lock()
	if r.shutdown {
		r.mu.Unlock()
		return nil, nil, config.Config{}, nil, ErrShutdown
	}
	rootCtx, cfg, runner := r.rootCtx, r.cfg, r.runner
	r.mu.Unlock()
	taskCtx, cancel := context.WithCancel(rootCtx)
	stop := context.AfterFunc(ctx, cancel)
	cleanup := func() {
		stop()
		cancel()
	}
	return taskCtx, cleanup, cfg, runner, nil
}
