// 本文件主要内容：提供可去重的异步任务管理器。
package asyncjob

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

type Status string

const (
	StatusQueued  Status = "queued"
	StatusRunning Status = "running"
	StatusDone    Status = "done"
	StatusFailed  Status = "failed"
)

type Snapshot[T any] struct {
	ID         string
	Key        string
	Status     Status
	CreatedAt  time.Time
	StartedAt  time.Time
	FinishedAt time.Time
	Value      T
	Err        error
}

type Manager[T any] struct {
	mu   sync.Mutex
	jobs map[string]*job[T]
}

type job[T any] struct {
	id         string
	key        string
	status     Status
	createdAt  time.Time
	startedAt  time.Time
	finishedAt time.Time
	value      T
	err        error
}

func NewManager[T any]() *Manager[T] {
	return &Manager[T]{jobs: make(map[string]*job[T])}
}

func (m *Manager[T]) Enqueue(ctx context.Context, key string, fn func(context.Context, string) (T, error)) (Snapshot[T], bool, error) {
	if strings.TrimSpace(key) == "" {
		return Snapshot[T]{}, false, fmt.Errorf("job key is required")
	}
	if fn == nil {
		return Snapshot[T]{}, false, fmt.Errorf("job function is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	m.mu.Lock()
	if m.jobs == nil {
		m.jobs = make(map[string]*job[T])
	}
	if existing, ok := m.jobs[key]; ok {
		snap := snapshotFrom(existing)
		m.mu.Unlock()
		return snap, false, nil
	}
	j := &job[T]{
		id:        uuid.NewString(),
		key:       key,
		status:    StatusQueued,
		createdAt: time.Now(),
	}
	m.jobs[key] = j
	snap := snapshotFrom(j)
	m.mu.Unlock()

	go m.runJob(ctx, j, fn)
	return snap, true, nil
}

func (m *Manager[T]) runJob(ctx context.Context, j *job[T], fn func(context.Context, string) (T, error)) {
	m.mu.Lock()
	j.status = StatusRunning
	j.startedAt = time.Now()
	m.mu.Unlock()

	value, err := fn(ctx, j.id)

	m.mu.Lock()
	j.finishedAt = time.Now()
	if err != nil {
		j.status = StatusFailed
		j.err = err
	} else {
		j.status = StatusDone
		j.value = value
	}
	// Only dedupe in-flight jobs. Completed jobs should not block new requests.
	if m.jobs != nil {
		if existing, ok := m.jobs[j.key]; ok && existing == j {
			delete(m.jobs, j.key)
		}
	}
	m.mu.Unlock()
}

func snapshotFrom[T any](j *job[T]) Snapshot[T] {
	if j == nil {
		return Snapshot[T]{}
	}
	return Snapshot[T]{
		ID:         j.id,
		Key:        j.key,
		Status:     j.status,
		CreatedAt:  j.createdAt,
		StartedAt:  j.startedAt,
		FinishedAt: j.finishedAt,
		Value:      j.value,
		Err:        j.err,
	}
}
