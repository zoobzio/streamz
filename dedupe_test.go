package streamz

import (
	"context"
	"testing"
	"time"
)

func TestDedupe(t *testing.T) {
	ctx := context.Background()
	in := make(chan int)

	dedupe := NewDedupe(func(i int) int { return i }, 100*time.Millisecond)
	out := dedupe.Process(ctx, in)

	go func() {
		for i := 0; i < 3; i++ {
			in <- 1
			in <- 2
			in <- 1
		}
		close(in)
	}()

	results := []int{}
	for result := range out {
		results = append(results, result)
	}

	if len(results) != 2 {
		t.Errorf("expected 2 unique items, got %d: %v", len(results), results)
	}

	if results[0] != 1 || results[1] != 2 {
		t.Errorf("expected [1, 2], got %v", results)
	}
}

func TestDedupeTTL(t *testing.T) {
	ctx := context.Background()
	in := make(chan int)

	dedupe := NewDedupe(func(i int) int { return i }, 50*time.Millisecond)
	out := dedupe.Process(ctx, in)

	go func() {
		in <- 1
		time.Sleep(60 * time.Millisecond)
		in <- 1
		in <- 2
		close(in)
	}()

	results := []int{}
	for result := range out {
		results = append(results, result)
	}

	if len(results) != 3 {
		t.Errorf("expected 3 items (1 appeared twice after TTL), got %d: %v", len(results), results)
	}
}

func TestDedupeCustomKey(t *testing.T) {
	ctx := context.Background()
	in := make(chan string)

	dedupe := NewDedupe(func(s string) int { return len(s) }, 100*time.Millisecond)
	out := dedupe.Process(ctx, in)

	go func() {
		in <- "a"
		in <- "bb"
		in <- "c"
		in <- "dd"
		close(in)
	}()

	results := []string{}
	for result := range out {
		results = append(results, result)
	}

	if len(results) != 2 {
		t.Errorf("expected 2 items (different lengths), got %d: %v", len(results), results)
	}

	if results[0] != "a" || results[1] != "bb" {
		t.Errorf("expected first item of each length, got %v", results)
	}
}

func TestDedupeCleanup(t *testing.T) {
	ctx := context.Background()
	in := make(chan int)

	dedupe := NewDedupe(func(i int) int { return i }, 40*time.Millisecond)
	out := dedupe.Process(ctx, in)

	go func() {
		for i := 0; i < 100; i++ {
			in <- i
			time.Sleep(time.Millisecond)
		}
		close(in)
	}()

	go func() {
		//nolint:revive // empty-block: necessary to drain output for cleanup test
		for range out {
		}
	}()

	time.Sleep(150 * time.Millisecond)

	dedupe.mu.Lock()
	seenCount := len(dedupe.seen)
	dedupe.mu.Unlock()

	if seenCount > 50 {
		t.Errorf("cleanup not working, too many items in memory: %d", seenCount)
	}
}
