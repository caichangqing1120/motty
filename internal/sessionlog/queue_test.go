package sessionlog

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

func awaitAck(t *testing.T, ack <-chan error) error {
	t.Helper()
	select {
	case err, ok := <-ack:
		if !ok {
			t.Fatal("ack channel closed without a result")
		}
		return err
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for a queue acknowledgment")
		return nil
	}
}

func TestQueueOrdersStartBeforeFinish(t *testing.T) {
	store, _ := openTestStore(t)
	queue := NewQueue(store)
	t.Cleanup(func() { _ = queue.Close() })
	base := testBase()
	id := newUniqueID(t)

	acks := []<-chan error{
		queue.Submit(Operation{Kind: OpStart, Record: sampleRecord(id, base)}),
		queue.Submit(Operation{Kind: OpAppendEvent, Event: Event{RecordID: id, Kind: EventConnected, At: base.Add(time.Second)}}),
		queue.Submit(Operation{Kind: OpFinish, ID: id, Status: StatusEnded, EndedAt: base.Add(time.Minute), Reason: ReasonUserClosed}),
	}
	for i, ack := range acks {
		if err := awaitAck(t, ack); err != nil {
			t.Fatalf("queued operation %d: %v", i, err)
		}
	}

	got := findRecord(t, store, id)
	if got.Status != StatusEnded || !got.EndedAt.Equal(base.Add(time.Minute)) || got.Reason != ReasonUserClosed {
		t.Fatalf("record after queued start/event/finish = %+v", got)
	}
	events, err := store.Events(id)
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	if len(events) != 1 || events[0].Kind != EventConnected {
		t.Fatalf("events after queued writes = %+v", events)
	}
	if err := queue.Close(); err != nil {
		t.Fatalf("Queue.Close: %v", err)
	}
}

func TestQueueKeepsPerCallerOrder(t *testing.T) {
	store, _ := openTestStore(t)
	queue := NewQueue(store)
	t.Cleanup(func() { _ = queue.Close() })
	base := testBase()

	var acks []<-chan error
	for i := 0; i < 20; i++ {
		id := fmt.Sprintf("ordered-%02d", i)
		acks = append(acks,
			queue.Submit(Operation{Kind: OpStart, Record: sampleRecord(id, base.Add(time.Duration(i)*time.Millisecond))}),
			queue.Submit(Operation{Kind: OpFinish, ID: id, Status: StatusEnded, EndedAt: base.Add(time.Hour), Reason: ReasonUserClosed}),
		)
	}
	for i, ack := range acks {
		if err := awaitAck(t, ack); err != nil {
			t.Fatalf("queued operation %d: %v", i, err)
		}
	}
	if err := queue.Close(); err != nil {
		t.Fatalf("Queue.Close: %v", err)
	}
	for i := 0; i < 20; i++ {
		id := fmt.Sprintf("ordered-%02d", i)
		if got := findRecord(t, store, id); got.Status != StatusEnded {
			t.Fatalf("%q status = %v, want ended: its finish preceded its start", id, got.Status)
		}
	}
}

