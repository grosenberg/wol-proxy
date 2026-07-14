package concurrency

import (
	"errors"
	"sync"
)

// Pool represents a worker pool for concurrent task execution
type Pool struct {
	maxWorkers int
	tasks      chan func()
	wg         sync.WaitGroup
	mu         sync.Mutex
	closed     bool
}

// ErrPoolClosed is returned when trying to run a task on a closed pool
var ErrPoolClosed = errors.New("pool is closed")

// NewPool creates a new worker pool with the specified number of workers
func NewPool(maxWorkers int) (*Pool, error) {
	if maxWorkers <= 0 {
		return nil, errors.New("maxWorkers must be greater than 0")
	}

	pool := &Pool{
		maxWorkers: maxWorkers,
		tasks:      make(chan func(), maxWorkers*10),
	}

	// Start workers
	for i := 0; i < maxWorkers; i++ {
		pool.wg.Add(1)
		go pool.worker(i)
	}

	return pool, nil
}

// worker processes tasks from the channel
func (p *Pool) worker(id int) {
	defer p.wg.Done()

	for task := range p.tasks {
		task()
	}
}

// Run submits a task to the pool
func (p *Pool) Run(task func()) error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return ErrPoolClosed
	}
	p.mu.Unlock()

	select {
	case p.tasks <- task:
		return nil
	default:
		// Channel buffer full - this is a simple implementation
		// In production, you might want to handle this differently
		return errors.New("pool task buffer full")
	}
}

// Close waits for all tasks to complete and shuts down the pool
func (p *Pool) Close() {
	p.mu.Lock()
	if !p.closed {
		p.closed = true
		close(p.tasks)
	}
	p.mu.Unlock()

	p.wg.Wait()
}
