package scheduler

import (
	"sync"
)

type Task func()

type Pool struct {
	wg          sync.WaitGroup
	tasks       chan Task
	concurrency int
}

func NewPool(concurrency int) *Pool {
	if concurrency < 1 {
		concurrency = 1
	}
	p := &Pool{
		tasks:       make(chan Task, concurrency*4),
		concurrency: concurrency,
	}
	p.start()
	return p
}

func (p *Pool) start() {
	for i := 0; i < p.concurrency; i++ {
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			for task := range p.tasks {
				if task != nil {
					task()
				}
			}
		}()
	}
}

func (p *Pool) Submit(task Task) {
	p.tasks <- task
}

func (p *Pool) Wait() {
	close(p.tasks)
	p.wg.Wait()
}

type Runner struct {
	pool *Pool
	wg   sync.WaitGroup
}

func NewRunner(concurrency int) *Runner {
	return &Runner{
		pool: NewPool(concurrency),
	}
}

func (r *Runner) Go(fn func()) {
	r.wg.Add(1)
	r.pool.Submit(func() {
		defer r.wg.Done()
		fn()
	})
}

func (r *Runner) Wait() {
	r.wg.Wait()
	r.pool.Wait()
}