func TestQueueAcksAreBufferedAndClosedEvenIfIgnored(t *testing.T) {
	store, _ := openTestStore(t)
	queue := NewQueue(store)
	t.Cleanup(func() { _ = queue.Close() })
	base := testBase()
	id := newUniqueID(t)

	if err := awaitAck(t, queue.Submit(Operation{Kind: OpStart, Record: sampleRecord(id, base)})); err != nil {
		t.Fatalf("start: %v", err)
	}
	ignored := queue.Submit(Operation{Kind: OpFinish, ID: id, Status: StatusEnded, EndedAt: base.Add(time.Minute), Reason: ReasonUserClosed})
	// The writer must not block on an acknowledgment nobody reads.
	if err := queue.Close(); err != nil {
		t.Fatalf("Queue.Close with an ignored ack: %v", err)
	}

	select {
	case err, ok := <-ignored:
		if !ok {
			t.Fatal("ignored ack closed without a result")
		}
		if err != nil {
			t.Fatalf("ignored ack error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ignored ack was never delivered")
	}
	if _, ok := <-ignored; ok {
		t.Fatal("ack channel delivered more than one result")
	}
	if got := findRecord(t, store, id); got.Status != StatusEnded {
		t.Fatalf("drained record = %+v, want ended", got)
	}
}

func TestQueueCloseDrainsPendingWrites(t *testing.T) {
	store, _ := openTestStore(t)
	queue := NewQueue(store)
	base := testBase()
	id := newUniqueID(t)

	// No acknowledgment is read before Close: Close drains what is pending.
	queue.Submit(Operation{Kind: OpStart, Record: sampleRecord(id, base)})
	queue.Submit(Operation{Kind: OpAppendEvent, Event: Event{RecordID: id, Kind: EventConnected, At: base.Add(time.Second)}})
	if err := queue.Close(); err != nil {
		t.Fatalf("Queue.Close: %v", err)
	}
	if got := findRecord(t, store, id); got.Status != StatusActive {
		t.Fatalf("record after draining Close = %+v, want active", got)
	}
	events, err := store.Events(id)
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("drained events = %d, want 1", len(events))
	}
}

func TestQueueConcurrentSubmissionsAreAllApplied(t *testing.T) {
	store, _ := openTestStore(t)
	queue := NewQueue(store)
	t.Cleanup(func() { _ = queue.Close() })
	base := testBase()
	const count = 40
	acks := make([]<-chan error, count)
	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprintf("concurrent-queue-%02d", i)
			acks[i] = queue.Submit(Operation{Kind: OpStart, Record: sampleRecord(id, base.Add(time.Duration(i)*time.Millisecond))})
		}(i)
	}
	wg.Wait()
	for i, ack := range acks {
		if err := awaitAck(t, ack); err != nil {
			t.Fatalf("submission %d: %v", i, err)
		}
	}
	if err := queue.Close(); err != nil {
		t.Fatalf("Queue.Close: %v", err)
	}
	if got := countRows(t, store, ""); got != count {
		t.Fatalf("records after concurrent submissions = %d, want %d", got, count)
	}
}

func TestQueueSubmitAfterCloseReturnsClosedError(t *testing.T) {
	store, _ := openTestStore(t)
	queue := NewQueue(store)
	if err := queue.Close(); err != nil {
		t.Fatalf("Queue.Close: %v", err)
	}
	ack := queue.Submit(Operation{Kind: OpClear})
	select {
	case err, ok := <-ack:
		if !ok {
			t.Fatal("post-Close ack closed without a result")
		}
		if !errors.Is(err, ErrQueueClosed) {
			t.Fatalf("post-Close ack error = %v, want ErrQueueClosed", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("post-Close Submit did not return a result")
	}
	if _, ok := <-ack; ok {
		t.Fatal("post-Close ack channel delivered more than one result")
	}
	if err := queue.Close(); err != nil {
		t.Fatalf("second Queue.Close: %v", err)
	}
}

func TestQueueReportsWriteErrors(t *testing.T) {
	store, _ := openTestStore(t)
	queue := NewQueue(store)
	bad := Record{ID: newUniqueID(t), RunID: "run-1", Host: "", Protocol: ProtocolSSH, StartedAt: testBase()}
	if err := awaitAck(t, queue.Submit(Operation{Kind: OpStart, Record: bad})); !errors.Is(err, ErrInvalidRecord) {
		t.Fatalf("queued invalid start error = %v, want ErrInvalidRecord", err)
	}
	if err := awaitAck(t, queue.Submit(Operation{Kind: OperationKind(99)})); err == nil {
		t.Fatal("queued unknown operation reported no error")
	}
	if err := queue.Close(); err == nil {
		t.Fatal("Queue.Close did not report the failed writes")
	}
}
