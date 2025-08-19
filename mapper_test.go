package streamz

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// TestMapperBasicFunctionality tests basic mapping operations.
func TestMapperBasicFunctionality(t *testing.T) {
	ctx := context.Background()

	// Create a mapper that doubles integers.
	doubler := NewMapper(func(n int) int {
		return n * 2
	}).WithName("doubler")

	// Test input.
	input := make(chan int, 5)
	for i := 1; i <= 5; i++ {
		input <- i
	}
	close(input)

	// Process.
	output := doubler.Process(ctx, input)

	// Verify.
	expected := []int{2, 4, 6, 8, 10}
	results := make([]int, 0, len(expected))
	for result := range output {
		results = append(results, result)
	}

	if len(results) != len(expected) {
		t.Errorf("expected %d results, got %d", len(expected), len(results))
	}

	for i, result := range results {
		if result != expected[i] {
			t.Errorf("position %d: expected %d, got %d", i, expected[i], result)
		}
	}

	if doubler.Name() != "doubler" {
		t.Errorf("expected name 'doubler', got %s", doubler.Name())
	}
}

// TestMapperTypeTransformation tests mapping between different types.
func TestMapperTypeTransformation(t *testing.T) {
	ctx := context.Background()

	// Create a mapper that converts integers to strings.
	stringifier := NewMapper(func(n int) string {
		return fmt.Sprintf("number-%d", n)
	})

	input := make(chan int, 3)
	input <- 1
	input <- 2
	input <- 3
	close(input)

	output := stringifier.Process(ctx, input)

	expected := []string{"number-1", "number-2", "number-3"}
	results := make([]string, 0, len(expected))
	for result := range output {
		results = append(results, result)
	}

	if len(results) != len(expected) {
		t.Errorf("expected %d results, got %d", len(expected), len(results))
	}

	for i, result := range results {
		if result != expected[i] {
			t.Errorf("position %d: expected %s, got %s", i, expected[i], result)
		}
	}
}

// TestMapperStructTransformation tests mapping with structs.
func TestMapperStructTransformation(t *testing.T) {
	ctx := context.Background()

	type User struct {
		ID   int
		Name string
	}

	type UserDTO struct {
		ID          int
		DisplayName string
	}

	// Create a mapper that transforms User to UserDTO.
	mapper := NewMapper(func(u User) UserDTO {
		return UserDTO{
			ID:          u.ID,
			DisplayName: strings.ToUpper(u.Name),
		}
	})

	input := make(chan User, 2)
	input <- User{ID: 1, Name: "alice"}
	input <- User{ID: 2, Name: "bob"}
	close(input)

	output := mapper.Process(ctx, input)

	results := make([]UserDTO, 0, 2)
	for dto := range output {
		results = append(results, dto)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	if results[0].DisplayName != "ALICE" {
		t.Errorf("expected display name 'ALICE', got %s", results[0].DisplayName)
	}

	if results[1].DisplayName != "BOB" {
		t.Errorf("expected display name 'BOB', got %s", results[1].DisplayName)
	}
}

// TestMapperEmptyInput tests mapper with empty input.
func TestMapperEmptyInput(t *testing.T) {
	ctx := context.Background()

	mapper := NewMapper(func(n int) int { return n })

	input := make(chan int)
	close(input)

	output := mapper.Process(ctx, input)

	count := 0
	for range output {
		count++
	}

	if count != 0 {
		t.Errorf("expected 0 results for empty input, got %d", count)
	}
}

// TestMapperContextCancellation tests graceful shutdown.
func TestMapperContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	mapper := NewMapper(func(n int) int {
		return n * 2
	})

	input := make(chan int)
	output := mapper.Process(ctx, input)

	// Send values continuously.
	go func() {
		for i := 0; ; i++ {
			select {
			case input <- i:
			case <-ctx.Done():
				close(input)
				return
			}
		}
	}()

	// Collect some results.
	resultCount := 0
	done := make(chan bool)
	go func() {
		for range output {
			resultCount++
			if resultCount >= 5 {
				done <- true
				return
			}
		}
		done <- true
	}()

	// Wait for some processing then cancel.
	<-done
	cancel()

	// Verify we got some results.
	if resultCount < 5 {
		t.Errorf("expected at least 5 results before cancellation, got %d", resultCount)
	}
}

