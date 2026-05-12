package usage

import "testing"

// BenchmarkAdd measures the cost of summing a long stream of Usage values —
// the dominant hot path when aggregating cross-session token totals. Pure
// stdlib types, so this is also a baseline number we can use to detect a
// regression if Usage gains a non-trivial field later.
func BenchmarkAdd(b *testing.B) {
	a := Usage{Input: 1, Output: 2, CacheRead: 3, CacheCreation: 4}
	c := Usage{Input: 10, Output: 20, CacheRead: 30, CacheCreation: 40}
	for b.Loop() {
		a = a.Add(c)
	}
	// Prevent the compiler from eliding the result.
	if a.Input < 0 {
		b.Fatal("unreachable")
	}
}

// BenchmarkAdd_TightLoop simulates the realistic 5-hour aggregation: thousands
// of per-event Usage instances reduced into a running total.
func BenchmarkAdd_TightLoop(b *testing.B) {
	stream := make([]Usage, 1024)
	for i := range stream {
		stream[i] = Usage{Input: int64(i), Output: int64(i * 2)}
	}
	for b.Loop() {
		var total Usage
		for _, u := range stream {
			total = total.Add(u)
		}
		// Prevent the compiler from eliding the loop.
		if total.Input < 0 {
			b.Fatal("unreachable")
		}
	}
}
