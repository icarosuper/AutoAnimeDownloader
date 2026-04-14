package daemon

import (
	"AutoAnimeDownloader/src/internal/logger"
	"context"
	"sync"
	"time"
)

type StartLoopPayload struct {
	FileManager FileManagerInterface
	Interval    time.Duration
	State       *State
	JobQueue    *JobQueue
}

type LoopControl struct {
	UpdateInterval func(time.Duration)
	Cancel         context.CancelFunc
	Done           <-chan struct{}
}

type StartLoopFuncType func(StartLoopPayload) *LoopControl

func createStartFunc(p StartLoopPayload) func(d time.Duration, c context.Context) chan struct{} {
	return func(d time.Duration, c context.Context) chan struct{} {
		done := make(chan struct{})
		go func() {
			defer close(done)
			select {
			case <-c.Done():
				logger.Logger.Info().Msg("Verification loop cancelled before start")
				return
			default:
			}
			p.State.SetStatus(StatusRunning)

			for {
				select {
				case <-c.Done():
					logger.Logger.Info().Msg("Verification loop stopped")
					p.State.SetStatus(StatusStopped)
					return
				default:
				}

				p.State.SetStatus(StatusChecking)

				AnimeVerification(c, p.FileManager, p.State, p.JobQueue)

				select {
				case <-c.Done():
					logger.Logger.Info().Msg("Verification loop stopped")
					p.State.SetStatus(StatusStopped)
					return
				default:
					p.State.SetStatus(StatusRunning)
				}

				select {
				case <-time.After(d):
					continue
				case <-c.Done():
					logger.Logger.Info().Msg("Verification loop stopped")
					p.State.SetStatus(StatusStopped)
					return
				}
			}
		}()
		return done
	}
}

func StartLoop(p StartLoopPayload) *LoopControl {
	var mu sync.Mutex
	ctx, cancel := context.WithCancel(context.Background())

	cancelPtr := &cancel

	start := createStartFunc(p)

	doneCh := start(p.Interval, ctx)
	donePtr := &doneCh

	return &LoopControl{
		UpdateInterval: func(newDur time.Duration) {
			mu.Lock()
			defer mu.Unlock()
			(*cancelPtr)()
			ctx, cancel = context.WithCancel(context.Background())
			cancelPtr = &cancel
			newDone := start(newDur, ctx)
			donePtr = &newDone
		},
		Cancel: func() {
			mu.Lock()
			defer mu.Unlock()
			(*cancelPtr)()
		},
		Done: func() <-chan struct{} {
			mu.Lock()
			defer mu.Unlock()
			return *donePtr
		}(),
	}
}
