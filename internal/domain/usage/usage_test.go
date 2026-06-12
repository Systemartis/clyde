package usage_test

import (
	"testing"

	"github.com/Systemartis/clyde/internal/domain/usage"
)

// TestUsageZeroIsIdentity tests the left-identity law: Zero.Add(a) == a
func TestUsageZeroIsIdentity(t *testing.T) {
	tests := []struct {
		name string
		a    usage.Usage
	}{
		{
			name: "zero_plus_zero",
			a:    usage.Zero(),
		},
		{
			name: "non_zero_input",
			a:    usage.Usage{Input: 3, Output: 4, CacheRead: 1, CacheCreation: 2},
		},
		{
			name: "only_output",
			a:    usage.Usage{Input: 0, Output: 10, CacheRead: 0, CacheCreation: 0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := usage.Zero().Add(tt.a)
			if result != tt.a {
				t.Errorf("Zero().Add(%v) = %v, want %v", tt.a, result, tt.a)
			}
		})
	}
}

// TestUsageZeroIsIdentityRight tests the right-identity law: a.Add(Zero) == a
func TestUsageZeroIsIdentityRight(t *testing.T) {
	tests := []struct {
		name string
		a    usage.Usage
	}{
		{
			name: "zero_plus_zero",
			a:    usage.Zero(),
		},
		{
			name: "non_zero_values",
			a:    usage.Usage{Input: 7, Output: 3, CacheRead: 5, CacheCreation: 1},
		},
		{
			name: "large_counters",
			a:    usage.Usage{Input: 100000, Output: 200000, CacheRead: 50000, CacheCreation: 75000},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.a.Add(usage.Zero())
			if result != tt.a {
				t.Errorf("a.Add(Zero()) = %v, want %v", result, tt.a)
			}
		})
	}
}

// TestUsageCommutativity tests that a.Add(b) == b.Add(a) for any a, b.
func TestUsageCommutativity(t *testing.T) {
	tests := []struct {
		name string
		a    usage.Usage
		b    usage.Usage
	}{
		{
			name: "both_zero",
			a:    usage.Zero(),
			b:    usage.Zero(),
		},
		{
			name: "a_non_zero_b_zero",
			a:    usage.Usage{Input: 5, Output: 3, CacheRead: 2, CacheCreation: 1},
			b:    usage.Zero(),
		},
		{
			name: "both_non_zero",
			a:    usage.Usage{Input: 3, Output: 4, CacheRead: 1, CacheCreation: 2},
			b:    usage.Usage{Input: 1, Output: 2, CacheRead: 3, CacheCreation: 4},
		},
		{
			name: "large_values",
			a:    usage.Usage{Input: 100, Output: 200, CacheRead: 300, CacheCreation: 400},
			b:    usage.Usage{Input: 400, Output: 300, CacheRead: 200, CacheCreation: 100},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ab := tt.a.Add(tt.b)
			ba := tt.b.Add(tt.a)
			if ab != ba {
				t.Errorf("commutativity violated: a.Add(b) = %v, b.Add(a) = %v", ab, ba)
			}
		})
	}
}

// TestUsageAssociativity tests that (a.Add(b)).Add(c) == a.Add(b.Add(c)).
func TestUsageAssociativity(t *testing.T) {
	tests := []struct {
		name string
		a    usage.Usage
		b    usage.Usage
		c    usage.Usage
	}{
		{
			name: "all_zero",
			a:    usage.Zero(),
			b:    usage.Zero(),
			c:    usage.Zero(),
		},
		{
			name: "sequential_accumulation",
			a:    usage.Usage{Input: 1, Output: 1, CacheRead: 1, CacheCreation: 1},
			b:    usage.Usage{Input: 2, Output: 2, CacheRead: 2, CacheCreation: 2},
			c:    usage.Usage{Input: 3, Output: 3, CacheRead: 3, CacheCreation: 3},
		},
		{
			name: "mixed_values",
			a:    usage.Usage{Input: 10, Output: 0, CacheRead: 5, CacheCreation: 0},
			b:    usage.Usage{Input: 0, Output: 20, CacheRead: 0, CacheCreation: 10},
			c:    usage.Usage{Input: 7, Output: 7, CacheRead: 7, CacheCreation: 7},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			left := tt.a.Add(tt.b).Add(tt.c)
			right := tt.a.Add(tt.b.Add(tt.c))
			if left != right {
				t.Errorf("associativity violated: (a+b)+c = %v, a+(b+c) = %v", left, right)
			}
		})
	}
}

// TestUsageCounterAccumulation tests exact counter arithmetic:
// a{3,4,1,2} + b{1,2,3,4} = {4,6,4,6}.
func TestUsageCounterAccumulation(t *testing.T) {
	tests := []struct {
		name string
		a    usage.Usage
		b    usage.Usage
		want usage.Usage
	}{
		{
			name: "spec_example",
			a:    usage.Usage{Input: 3, Output: 4, CacheRead: 1, CacheCreation: 2},
			b:    usage.Usage{Input: 1, Output: 2, CacheRead: 3, CacheCreation: 4},
			want: usage.Usage{Input: 4, Output: 6, CacheRead: 4, CacheCreation: 6},
		},
		{
			name: "zero_base",
			a:    usage.Zero(),
			b:    usage.Usage{Input: 5, Output: 10, CacheRead: 15, CacheCreation: 20},
			want: usage.Usage{Input: 5, Output: 10, CacheRead: 15, CacheCreation: 20},
		},
		{
			name: "all_fields",
			a:    usage.Usage{Input: 100, Output: 200, CacheRead: 300, CacheCreation: 400},
			b:    usage.Usage{Input: 1, Output: 2, CacheRead: 3, CacheCreation: 4},
			want: usage.Usage{Input: 101, Output: 202, CacheRead: 303, CacheCreation: 404},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.a.Add(tt.b)
			if result != tt.want {
				t.Errorf("a.Add(b) = %v, want %v", result, tt.want)
			}
		})
	}
}

// TestUsageAddIsNonMutating asserts that Add returns a new value and does not
// modify the receiver or the argument.
func TestUsageAddIsNonMutating(t *testing.T) {
	original := usage.Usage{Input: 1, Output: 2, CacheRead: 3, CacheCreation: 4}
	other := usage.Usage{Input: 10, Output: 20, CacheRead: 30, CacheCreation: 40}

	// Capture snapshots before Add.
	snapshotOriginal := original
	snapshotOther := other

	result := original.Add(other)

	// Receiver must be unchanged.
	if original != snapshotOriginal {
		t.Errorf("original was mutated: got %v, want %v", original, snapshotOriginal)
	}
	// Argument must be unchanged.
	if other != snapshotOther {
		t.Errorf("other was mutated: got %v, want %v", other, snapshotOther)
	}
	// Result must be the element-wise sum.
	want := usage.Usage{Input: 11, Output: 22, CacheRead: 33, CacheCreation: 44}
	if result != want {
		t.Errorf("result = %v, want %v", result, want)
	}
}
