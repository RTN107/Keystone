package ratelimit

import (
	"testing"
	"time"
)

func TestWindowAllowsUpToMaxThenRefusesUntilTheOldestLeaves(t *testing.T) {
	w := New(3, time.Minute)
	clock := time.Unix(1000, 0)
	w.now = func() time.Time { return clock }

	for i := 0; i < 3; i++ {
		if ok, _ := w.Allow("a"); !ok {
			t.Fatalf("event %d should be allowed", i)
		}
		clock = clock.Add(10 * time.Second)
	}
	ok, retry := w.Allow("a")
	if ok {
		t.Fatal("the fourth event inside the window must be refused")
	}
	if retry != 30*time.Second {
		t.Errorf("retry after %v, want 30s (until the first event leaves)", retry)
	}
	if ok, _ := w.Allow("b"); !ok {
		t.Error("another key has its own allowance")
	}
	clock = clock.Add(31 * time.Second)
	if ok, _ := w.Allow("a"); !ok {
		t.Error("allowed again once the oldest event has left the window")
	}
}

func TestRefusedEventsAreNotCounted(t *testing.T) {
	w := New(1, time.Minute)
	clock := time.Unix(1000, 0)
	w.now = func() time.Time { return clock }
	w.Allow("a")
	for i := 0; i < 10; i++ {
		clock = clock.Add(time.Second)
		w.Allow("a") // refused each time
	}
	clock = clock.Add(50 * time.Second) // 60s after the one counted event
	if ok, _ := w.Allow("a"); !ok {
		t.Error("hammering while refused must not extend the wait")
	}
}
