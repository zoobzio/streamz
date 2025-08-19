package streamz

import (
	"sync"
	"testing"
	"time"
)

// TestAtomicTimeBasic tests basic store and load operations.
func TestAtomicTimeBasic(t *testing.T) {
	at := &AtomicTime{}

	// Test zero value.
	if !at.Load().IsZero() {
		t.Error("expected zero time for uninitialized AtomicTime")
	}

	// Test store and load.
	now := time.Now()
	at.Store(now)

	loaded := at.Load()
	if !loaded.Equal(now) {
		t.Errorf("expected %v, got %v", now, loaded)
	}
}

// TestAtomicTimeZeroValue tests handling of zero time.
func TestAtomicTimeZeroValue(t *testing.T) {
	at := &AtomicTime{}

	// Initial state should be zero.
	if !at.IsZero() {
		t.Error("expected IsZero() to return true for uninitialized AtomicTime")
	}

	// Store a non-zero time then check IsZero.
	now := time.Now()
	at.Store(now)
	if at.IsZero() {
		t.Error("expected IsZero() to return false after storing non-zero time")
	}
}

// TestAtomicTimeConcurrent tests concurrent access.
func TestAtomicTimeConcurrent(t *testing.T) {
	at := &AtomicTime{}

	// Use a fixed set of times to avoid precision issues.
	times := []time.Time{
		time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
		time.Date(2024, 1, 1, 11, 0, 0, 0, time.UTC),
		time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		time.Date(2024, 1, 1, 13, 0, 0, 0, time.UTC),
		time.Date(2024, 1, 1, 14, 0, 0, 0, time.UTC),
	}

	var wg sync.WaitGroup

	// Multiple writers.
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				at.Store(times[idx])
			}
		}(i)
	}

	// Multiple readers.
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				loaded := at.Load()
				// Verify it's one of our times or zero.
				if !loaded.IsZero() {
					found := false
					for _, t := range times {
						if loaded.Equal(t) {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("loaded unexpected time: %v", loaded)
					}
				}
			}
		}()
	}

	wg.Wait()

	// Final value should be one of our times.
	final := at.Load()
	found := false
	for _, t := range times {
		if final.Equal(t) {
			found = true
			break
		}
	}
	if !found && !final.IsZero() {
		t.Errorf("final time not one of expected values: %v", final)
	}
}

// TestAtomicTimeMultipleInstances tests multiple AtomicTime instances.
func TestAtomicTimeMultipleInstances(t *testing.T) {
	at1 := &AtomicTime{}
	at2 := &AtomicTime{}

	time1 := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	time2 := time.Date(2024, 1, 1, 11, 0, 0, 0, time.UTC)

	at1.Store(time1)
	at2.Store(time2)

	if at1.Load().Equal(at2.Load()) {
		t.Error("different AtomicTime instances should have independent values")
	}

	if !at1.Load().Equal(time1) {
		t.Errorf("at1: expected %v, got %v", time1, at1.Load())
	}

	if !at2.Load().Equal(time2) {
		t.Errorf("at2: expected %v, got %v", time2, at2.Load())
	}
}

// TestAtomicTimeNanosecondPrecision tests nanosecond precision.
func TestAtomicTimeNanosecondPrecision(t *testing.T) {
	at := &AtomicTime{}

	// Create time with nanosecond precision.
	baseTime := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	nanoTime := baseTime.Add(123456789 * time.Nanosecond)

	at.Store(nanoTime)
	loaded := at.Load()

	if !loaded.Equal(nanoTime) {
		t.Errorf("lost precision: expected %v, got %v", nanoTime, loaded)
	}

	// Verify nanoseconds are preserved.
	if loaded.Nanosecond() != nanoTime.Nanosecond() {
		t.Errorf("nanoseconds not preserved: expected %d, got %d",
			nanoTime.Nanosecond(), loaded.Nanosecond())
	}
}

// TestAtomicTimeSequentialUpdates tests sequential time updates.
func TestAtomicTimeSequentialUpdates(t *testing.T) {
	at := &AtomicTime{}

	// Store increasing times.
	for i := 0; i < 10; i++ {
		tm := time.Date(2024, 1, 1, i, 0, 0, 0, time.UTC)
		at.Store(tm)

		loaded := at.Load()
		if !loaded.Equal(tm) {
			t.Errorf("iteration %d: expected %v, got %v", i, tm, loaded)
		}
	}
}

// BenchmarkAtomicTimeStore benchmarks Store operation.
func BenchmarkAtomicTimeStore(b *testing.B) {
	at := &AtomicTime{}
	now := time.Now()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		at.Store(now)
	}
}

// BenchmarkAtomicTimeLoad benchmarks Load operation.
func BenchmarkAtomicTimeLoad(b *testing.B) {
	at := &AtomicTime{}
	at.Store(time.Now())

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = at.Load()
	}
}

// BenchmarkAtomicTimeConcurrentStoreLoad benchmarks concurrent operations.
func BenchmarkAtomicTimeConcurrentStoreLoad(b *testing.B) {
	at := &AtomicTime{}

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if i%2 == 0 {
				at.Store(time.Now())
			} else {
				_ = at.Load()
			}
			i++
		}
	})
}
