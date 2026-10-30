package sessionlog

import (
	"errors"
	"fmt"
	"sync"
)

// queueBuffer bounds the operations waiting for the writer. Submit blocks when
// the buffer is full, which backpressures a caller instead of growing without
// limit; it never waits for a database commit.
const queueBuffer = 64

// queuedOperation is one operation and the channel that reports its result.
type queuedOperation struct {
	operation Operation
	ack       chan error
}

// Queue runs the store's writes on one goroutine in submission order, so the UI
// event path never waits for SQLite. It has no callback hooks: the returned
// acknowledgment channel is the only notification, and no database work runs on
// the caller's goroutine.
//
// An acknowledgment channel is buffered for one result and then closed, so a
// caller may ignore it without blocking the writer. Close drains every submitted
// operation, stops the writer, and returns the first error the writer saw. Queue
// does not close its Store: close the Queue first (so pending writes land), then
// the Store. Submit after Close returns a closed channel holding ErrQueueClosed.
type Queue struct {
	store *Store
	ops   chan queuedOperation
	done  chan struct{}

	mu        sync.Mutex
	closed    bool
	firstErr  error
	closeOnce sync.Once
	closeErr  error
}

// NewQueue starts an ordered writer for store.
func NewQueue(store *Store) *Queue {
	queue := &Queue{
		store: store,
		ops:   make(chan queuedOperation, queueBuffer),
		done:  make(chan struct{}),
	}
	go queue.run()
	return queue
}

func (q *Queue) run() {
	defer close(q.done)
	for item := range q.ops {
		err := q.store.apply(item.operation)
		if err != nil {
			q.mu.Lock()
			q.firstErr = errors.Join(q.firstErr, err)
			q.mu.Unlock()
		}
		item.ack <- err
		close(item.ack)
	}
}

// Submit queues one operation. Operations are applied in submission order, also
// when many goroutines submit. The returned channel receives exactly one error
// and is then closed. Submit never blocks for the database, only while the
// bounded queue is full.
func (q *Queue) Submit(operation Operation) <-chan error {
	ack := make(chan error, 1)
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		ack <- ErrQueueClosed
		close(ack)
		return ack
	}
	// Hold the mutex across the send so Close cannot close the channel under an
	// in-flight Submit; the writer keeps consuming, so this does not deadlock.
	q.ops <- queuedOperation{operation: operation, ack: ack}
	q.mu.Unlock()
	return ack
}

// Close flushes the submitted operations, stops the writer and returns the first
// write error it saw. It blocks until every operation that was submitted before
// the close has been applied. It is safe to call more than once, and it does not
// close the Store.
func (q *Queue) Close() error {
	q.closeOnce.Do(func() {
		q.mu.Lock()
		q.closed = true
		close(q.ops)
		q.mu.Unlock()
		<-q.done
		q.mu.Lock()
		q.closeErr = q.firstErr
		q.mu.Unlock()
	})
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.closeErr
}

// apply performs one operation. The Queue calls it only from its writer.
func (s *Store) apply(operation Operation) error {
	switch operation.Kind {
	case OpStart:
		return s.Start(operation.Record)
	case OpAppendEvent:
		return s.AppendEvent(operation.Event)
	case OpFinish:
		return s.Finish(operation.ID, operation.Status, operation.EndedAt, operation.Reason)
	case OpSetFavorite:
		return s.SetFavorite(operation.ID, operation.Favorite)
	case OpDelete:
		return s.Delete(operation.ID)
	case OpClear:
		return s.Clear(operation.IncludeFavorites)
	default:
		return fmt.Errorf("%w: operation kind %d", ErrInvalidRecord, operation.Kind)
	}
}
