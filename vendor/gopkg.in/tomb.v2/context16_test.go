// +build !go1.7

package tomb_test

import (
	"testing"
	"time"

	"golang.org/x/net/context"

	"gopkg.in/tomb.v2"
)

func TestWithContext(t *testing.T) {
	parent1, cancel1 := context.WithCancel(context.Background())

	tb, child1 := tomb.WithContext(parent1)

	if !tb.Alive() {
		t.Fatalf("WithContext returned dead tomb")
	}
	if tb.Context(parent1) != child1 {
		t.Fatalf("Context returned different context for same parent")
	}
	if tb.Context(nil) != child1 {
		t.Fatalf("Context returned different context for nil parent")
	}
	select {
	case <-child1.Done():
		t.Fatalf("Tomb's child context was born dead")
	default:
	}

	parent2, cancel2 := context.WithCancel(context.WithValue(context.Background(), "parent", "parent2"))
	child2 := tb.Context(parent2)

	if tb.Context(parent2) != child2 {
		t.Fatalf("Context returned different context for same parent")
	}
	if child2.Value("parent") != "parent2" {
		t.Fatalf("Child context didn't inherit its parent's properties")
	}
	select {
	case <-child2.Done():
		t.Fatalf("Tomb's child context was born dead")
	default:
	}

	cancel2()

	select {
	case <-child2.Done():
	case <-time.After(5 * time.Second):
		t.Fatalf("Tomb's child context didn't die after parent was canceled")
	}
	if !tb.Alive() {
		t.Fatalf("Canceling unrelated parent context killed tomb")
	}

	parent3 := context.WithValue(context.Background(), "parent", "parent3")
	child3 := tb.Context(parent3)

	if child3.Value("parent") != "parent3" {
		t.Fatalf("Child context didn't inherit its parent's properties")
	}
	select {
	case <-child3.Done():
		t.Fatalf("Tomb's child context was born dead")
	default:
	}

	cancel1()

	select {
	case <-tb.Dying():
	case <-time.After(5 * time.Second):
		t.Fatalf("Canceling parent context did not kill tomb")
	}

	if tb.Err() != context.Canceled {
		t.Fatalf("tomb should be %v, got %v", context.Canceled, tb.Err())
	}

	if tb.Context(parent1) == child1 || tb.Context(parent3) == child3 {
		t.Fatalf("Tomb is dead and shouldn't be tracking children anymore")
	}
	select {
	case <-child3.Done():
	case <-time.After(5 * time.Second):
		t.Fatalf("Child context didn't die after tomb's death")
	}

	parent4 := context.WithValue(context.Background(), "parent", "parent4")
	child4 := tb.Context(parent4)

	select {
	case <-child4.Done():
	case <-time.After(5 * time.Second):
		t.Fatalf("Child context should be born canceled")
	}

	childnil := tb.Context(nil)
	select {
	case <-childnil.Done():
	default:
		t.Fatalf("Child context should be born canceled")
	}
}

func TestContextNoParent(t *testing.T) {
	var tb tomb.Tomb

	parent2, cancel2 := context.WithCancel(context.WithValue(context.Background(), "parent", "parent2"))
	child2 := tb.Context(parent2)

	if tb.Context(parent2) != child2 {
		t.Fatalf("Context returned different context for same parent")
	}
	if child2.Value("parent") != "parent2" {
		t.Fatalf("Child context didn't inherit its parent's properties")
	}
	select {
	case <-child2.Done():
		t.Fatalf("Tomb's child context was born dead")
	default:
	}

	cancel2()

	select {
	case <-child2.Done():
	default:
		t.Fatalf("Tomb's child context didn't die after parent was canceled")
	}
	if !tb.Alive() {
		t.Fatalf("Canceling unrelated parent context killed tomb")
	}

	parent3 := context.WithValue(context.Background(), "parent", "parent3")
	child3 := tb.Context(parent3)

	if child3.Value("parent") != "parent3" {
		t.Fatalf("Child context didn't inherit its parent's properties")
	}
	select {
	case <-child3.Done():
		t.Fatalf("Tomb's child context was born dead")
	default:
	}

	tb.Kill(nil)

	if tb.Context(parent3) == child3 {
		t.Fatalf("Tomb is dead and shouldn't be tracking children anymore")
	}
	select {
	case <-child3.Done():
	default:
		t.Fatalf("Child context didn't die after tomb's death")
	}

	parent4 := context.WithValue(context.Background(), "parent", "parent4")
	child4 := tb.Context(parent4)

	select {
	case <-child4.Done():
	default:
		t.Fatalf("Child context should be born canceled")
	}

	childnil := tb.Context(nil)
	select {
	case <-childnil.Done():
	default:
		t.Fatalf("Child context should be born canceled")
	}
}
