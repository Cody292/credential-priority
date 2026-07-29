package runtime

import (
	"context"
	"fmt"
	"time"
)

type tickerWorker struct {
	interval time.Duration
	ticker   Ticker // 可选；生产路径用 Sleep 调度，测试可注入 Ticker
	cancel   context.CancelFunc
	done     chan struct{}
}

func (w *tickerWorker) start(rootCtx context.Context, rt *Runtime) {
	ctx, cancel := context.WithCancel(rootCtx)
	w.cancel = cancel
	interval := w.interval
	if interval <= 0 {
		interval = 15 * time.Minute
	}
	go func() {
		defer close(w.done)
		// 延迟首轮：避免在 plugin.register/reconfigure 调用栈内立刻回调宿主（重入死锁）。
		// 之后：执行成功 → 等待完整 interval → 再执行。
		startupDelay := 2 * time.Second
		timer := time.NewTimer(startupDelay)
		select {
		case <-timer.C:
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return
		}
		for {
			if ctx.Err() != nil {
				return
			}
			_ = rt.AutoApply(ctx)
			wait := rt.nextAutoApplyWait(interval)
			timer = time.NewTimer(wait)
			select {
			case <-timer.C:
			case <-ctx.Done():
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				return
			}
		}
	}()
}

func stopWorker(ctx context.Context, worker *tickerWorker) error {
	if worker == nil {
		return nil
	}
	if worker.ticker != nil {
		worker.ticker.Stop()
	}
	if worker.cancel != nil {
		worker.cancel()
	}
	select {
	case <-worker.done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("wait ticker worker: %w", ctx.Err())
	}
}

func stopNewWorker(worker *tickerWorker, err error) error {
	if worker != nil {
		if worker.ticker != nil {
			worker.ticker.Stop()
		}
		if worker.cancel != nil {
			worker.cancel()
		}
	}
	return err
}

type timeTickerFactory struct{}

func (timeTickerFactory) NewTicker(interval time.Duration) Ticker {
	// 生产路径不再依赖周期性 Ticker 触发；保留接口兼容测试注入。
	return nil
}

type timeTicker struct {
	ticker *time.Ticker
}

func (t timeTicker) Chan() <-chan time.Time {
	return t.ticker.C
}

func (t timeTicker) Stop() {
	t.ticker.Stop()
}
