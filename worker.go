package gpool

type Worker struct {
	pool *GPool
	task chan Task
}

func (w *Worker) Run() {
	go func() {
		for fn := range w.task {
			if fn == nil {
				w.pool.DecRunning()
				w.pool.bufferWorker.Put(w)
				return
			}

			fn()
			//free
			if succ := w.pool.freeWorker(w); !succ {
				return
			}
		}
	}()
}
