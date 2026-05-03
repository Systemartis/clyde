# Domain Spec: Usage

## Value Object: Usage

Usage is an immutable value object that holds token counts accumulated during assistant Events.

### Structure

```
Usage {
  Input:         int64
  Output:        int64
  CacheRead:     int64
  CacheCreation: int64
}
```

All counters are non-negative integers.

### Laws (Monoid under Add)

Usage MUST satisfy the following algebraic laws for the `Add` operation:

| Law | Statement |
|-----|-----------|
| Identity (left) | `Zero().Add(a)` equals `a` for any Usage `a` |
| Identity (right) | `a.Add(Zero())` equals `a` for any Usage `a` |
| Commutativity | `a.Add(b)` equals `b.Add(a)` for any Usage `a, b` |
| Associativity | `a.Add(b).Add(c)` equals `a.Add(b.Add(c))` for any Usage `a, b, c` |

### Constructor

**Zero()** Usage

Returns the identity element (all counters zero).

### Method

**Add**(other Usage) Usage

Combines two Usage values by summing each counter independently. Returns a NEW Usage value without mutating the receiver.

| Input a | Input b | Result |
|---------|---------|--------|
| {3, 4, 1, 2} | {1, 2, 3, 4} | {4, 6, 4, 6} |

### Invariants

- Usage is immutable. `Add` always returns a new value.
- The receiver and argument to `Add` are never modified.
- All counter values remain non-negative after `Add`.

## Out of Scope for V1

- Serialization to JSON or other formats (handled by adapters)
- Cost calculation or pricing models (deferred to `usage-and-cost-panel`)
