package streamz

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

// Test domain types for comprehensive testing.
type PaymentRoute string

const (
	RouteStandard  PaymentRoute = "standard"
	RouteHighValue PaymentRoute = "high_value"
	RouteFraud     PaymentRoute = "fraud"
)

type Payment struct {
	Amount    float64
	RiskScore float64
	Currency  string
}

type Order struct {
	ID       string
	Priority int
	Value    float64
}

func TestSwitch_BasicRouting(t *testing.T) {
	predicate := func(payment Payment) PaymentRoute {
		if payment.Amount > 10000 {
			return RouteHighValue
		}
		if payment.RiskScore > 0.8 {
			return RouteFraud
		}
		return RouteStandard
	}

	sw := NewSwitchSimple(predicate)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	input := make(chan Result[Payment], 10)
	_, errors := sw.Process(ctx, input)

	// Add routes
	standardCh := sw.AddRoute(RouteStandard)
	highValueCh := sw.AddRoute(RouteHighValue)
	fraudCh := sw.AddRoute(RouteFraud)

	// Send test payments
	input <- NewSuccess(Payment{Amount: 100, RiskScore: 0.1, Currency: "USD"})   // Standard
	input <- NewSuccess(Payment{Amount: 15000, RiskScore: 0.2, Currency: "USD"}) // High value
	input <- NewSuccess(Payment{Amount: 500, RiskScore: 0.9, Currency: "USD"})   // Fraud
	close(input)

	// Verify routing
	select {
	case result := <-standardCh:
		if !result.IsSuccess() {
			t.Fatal("Expected success result")
		}
		payment := result.Value()
		if payment.Amount != 100 {
			t.Errorf("Expected amount 100, got %f", payment.Amount)
		}
		// Check metadata
		if route, exists := result.GetMetadata("route"); !exists || route != RouteStandard {
			t.Errorf("Expected route metadata %v, got %v", RouteStandard, route)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Timeout waiting for standard route")
	}

	select {
	case result := <-highValueCh:
		if !result.IsSuccess() {
			t.Fatal("Expected success result")
		}
		payment := result.Value()
		if payment.Amount != 15000 {
			t.Errorf("Expected amount 15000, got %f", payment.Amount)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Timeout waiting for high value route")
	}

	select {
	case result := <-fraudCh:
		if !result.IsSuccess() {
			t.Fatal("Expected success result")
		}
		payment := result.Value()
		if payment.RiskScore != 0.9 {
			t.Errorf("Expected risk score 0.9, got %f", payment.RiskScore)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Timeout waiting for fraud route")
	}

	// Verify routes exist via HasRoute
	if !sw.HasRoute(RouteStandard) {
		t.Error("Standard route should exist")
	}
	if !sw.HasRoute(RouteHighValue) {
		t.Error("High value route should exist")
	}
	if !sw.HasRoute(RouteFraud) {
		t.Error("Fraud route should exist")
	}

	// Verify error channel access
	if sw.ErrorChannel() != errors {
		t.Error("ErrorChannel() did not return the correct error channel")
	}
}

func TestSwitch_ErrorPassthrough(t *testing.T) {
	predicate := func(_ Payment) PaymentRoute {
		return RouteStandard
	}

	sw := NewSwitchSimple(predicate)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	input := make(chan Result[Payment], 10)
	_, errorCh := sw.Process(ctx, input)

	// Send error result
	errorResult := NewError(Payment{Amount: 100}, errors.New("test error"), "upstream")
	input <- errorResult
	close(input)

	// Verify error goes to error channel
	select {
	case result := <-errorCh:
		if !result.IsError() {
			t.Fatal("Expected error result")
		}
		if result.Error().Err.Error() != "test error" {
			t.Errorf("Expected error message 'test error', got %s", result.Error().Err.Error())
		}
		// Check that processor metadata was added
		if processor, exists := result.GetMetadata(MetadataProcessor); !exists || processor != "switch" {
			t.Errorf("Expected processor metadata 'switch', got %v", processor)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Timeout waiting for error")
	}
}

func TestSwitch_PredicatePanic(t *testing.T) {
	predicate := func(payment Payment) PaymentRoute {
		if payment.Amount == 0 {
			panic("zero amount not allowed")
		}
		return RouteStandard
	}

	sw := NewSwitchSimple(predicate)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	input := make(chan Result[Payment], 10)
	_, errorCh := sw.Process(ctx, input)

	// Send payment that will cause panic
	input <- NewSuccess(Payment{Amount: 0, RiskScore: 0.1, Currency: "USD"})
	close(input)

	// Verify panic is recovered and sent to error channel
	select {
	case result := <-errorCh:
		if !result.IsError() {
			t.Fatal("Expected error result from panic")
		}
		errMsg := result.Error().Err.Error()
		if !contains(errMsg, "predicate panic") || !contains(errMsg, "zero amount not allowed") {
			t.Errorf("Expected panic error message, got: %s", errMsg)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Timeout waiting for panic error")
	}
}

func TestSwitch_UnknownRoute(t *testing.T) {
	// Test with default route
	defaultKey := RouteStandard
	config := SwitchConfig[PaymentRoute]{
		BufferSize: 0,
		DefaultKey: &defaultKey,
	}

	predicate := func(_ Payment) PaymentRoute {
		// Return a route that doesn't exist
		return PaymentRoute("unknown")
	}

	sw := NewSwitch(predicate, config)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	input := make(chan Result[Payment], 10)
	_, _ = sw.Process(ctx, input)

	// Add only the default route
	defaultCh := sw.AddRoute(RouteStandard)

	// Send payment that will route to unknown key
	input <- NewSuccess(Payment{Amount: 100, RiskScore: 0.1, Currency: "USD"})
	close(input)

	// Verify it goes to default route
	select {
	case result := <-defaultCh:
		if !result.IsSuccess() {
			t.Fatal("Expected success result in default route")
		}
		payment := result.Value()
		if payment.Amount != 100 {
			t.Errorf("Expected amount 100, got %f", payment.Amount)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Timeout waiting for default route")
	}

	// Verify default route exists
	if !sw.HasRoute(RouteStandard) {
		t.Error("Default route should exist")
	}
}

func TestSwitch_UnknownRouteDrop(t *testing.T) {
	// Test without default route (should drop)
	predicate := func(_ Payment) PaymentRoute {
		// Return a route that doesn't exist
		return PaymentRoute("unknown")
	}

	sw := NewSwitchSimple(predicate)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	input := make(chan Result[Payment], 10)
	_, errorCh := sw.Process(ctx, input)

	// Don't add any routes

	// Send payment that will route to unknown key
	input <- NewSuccess(Payment{Amount: 100, RiskScore: 0.1, Currency: "USD"})

	// Verify nothing is sent anywhere (message is dropped)
	select {
	case <-errorCh:
		t.Fatal("Unexpected message in error channel")
	case <-time.After(50 * time.Millisecond):
		// Expected - message should be dropped
	}

	close(input)
}

func TestSwitch_ConcurrentAccess(t *testing.T) {
	predicate := func(order Order) int {
		return order.Priority
	}

	sw := NewSwitchSimple(predicate)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	input := make(chan Result[Order], 100)
	_, _ = sw.Process(ctx, input)

	// Add routes concurrently
	var wg sync.WaitGroup
	numGoroutines := 10

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(priority int) {
			defer wg.Done()
			sw.AddRoute(priority)
		}(i)
	}

	// Send orders concurrently
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(priority int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				order := Order{
					ID:       fmt.Sprintf("order-%d-%d", priority, j),
					Priority: priority,
					Value:    float64(j * 100),
				}
				input <- NewSuccess(order)
			}
		}(i)
	}

	wg.Wait()
	close(input)

	// Verify all routes exist
	keys := sw.RouteKeys()
	if len(keys) != numGoroutines {
		t.Errorf("Expected %d routes, got %d", numGoroutines, len(keys))
	}

	// Verify all routes exist
	for i := 0; i < numGoroutines; i++ {
		if !sw.HasRoute(i) {
			t.Errorf("Route %d not found", i)
		}
	}
}

func TestSwitch_ContextCancellation(t *testing.T) {
	predicate := func(_ Payment) PaymentRoute {
		return RouteStandard
	}

	sw := NewSwitchSimple(predicate)

	ctx, cancel := context.WithCancel(context.Background())

	input := make(chan Result[Payment], 10)
	_, _ = sw.Process(ctx, input)

	// Add route
	standardCh := sw.AddRoute(RouteStandard)

	// Send a payment
	input <- NewSuccess(Payment{Amount: 100, RiskScore: 0.1, Currency: "USD"})

	// Cancel context
	cancel()

	// Verify channel gets closed
	select {
	case _, ok := <-standardCh:
		if ok {
			// Should receive the payment first
			select {
			case _, ok := <-standardCh:
				if ok {
					t.Fatal("Channel should be closed after context cancellation")
				}
			case <-time.After(100 * time.Millisecond):
				t.Fatal("Channel should be closed quickly after context cancellation")
			}
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Should receive at least one message or closed channel")
	}
}

func TestSwitch_MetadataPreservation(t *testing.T) {
	predicate := func(_ Payment) PaymentRoute {
		return RouteStandard
	}

	sw := NewSwitchSimple(predicate)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	input := make(chan Result[Payment], 10)
	_, _ = sw.Process(ctx, input)

	// Add route
	standardCh := sw.AddRoute(RouteStandard)

	// Send payment with existing metadata
	originalResult := NewSuccess(Payment{Amount: 100, RiskScore: 0.1, Currency: "USD"}).
		WithMetadata("custom", "value").
		WithMetadata("source", "test")

	input <- originalResult
	close(input)

	// Verify metadata is preserved and enhanced
	select {
	case result := <-standardCh:
		// Check original metadata is preserved
		if custom, exists := result.GetMetadata("custom"); !exists || custom != "value" {
			t.Error("Original metadata not preserved")
		}
		if source, exists := result.GetMetadata("source"); !exists || source != "test" {
			t.Error("Original metadata not preserved")
		}

		// Check switch metadata is added
		if route, exists := result.GetMetadata("route"); !exists || route != RouteStandard {
			t.Error("Route metadata not added")
		}
		if processor, exists := result.GetMetadata(MetadataProcessor); !exists || processor != "switch" {
			t.Error("Processor metadata not added")
		}
		if _, exists := result.GetMetadata(MetadataTimestamp); !exists {
			t.Error("Timestamp metadata not added")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Timeout waiting for result")
	}
}

func TestSwitch_ChannelBuffering(t *testing.T) {
	predicate := func(_ Payment) PaymentRoute {
		return RouteStandard
	}

	// Test buffered channels
	config := SwitchConfig[PaymentRoute]{
		BufferSize: 5,
		DefaultKey: nil,
	}

	sw := NewSwitch(predicate, config)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	input := make(chan Result[Payment], 10)
	_, _ = sw.Process(ctx, input)

	// Add route
	standardCh := sw.AddRoute(RouteStandard)

	// Send multiple payments without reading
	for i := 0; i < 5; i++ {
		input <- NewSuccess(Payment{Amount: float64(i * 100), RiskScore: 0.1, Currency: "USD"})
	}

	// All should be buffered without blocking
	close(input)

	// Read all buffered payments
	for i := 0; i < 5; i++ {
		select {
		case result := <-standardCh:
			if !result.IsSuccess() {
				t.Fatal("Expected success result")
			}
			payment := result.Value()
			expectedAmount := float64(i * 100)
			if payment.Amount != expectedAmount {
				t.Errorf("Expected amount %f, got %f", expectedAmount, payment.Amount)
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("Timeout waiting for payment %d", i)
		}
	}
}

func TestSwitch_RouteManagement(t *testing.T) {
	predicate := func(order Order) int {
		return order.Priority
	}

	sw := NewSwitchSimple(predicate)

	// Test AddRoute
	ch1 := sw.AddRoute(1)
	if ch1 == nil {
		t.Fatal("AddRoute returned nil channel")
	}

	// Test HasRoute
	if !sw.HasRoute(1) {
		t.Error("HasRoute should return true for added route")
	}
	if sw.HasRoute(2) {
		t.Error("HasRoute should return false for non-existent route")
	}

	// Test RouteKeys
	keys := sw.RouteKeys()
	if len(keys) != 1 || keys[0] != 1 {
		t.Errorf("Expected keys [1], got %v", keys)
	}

	// Add another route
	_ = sw.AddRoute(2)
	keys = sw.RouteKeys()
	if len(keys) != 2 {
		t.Errorf("Expected 2 keys, got %d", len(keys))
	}

	// Test that adding same route returns same channel
	ch1Again := sw.AddRoute(1)
	if ch1 != ch1Again {
		t.Error("AddRoute should return same channel for existing route")
	}

	// Test RemoveRoute
	if !sw.RemoveRoute(1) {
		t.Error("RemoveRoute should return true for existing route")
	}
	if sw.HasRoute(1) {
		t.Error("Route should not exist after removal")
	}
	if sw.RemoveRoute(1) {
		t.Error("RemoveRoute should return false for non-existent route")
	}

	// Verify channel is closed after removal
	select {
	case _, ok := <-ch1:
		if ok {
			t.Error("Channel should be closed after route removal")
		}
	case <-time.After(10 * time.Millisecond):
		t.Error("Channel should be closed immediately after route removal")
	}

	// Verify remaining route still exists
	if !sw.HasRoute(2) {
		t.Error("Remaining route should still exist")
	}

	keys = sw.RouteKeys()
	if len(keys) != 1 || keys[0] != 2 {
		t.Errorf("Expected keys [2], got %v", keys)
	}
}

func TestSwitch_BackpressureIsolation(t *testing.T) {
	predicate := func(order Order) int {
		return order.Priority
	}

	sw := NewSwitchSimple(predicate)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	input := make(chan Result[Order], 10)
	_, _ = sw.Process(ctx, input)

	// Add two routes
	ch1 := sw.AddRoute(1)
	ch2 := sw.AddRoute(2)

	// Send two messages to different routes
	input <- NewSuccess(Order{ID: "1", Priority: 1, Value: 100})
	input <- NewSuccess(Order{ID: "2", Priority: 2, Value: 200})

	// Both channels should receive their messages independently
	// Channel 1 should not block channel 2
	var wg sync.WaitGroup
	var order1, order2 Order
	var success1, success2 bool

	wg.Add(2)

	// Read from channel 1
	go func() {
		defer wg.Done()
		select {
		case result := <-ch1:
			if result.IsSuccess() {
				order1 = result.Value()
				success1 = true
			}
		case <-time.After(100 * time.Millisecond):
			// timeout
		}
	}()

	// Read from channel 2
	go func() {
		defer wg.Done()
		select {
		case result := <-ch2:
			if result.IsSuccess() {
				order2 = result.Value()
				success2 = true
			}
		case <-time.After(100 * time.Millisecond):
			// timeout
		}
	}()

	wg.Wait()

	// Verify both channels received their messages
	if !success1 {
		t.Fatal("Channel 1 should receive its message")
	}
	if !success2 {
		t.Fatal("Channel 2 should receive its message")
	}
	if order1.ID != "1" {
		t.Errorf("Expected order ID '1', got '%s'", order1.ID)
	}
	if order2.ID != "2" {
		t.Errorf("Expected order ID '2', got '%s'", order2.ID)
	}

	close(input)
}

func TestSwitch_ChannelCleanup(t *testing.T) {
	predicate := func(_ Payment) PaymentRoute {
		return RouteStandard
	}

	sw := NewSwitchSimple(predicate)

	ctx, cancel := context.WithCancel(context.Background())

	input := make(chan Result[Payment], 10)
	_, errorCh := sw.Process(ctx, input)

	// Add routes
	standardCh := sw.AddRoute(RouteStandard)
	highValueCh := sw.AddRoute(RouteHighValue)

	// Close input to trigger cleanup
	close(input)

	// Verify all channels are closed
	select {
	case _, ok := <-standardCh:
		if ok {
			t.Error("Standard channel should be closed")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Standard channel should be closed")
	}

	select {
	case _, ok := <-highValueCh:
		if ok {
			t.Error("High value channel should be closed")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("High value channel should be closed")
	}

	select {
	case _, ok := <-errorCh:
		if ok {
			t.Error("Error channel should be closed")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Error channel should be closed")
	}

	cancel() // cleanup
}

func TestSwitch_ErrorChannelInit(t *testing.T) {
	predicate := func(_ Payment) PaymentRoute {
		return RouteStandard
	}

	// Test unbuffered error channel
	sw := NewSwitchSimple(predicate)
	if sw.errorChan == nil {
		t.Fatal("Error channel should be initialized")
	}

	// Test buffered error channel
	config := SwitchConfig[PaymentRoute]{
		BufferSize: 5,
		DefaultKey: nil,
	}
	swBuffered := NewSwitch(predicate, config)
	if swBuffered.errorChan == nil {
		t.Fatal("Buffered error channel should be initialized")
	}

	// Verify error channel is accessible
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	input := make(chan Result[Payment], 1)
	_, errorCh := sw.Process(ctx, input)

	if errorCh != sw.ErrorChannel() {
		t.Error("ErrorChannel() should return the same channel as Process")
	}

	close(input)
}

func TestSwitch_MetadataConstants(t *testing.T) {
	predicate := func(_ Payment) PaymentRoute {
		return RouteStandard
	}

	sw := NewSwitchSimple(predicate)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	input := make(chan Result[Payment], 10)
	_, _ = sw.Process(ctx, input)

	// Add route
	standardCh := sw.AddRoute(RouteStandard)

	// Send payment
	input <- NewSuccess(Payment{Amount: 100, RiskScore: 0.1, Currency: "USD"})
	close(input)

	// Verify metadata constants are used correctly
	select {
	case result := <-standardCh:
		// Check processor metadata uses constant
		processor, exists := result.GetMetadata(MetadataProcessor)
		if !exists {
			t.Error("MetadataProcessor constant not used")
		}
		if processor != "switch" {
			t.Errorf("Expected processor 'switch', got %v", processor)
		}

		// Check timestamp metadata uses constant
		timestamp, exists := result.GetMetadata(MetadataTimestamp)
		if !exists {
			t.Error("MetadataTimestamp constant not used")
		}
		if _, ok := timestamp.(time.Time); !ok {
			t.Errorf("Expected timestamp to be time.Time, got %T", timestamp)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Timeout waiting for result")
	}
}

func TestSwitch_PaymentRouting(t *testing.T) {
	predicate := func(payment Payment) PaymentRoute {
		if payment.Amount > 10000 {
			return RouteHighValue
		}
		if payment.RiskScore > 0.8 {
			return RouteFraud
		}
		return RouteStandard
	}

	sw := NewSwitchSimple(predicate)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	input := make(chan Result[Payment], 10)
	_, errorCh := sw.Process(ctx, input)

	// Create test scenarios
	testCases := []struct {
		payment       Payment
		expectedRoute PaymentRoute
	}{
		{Payment{Amount: 100, RiskScore: 0.1, Currency: "USD"}, RouteStandard},
		{Payment{Amount: 15000, RiskScore: 0.2, Currency: "EUR"}, RouteHighValue},
		{Payment{Amount: 500, RiskScore: 0.9, Currency: "GBP"}, RouteFraud},
		{Payment{Amount: 25000, RiskScore: 0.95, Currency: "CAD"}, RouteHighValue}, // Amount takes precedence
	}

	// Add all routes
	standardCh := sw.AddRoute(RouteStandard)
	highValueCh := sw.AddRoute(RouteHighValue)
	fraudCh := sw.AddRoute(RouteFraud)

	// Send test payments
	for _, tc := range testCases {
		input <- NewSuccess(tc.payment)
	}
	close(input)

	// Verify routing for each test case
	receivedStandard := 0
	receivedHighValue := 0
	receivedFraud := 0

	// Collect all results
	collectionErrors := make(chan error, len(testCases))
	done := make(chan bool)
	go func() {
		defer close(done)
		for i := 0; i < len(testCases); i++ {
			select {
			case result := <-standardCh:
				receivedStandard++
				payment := result.Value()
				if payment.Amount != 100 {
					collectionErrors <- fmt.Errorf("Standard route: expected amount 100, got %f", payment.Amount)
				}
			case result := <-highValueCh:
				receivedHighValue++
				payment := result.Value()
				if payment.Amount <= 10000 {
					collectionErrors <- fmt.Errorf("High value route: expected amount > 10000, got %f", payment.Amount)
				}
			case result := <-fraudCh:
				receivedFraud++
				payment := result.Value()
				if payment.RiskScore <= 0.8 {
					collectionErrors <- fmt.Errorf("fraud route: expected risk score > 0.8, got %f", payment.RiskScore)
				}
			case <-time.After(100 * time.Millisecond):
				collectionErrors <- fmt.Errorf("timeout waiting for result %d", i)
				return
			}
		}
	}()

	select {
	case <-done:
		// Check for any errors from goroutine
		select {
		case err := <-collectionErrors:
			t.Fatal(err)
		default:
		}
		// Verify counts
		if receivedStandard != 1 {
			t.Errorf("Expected 1 standard payment, got %d", receivedStandard)
		}
		if receivedHighValue != 2 {
			t.Errorf("Expected 2 high value payments, got %d", receivedHighValue)
		}
		if receivedFraud != 1 {
			t.Errorf("Expected 1 fraud payment, got %d", receivedFraud)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Timeout waiting for all results")
	}

	// Verify no errors
	select {
	case <-errorCh:
		t.Error("Unexpected error received")
	default:
		// Expected - no errors
	}

	// Verify all routes exist
	if !sw.HasRoute(RouteStandard) || !sw.HasRoute(RouteHighValue) || !sw.HasRoute(RouteFraud) {
		t.Error("Not all expected routes exist")
	}
}

// Helper function for string contains check.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || substr == "" ||
		(len(s) > len(substr) &&
			(s[:len(substr)] == substr ||
				s[len(s)-len(substr):] == substr ||
				findSubstring(s, substr))))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
