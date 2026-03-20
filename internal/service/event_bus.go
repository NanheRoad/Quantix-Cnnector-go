package service

import (
	"sync"
	"sync/atomic"
)

type EventBus struct {
	mu      sync.Mutex
	queues  map[chan map[string]any]struct{}
	maxSize int
	dropped atomic.Uint64
	sent    atomic.Uint64
}

func NewEventBus(maxSize int) *EventBus {
	if maxSize <= 0 {
		maxSize = 200
	}
	return &EventBus{queues: map[chan map[string]any]struct{}{}, maxSize: maxSize}
}

func (b *EventBus) Subscribe() chan map[string]any {
	ch := make(chan map[string]any, b.maxSize)
	b.mu.Lock()
	b.queues[ch] = struct{}{}
	b.mu.Unlock()
	return ch
}

func (b *EventBus) Unsubscribe(ch chan map[string]any) {
	b.mu.Lock()
	if _, ok := b.queues[ch]; ok {
		delete(b.queues, ch)
	}
	b.mu.Unlock()
}

func (b *EventBus) Publish(message map[string]any) {
	b.sent.Add(1)
	b.mu.Lock()
	targets := make([]chan map[string]any, 0, len(b.queues))
	for ch := range b.queues {
		targets = append(targets, ch)
	}
	b.mu.Unlock()
	for _, ch := range targets {
		msg := cloneBusMessage(message)
		sent, panicked := trySend(ch, msg)
		if panicked {
			b.dropped.Add(1)
			continue
		}
		if sent {
			continue
		}
		droppedOne, dropPanicked := tryDropOldest(ch)
		if dropPanicked {
			b.dropped.Add(1)
			continue
		}
		if droppedOne {
			b.dropped.Add(1)
		}
		sent, panicked = trySend(ch, msg)
		if panicked || !sent {
			b.dropped.Add(1)
		}
	}
}

func trySend(ch chan map[string]any, msg map[string]any) (sent bool, panicked bool) {
	defer func() {
		if recover() != nil {
			sent = false
			panicked = true
		}
	}()
	select {
	case ch <- msg:
		return true, false
	default:
		return false, false
	}
}

func tryDropOldest(ch chan map[string]any) (dropped bool, panicked bool) {
	defer func() {
		if recover() != nil {
			dropped = false
			panicked = true
		}
	}()
	select {
	case <-ch:
		return true, false
	default:
		return false, false
	}
}

func (b *EventBus) Stats() map[string]any {
	b.mu.Lock()
	queueCount := len(b.queues)
	b.mu.Unlock()
	return map[string]any{
		"queues":        queueCount,
		"queue_size":    b.maxSize,
		"sent":          b.sent.Load(),
		"dropped":       b.dropped.Load(),
		"drop_strategy": "drop_oldest",
	}
}

func cloneBusMessage(in map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range in {
		out[k] = v
	}
	return out
}