// TestMapperChaining tests composing multiple mappers.
func TestMapperChaining(t *testing.T) {
	ctx := context.Background()

	// Create a chain: double -> add 10 -> to string.
	doubler := NewMapper(func(n int) int { return n * 2 })
	adder := NewMapper(func(n int) int { return n + 10 })
	stringifier := NewMapper(func(n int) string { return fmt.Sprintf("result: %d", n) })

	input := make(chan int, 3)
	input <- 1  // 1 * 2 = 2, 2 + 10 = 12
	input <- 5  // 5 * 2 = 10, 10 + 10 = 20
	input <- 10 // 10 * 2 = 20, 20 + 10 = 30
	close(input)

	// Chain the processors.
	doubled := doubler.Process(ctx, input)
	added := adder.Process(ctx, doubled)
	output := stringifier.Process(ctx, added)

	expected := []string{"result: 12", "result: 20", "result: 30"}
	results := make([]string, 0, len(expected))
	for result := range output {
		results = append(results, result)
	}

	if len(results) != len(expected) {
		t.Errorf("expected %d results, got %d", len(expected), len(results))
	}

	for i, result := range results {
		if result != expected[i] {
			t.Errorf("position %d: expected %s, got %s", i, expected[i], result)
		}
	}
}

// BenchmarkMapper benchmarks mapper performance.
func BenchmarkMapper(b *testing.B) {
	ctx := context.Background()

	mapper := NewMapper(func(n int) int {
		return n * 2
	})

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		input := make(chan int, 1)
		input <- i
		close(input)

		output := mapper.Process(ctx, input)
		<-output
	}
}

// BenchmarkMapperThroughput benchmarks high-throughput mapping.
func BenchmarkMapperThroughput(b *testing.B) {
	ctx := context.Background()

	mapper := NewMapper(func(n int) int {
		return n * 2
	})

	input := make(chan int, 1000)
	output := mapper.Process(ctx, input)

	// Consumer.
	done := make(chan bool)
	go func() {
		//nolint:revive // empty-block: necessary to drain channel
		for range output {
			// Consume.
		}
		done <- true
	}()

	b.ResetTimer()
	b.ReportAllocs()

	// Producer.
	for i := 0; i < b.N; i++ {
		input <- i
	}
	close(input)

	<-done
}

// Example demonstrates basic mapper usage.
func ExampleMapper() {
	ctx := context.Background()

	// Transform user data for display.
	type User struct {
		FirstName string
		LastName  string
		Age       int
	}

	type DisplayUser struct {
		FullName string
		Category string
	}

	mapper := NewMapper(func(u User) DisplayUser {
		// Combine names.
		fullName := fmt.Sprintf("%s %s", u.FirstName, u.LastName)

		// Categorize by age.
		category := "Adult"
		if u.Age < 18 {
			category = "Minor"
		} else if u.Age >= 65 {
			category = "Senior"
		}

		return DisplayUser{
			FullName: fullName,
			Category: category,
		}
	}).WithName("user-transformer")

	// Create test users.
	users := make(chan User, 3)
	users <- User{FirstName: "Alice", LastName: "Smith", Age: 12}
	users <- User{FirstName: "Bob", LastName: "Jones", Age: 35}
	users <- User{FirstName: "Carol", LastName: "White", Age: 67}
	close(users)

	// Transform users.
	displayUsers := mapper.Process(ctx, users)

	// Print results.
	for user := range displayUsers {
		fmt.Printf("%s (%s)\n", user.FullName, user.Category)
	}

	// Output:
	// Alice Smith (Minor)
	// Bob Jones (Adult)
	// Carol White (Senior)
}
