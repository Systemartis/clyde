// Package usage defines the Usage value object — an immutable accumulator of
// token counts produced during assistant Events.
//
// Usage is a commutative monoid under Add: it has an identity element (Zero)
// and an associative, commutative binary operation (Add).
package usage

// Usage holds token counts accumulated during a single assistant Event or a
// sum of many Events. It is an immutable value object; Add never mutates the
// receiver.
type Usage struct {
	Input         int64
	Output        int64
	CacheRead     int64
	CacheCreation int64
}

// Zero returns the identity element of the Usage monoid.
// Zero.Add(a) == a and a.Add(Zero) == a for any Usage a.
func Zero() Usage {
	return Usage{}
}

// Add returns a new Usage whose counters are the element-wise sum of u and
// other. Neither u nor other is modified.
func (u Usage) Add(other Usage) Usage {
	return Usage{
		Input:         u.Input + other.Input,
		Output:        u.Output + other.Output,
		CacheRead:     u.CacheRead + other.CacheRead,
		CacheCreation: u.CacheCreation + other.CacheCreation,
	}
}

// IsZero reports whether u is the zero value (identity element). Useful for
// distinguishing real assistant events from content-split duplicates whose
// usage has been zeroed at the JSONL adapter level.
func (u Usage) IsZero() bool {
	return u == Usage{}
}
