package service

import (
	"sync"
	"testing"
)

func TestEventBusPublishConcurrentUnsubscribe(t *testing.T) {
	bus := NewEventBus(8)
	const n = 32
	subs := make([]chan map[string]any, 0, n)
	for i := 0; i < n; i++ {
		subs = append(subs, bus.Subscribe())
	}

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				bus.Publish(map[string]any{"seq": j, "sub": i})
			}
		}(i)
	}
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(ch chan map[string]any) {
			defer wg.Done()
			bus.Unsubscribe(ch)
		}(subs[i])
	}
	wg.Wait()
}
