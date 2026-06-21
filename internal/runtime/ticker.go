package runtime

import (
	"context"
	"fmt"
	"time"
)

type tickerWorker struct {
	ticker Ticker
	cancel context.CancelFunc
	done   chan struct{}
}

func (w *tickerWorker) start(rootCtx context.Context, rt *Runtime) {
	ctx, cancel := context.WithCancel(rootCtx)
	w.cancel = cancel
	go func() {
		defer close(w.done)
		for {
			select {
			case <-w.ticker.Chan():
				_ = rt.AutoApply(ctx)
			case <-ctx.Done():
				return
			}
		}
	}()
}

func stopWorker(ctx context.Context, worker *tickerWorker) error {
	if worker == nil {
		return nil
	}
	worker.ticker.Stop()
	worker.cancel()
	select {
	case <-worker.done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("wait ticker worker: %w", ctx.Err())
	}
}

func stopNewWorker(worker *tickerWorker, err error) error {
	if worker != nil {
		worker.ticker.Stop()
	}
	return err
}

type timeTickerFactory struct{}

func (timeTickerFactory) NewTicker(interval time.Duration) Ticker {
	return timeTicker{ticker: time.NewTicker(interval)}
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
