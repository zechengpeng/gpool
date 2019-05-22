package gpool

import (
	"errors"
	"sync"
	"sync/atomic"
)

type Task func() error

type GPool struct {
	size     int32
	running  int32
	released int32

	idle chan struct{}

	workers      []*Worker
	mu           sync.Mutex
	bufferWorker sync.Pool
	cond         *sync.Cond
}

func NewPool(size int32) *GPool {
	pool := &GPool{
		size:    size,
		workers: make([]*Worker, 0),
		idle:    make(chan struct{}, 1),
	}

	pool.cond = sync.NewCond(&pool.mu)
	return pool
}

func (p *GPool) Assign(task Task) error {
	if atomic.LoadInt32(&p.released) == 1 {
		return errors.New("this pool has been closed")
	}

	p.getWorker().task <- task
	return nil
}

func (p *GPool) Stop() {
	atomic.StoreInt32(&p.released, 1)

	p.mu.Lock()
	defer p.mu.Unlock()
	for _, worker := range p.workers {
		worker.task <- nil
	}
	p.workers = nil
}

func (p *GPool) getWorker() *Worker {
	var w *Worker

	p.mu.Lock()
	defer p.mu.Unlock()

	n := len(p.workers) - 1
	if n >= 0 {
		w = p.workers[n]
		p.workers = p.workers[:n]
		return w
	}

	waiting := p.Running() >= p.Cap()
	if waiting {
		for {
			//waiting for free worker
			p.cond.Wait()
			n = len(p.workers) - 1
			if n < 0 {
				continue
			}
			w = p.workers[n]
			p.workers = p.workers[:n]
			break
		}
	} else {
		if buf := p.bufferWorker.Get(); buf != nil {
			w = buf.(*Worker)
		} else {
			w = &Worker{
				pool: p,
				task: make(chan Task, 1),
			}
		}
		w.Run()
		p.IncRunning()
	}

	return w
}

func (p *GPool) freeWorker(w *Worker) bool {
	if atomic.LoadInt32(&p.released) == 1 {
		return false
	}

	p.mu.Lock()
	p.workers = append(p.workers, w)
	p.mu.Unlock()
	p.cond.Signal()

	//p.mu.Lock()
	//if len(p.workers) == int(p.size) {
	//	p.idle <- struct{}{}
	//}
	//p.mu.Unlock()

	return true
}

func (p *GPool) Running() int32 {
	return atomic.LoadInt32(&p.running)
}

func (p *GPool) Cap() int32 {
	return atomic.LoadInt32(&p.size)
}

func (p *GPool) DecRunning() int32 {
	return atomic.AddInt32(&p.running, -1)
}

func (p *GPool) IncRunning() int32 {
	return atomic.AddInt32(&p.running, 1)
}
