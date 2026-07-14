package concurrency

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewPool(t *testing.T) {
	tests := []struct {
		name       string
		maxWorkers int
		wantErr    bool
	}{
		{"Valid pool size", 10, false},
		{"Zero workers", 0, true},
		{"Negative workers", -1, true},
		{"Single worker", 1, false},
		{"Large pool", 100, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool, err := NewPool(tt.maxWorkers)

			if (err != nil) != tt.wantErr {
				t.Errorf("NewPool(%d) error = %v, wantErr %v",
					tt.maxWorkers, err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if pool == nil {
					t.Error("NewPool() returned nil pool")
				} else {
					pool.Close() // Clean up
				}
			}
		})
	}
}

func TestPool_Run(t *testing.T) {
	pool, err := NewPool(2)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer pool.Close()

	var counter int32

	// Run multiple tasks
	for i := 0; i < 5; i++ {
		err := pool.Run(func() {
			atomic.AddInt32(&counter, 1)
			time.Sleep(10 * time.Millisecond)
		})

		if err != nil {
			t.Errorf("Run() error = %v, want nil", err)
		}
	}

	// Wait for tasks to complete
	time.Sleep(100 * time.Millisecond)

	if atomic.LoadInt32(&counter) != 5 {
		t.Errorf("Counter = %d, want %d", counter, 5)
	}
}

func TestPool_Close(t *testing.T) {
	pool, err := NewPool(2)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}

	var taskRun bool
	err = pool.Run(func() {
		taskRun = true
	})
	if err != nil {
		t.Errorf("Run() error = %v, want nil", err)
	}

	// Close the pool
	pool.Close()

	// Try to run after close - should get error
	err = pool.Run(func() {
		t.Error("Task should not run after pool is closed")
	})

	if err != ErrPoolClosed {
		t.Errorf("Run() after close error = %v, want %v", err, ErrPoolClosed)
	}

	// Give original task time to complete
	time.Sleep(50 * time.Millisecond)

	if !taskRun {
		t.Error("Original task did not run")
	}
}

func TestPool_WorkerLimit(t *testing.T) {
	const maxWorkers = 3
	pool, err := NewPool(maxWorkers)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer pool.Close()

	var activeWorkers int32
	var maxActive int32

	// Run more tasks than workers
	for i := 0; i < maxWorkers*2; i++ {
		err := pool.Run(func() {
			current := atomic.AddInt32(&activeWorkers, 1)
			defer atomic.AddInt32(&activeWorkers, -1)

			// Track maximum active workers
			for {
				old := atomic.LoadInt32(&maxActive)
				if current <= old {
					break
				}
				if atomic.CompareAndSwapInt32(&maxActive, old, current) {
					break
				}
			}

			time.Sleep(50 * time.Millisecond)
		})

		if err != nil {
			t.Errorf("Run() error = %v, want nil", err)
		}
	}

	// Wait for all tasks to complete
	time.Sleep(200 * time.Millisecond)

	actualMax := atomic.LoadInt32(&maxActive)
	if actualMax > maxWorkers {
		t.Errorf("Maximum active workers = %d, want <= %d", actualMax, maxWorkers)
	}
}

func TestPool_BufferFull(t *testing.T) {
	// Create pool with small buffer
	pool, err := NewPool(1)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer pool.Close()

	// Fill the buffer with slow tasks
	for i := 0; i < 20; i++ {
		pool.Run(func() {
			time.Sleep(100 * time.Millisecond)
		})
	}

	// Try to add one more - might get buffer full error
	// Note: This is timing dependent, so we just test it doesn't panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Pool.Run() panicked: %v", r)
		}
	}()

	// This might succeed or return buffer full error
	_ = pool.Run(func() {})
}

func TestPool_ConcurrentUse(t *testing.T) {
	pool, err := NewPool(5)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer pool.Close()

	var counter int32
	var wg sync.WaitGroup

	// Run many concurrent tasks
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := pool.Run(func() {
				atomic.AddInt32(&counter, 1)
				time.Sleep(time.Millisecond)
			})
			if err != nil && err != ErrPoolClosed {
				t.Errorf("Run() error = %v, want nil", err)
			}
		}()
	}

	wg.Wait()
	time.Sleep(50 * time.Millisecond)

	// All tasks should have run
	if atomic.LoadInt32(&counter) != 50 {
		t.Errorf("Counter = %d, want %d", counter, 50)
	}
}
