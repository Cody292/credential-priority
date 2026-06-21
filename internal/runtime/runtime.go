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

// Runtime 持有 CPA 插件生命周期、配置、ticker 和 single-flight 状态。
type Runtime struct {
	mu            sync.Mutex
	runMu         sync.Mutex
	tickerFactory TickerFactory
	runner        TaskRunner
	rootCtx       context.Context
	cancel        context.CancelFunc
	cfg           config.Config
	hostCallbacks host.HostCallbacks
	clock         Clock
	management    *management.Handler
	latestResult  apply.Result
	latestAudit   string
	worker        *tickerWorker
	shutdown      bool
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
	ctx, cancel := context.WithCancel(context.Background())
	runtime := &Runtime{tickerFactory: factory, rootCtx: ctx, cancel: cancel, cfg: config.Default(), hostCallbacks: options.Host, clock: clock}
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
	default:
		return failure(fmt.Errorf("%w: method %q", ErrInvalidRequest, method))
	}
}

func (r *Runtime) snapshotRun(result apply.Result, audit string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.latestResult = result
	r.latestAudit = audit
}

func (r *Runtime) currentRunSnapshot() (apply.Result, string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.latestResult, r.latestAudit
}

type realRuntimeClock struct{}

func (realRuntimeClock) Now() time.Time {
	return time.Now().UTC()
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
			Version:          "0.1.0",
			Author:           "CPA Plugins",
			GitHubRepository: "https://github.com/Cody292/credential-priority",
			Description:      "Fresh evidence based credential priority management API.",
			ConfigFields:     registerConfigFields(),
		},
		Capabilities: map[string]bool{"management_api": true},
	}
}

func registerConfigFields() []ConfigField {
	return []ConfigField{
		{Name: "enabled", Type: "boolean", Description: "Enable credential priority management."},
		{Name: "auto_apply", Type: "boolean", Description: "Automatically apply priority updates on the configured interval."},
		{Name: "interval", Type: "string", Description: "Automatic run interval, for example 5m."},
		{Name: "max_concurrency", Type: "integer", Description: "Maximum concurrent provider probes."},
		{Name: "min_change", Type: "integer", Description: "Minimum priority delta required before writing changes."},
		{Name: "top_priority_probe_count", Type: "integer", Description: "Number of top-priority credentials to probe."},
		{Name: "active_group_size", Type: "integer", Description: "Credential count in the active priority group."},
		{Name: "active_group_jitter", Type: "string", Description: "Priority jitter duration for active credentials."},
		{Name: "disabled_group_size", Type: "integer", Description: "Credential count in the disabled probe group."},
		{Name: "disabled_probe_interval", Type: "string", Description: "Minimum duration before probing disabled credentials again."},
		{Name: "cache_ttl", Type: "string", Description: "Fresh probe cache lifetime."},
		{Name: "cache_path", Type: "string", Description: "Path to the refresh cache JSON file."},
		{Name: "provider_overrides", Type: "object", Description: "Optional per-provider overrides."},
	}
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
	return r.run(ctx, TriggerManual)
}

// AutoApply 执行一轮自动任务；若已有任务运行则返回 ErrRunInProgress。
func (r *Runtime) AutoApply(ctx context.Context) error {
	return r.run(ctx, TriggerAutoApply)
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
	worker := r.newWorker(cfg)
	r.mu.Lock()
	if r.shutdown {
		r.mu.Unlock()
		return stopNewWorker(worker, ErrShutdown)
	}
	oldWorker := r.worker
	r.cfg = cfg
	r.worker = worker
	if worker != nil {
		worker.start(r.rootCtx, r)
	}
	r.mu.Unlock()
	return stopWorker(ctx, oldWorker)
}

func (r *Runtime) newWorker(cfg config.Config) *tickerWorker {
	if !cfg.Enabled || !cfg.AutoApply {
		return nil
	}
	return &tickerWorker{ticker: r.tickerFactory.NewTicker(cfg.Interval), done: make(chan struct{})}
}

func (r *Runtime) run(ctx context.Context, trigger Trigger) error {
	if !r.runMu.TryLock() {
		return ErrRunInProgress
	}
	defer r.runMu.Unlock()
	taskCtx, cleanup, cfg, runner, err := r.taskContext(ctx)
	if err != nil {
		return err
	}
	defer cleanup()
	if err := runner(taskCtx, TaskRequest{Config: cfg, Trigger: trigger}); err != nil && !errors.Is(err, context.Canceled) {
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
