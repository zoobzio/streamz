package streamz

import (
	"context"
	"fmt"
	"testing"
)

func TestFilter(t *testing.T) {
	ctx := context.Background()
	in := make(chan int)

	isEven := NewFilter(func(n int) bool { return n%2 == 0 }).WithName("even")
	out := isEven.Process(ctx, in)

	go func() {
		for i := 1; i <= 6; i++ {
			in <- i
		}
		close(in)
	}()

	results := []int{}
	for val := range out {
		results = append(results, val)
	}

	expected := []int{2, 4, 6}
	for i, v := range results {
		if v != expected[i] {
			t.Errorf("expected %d, got %d", expected[i], v)
		}
	}
}

// Example demonstrates filtering stream items based on a condition.
func ExampleFilter() {
	ctx := context.Background()

	// Create a filter that only allows valid email addresses.
	type User struct {
		Name  string
		Email string
	}

	isValidEmail := func(u User) bool {
		// Simple validation - just check for @ symbol.
		for _, ch := range u.Email {
			if ch == '@' {
				return true
			}
		}
		return false
	}

	filter := NewFilter(isValidEmail).WithName("valid-email-filter")

	// Create test users.
	users := make(chan User, 4)
	users <- User{Name: "Alice", Email: "alice@example.com"}
	users <- User{Name: "Bob", Email: "invalid-email"}
	users <- User{Name: "Carol", Email: "carol@company.org"}
	users <- User{Name: "Dave", Email: "no-at-symbol"}
	close(users)

	// Process only users with valid emails.
	validUsers := filter.Process(ctx, users)

	// Print valid users.
	for user := range validUsers {
		fmt.Printf("%s: %s\n", user.Name, user.Email)
	}

	// Output:
	// Alice: alice@example.com
	// Carol: carol@company.org
}
