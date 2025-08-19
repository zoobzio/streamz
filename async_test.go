package streamz

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestAsyncMapper(t *testing.T) {
	ctx := context.Background()
	in := make(chan int)

	mapper := NewAsyncMapper(func(_ context.Context, i int) (string, error) {
		time.Sleep(time.Duration(10-i) * time.Millisecond)
		return fmt.Sprintf("item-%d", i), nil
	}).WithWorkers(3)

	out := mapper.Process(ctx, in)

	go func() {
		for i := 0; i < 10; i++ {
			in <- i
		}
		close(in)
	}()

	results := []string{}
	for result := range out {
		results = append(results, result)
	}

	if len(results) != 10 {
		t.Errorf("expected 10 results, got %d", len(results))
	}

	for i, result := range results {
		expected := fmt.Sprintf("item-%d", i)
		if result != expected {
			t.Errorf("expected %s at position %d, got %s", expected, i, result)
		}
	}
}

func TestAsyncMapperWithErrors(t *testing.T) {
	ctx := context.Background()
	in := make(chan int)

	mapper := NewAsyncMapper(func(_ context.Context, i int) (int, error) {
		if i%2 == 0 {
			return i * 2, nil
		}
		return 0, fmt.Errorf("odd number")
	}).WithWorkers(2)

	out := mapper.Process(ctx, in)

	go func() {
		for i := 0; i < 10; i++ {
			in <- i
		}
		close(in)
	}()

	results := []int{}
	for result := range out {
		results = append(results, result)
	}

	expected := []int{0, 4, 8, 12, 16}
	if len(results) != len(expected) {
		t.Errorf("expected %d results (even numbers only), got %d: %v", len(expected), len(results), results)
	}

	for i, result := range results {
		if i < len(expected) && result != expected[i] {
			t.Errorf("expected %d at position %d, got %d", expected[i], i, result)
		}
	}
}

func TestAsyncMapperConcurrency(t *testing.T) {
	ctx := context.Background()
	in := make(chan int)

	processing := make(chan int, 5)
	mapper := NewAsyncMapper(func(_ context.Context, i int) (int, error) {
		processing <- i
		defer func() { <-processing }()
		time.Sleep(50 * time.Millisecond)
		return i, nil
	}).WithWorkers(5)

	out := mapper.Process(ctx, in)

	go func() {
		for i := 0; i < 10; i++ {
			in <- i
		}
		close(in)
	}()

	go func() {
		//nolint:revive // empty-block: necessary to drain output channel for concurrency test
		for range out {
		}
	}()

	time.Sleep(30 * time.Millisecond)

	concurrent := len(processing)
	if concurrent != 5 {
		t.Errorf("expected 5 concurrent operations, got %d", concurrent)
	}
}

// Example demonstrates concurrent async processing of HTTP requests.
func ExampleAsyncMapper() {
	ctx := context.Background()

	// Simulate enriching user data with external API calls.
	type User struct {
		ID   int
		Name string
	}

	type EnrichedUser struct {
		ID      int
		Name    string
		Profile string
	}

	// Create async mapper for concurrent API calls.
	enricher := NewAsyncMapper(func(_ context.Context, u User) (EnrichedUser, error) {
		// Simulate API call to fetch user profile.
		// In production, this would be an actual HTTP request.
		profile := fmt.Sprintf("Profile data for %s", u.Name)

		return EnrichedUser{
			ID:      u.ID,
			Name:    u.Name,
			Profile: profile,
		}, nil
	}).WithWorkers(3)

	// Create users to enrich.
	users := make(chan User, 3)
	users <- User{ID: 1, Name: "Alice"}
	users <- User{ID: 2, Name: "Bob"}
	users <- User{ID: 3, Name: "Carol"}
	close(users)

	// Process concurrently.
	enriched := enricher.Process(ctx, users)

	// Display enriched users.
	// Note: Order may vary due to concurrent processing.
	results := make([]string, 0, 3)
	for user := range enriched {
		results = append(results, fmt.Sprintf("User %d: %s - %s", user.ID, user.Name, user.Profile))
	}

	// Sort for consistent output in example.
	fmt.Println("Enriched users:")
	for _, result := range results {
		fmt.Println(result)
	}

	// Output:
	// Enriched users:
	// User 1: Alice - Profile data for Alice
	// User 2: Bob - Profile data for Bob
	// User 3: Carol - Profile data for Carol
}
