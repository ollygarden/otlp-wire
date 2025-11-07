# Performance Benchmarks

Comparison of otlp-wire operations vs traditional unmarshal/marshal approaches.

**Test Setup:**
- Platform: Apple M4
- Data: 5 resources, 100 data points each per resource
- Go version: 1.24.5

## Count Operations

### Metrics

| Method | Time | Memory | Allocations | Speedup |
|--------|------|--------|-------------|---------|
| Wire Format | 2.2 μs | 0 B | 0 | **baseline** |
| Unmarshal | 77.0 μs | 143 KB | 5,161 | **35x slower** |

**Result:** Wire format counting is **35x faster** with **zero allocations**.

### Traces

| Method | Time | Memory | Allocations | Speedup |
|--------|------|--------|-------------|---------|
| Wire Format | 1.9 μs | 0 B | 0 | **baseline** |
| Unmarshal | 99.4 μs | 217 KB | 5,131 | **52x slower** |

**Result:** Wire format counting is **52x faster** with **zero allocations**.

### Logs

| Method | Time | Memory | Allocations | Speedup |
|--------|------|--------|-------------|---------|
| Wire Format | 2.1 μs | 0 B | 0 | **baseline** |
| Unmarshal | 99.7 μs | 198 KB | 6,131 | **48x slower** |

**Result:** Wire format counting is **48x faster** with **zero allocations**.

---

## Split Operations

### Metrics

| Method | Time | Memory | Allocations | Speedup |
|--------|------|--------|-------------|---------|
| Wire Format | 134 ns | 488 B | 5 | **baseline** |
| Unmarshal + Remarshal | 133 μs | 281 KB | 7,742 | **997x slower** |

**Result:** Wire format splitting is **~1000x faster** with **99.8% fewer allocations**.

### Traces

| Method | Time | Memory | Allocations | Speedup |
|--------|------|--------|-------------|---------|
| Wire Format | 136 ns | 488 B | 5 | **baseline** |
| Unmarshal + Remarshal | 169 μs | 432 KB | 7,192 | **1242x slower** |

**Result:** Wire format splitting is **~1200x faster** with **99.9% fewer allocations**.

### Logs

| Method | Time | Memory | Allocations | Speedup |
|--------|------|--------|-------------|---------|
| Wire Format | 134 ns | 488 B | 5 | **baseline** |
| Unmarshal + Remarshal | 175 μs | 386 KB | 8,692 | **1302x slower** |

**Result:** Wire format splitting is **~1300x faster** with **99.9% fewer allocations**.

---

## Summary

### Counting Performance

- **35-52x faster** than unmarshaling
- **Zero allocations** vs thousands
- **Zero memory** usage vs 140-220 KB

**Use case:** Perfect for rate limiting, telemetry, monitoring.

### Splitting Performance

- **~1000x faster** than unmarshal+remarshal
- **99.8-99.9% fewer allocations**
- **<1 KB memory** vs 280-430 KB

**Use case:** Perfect for sharding, routing, parallel processing.

---

## Why Is It So Fast?

1. **No Unmarshaling** - Reads wire format tags directly
2. **Zero Allocations** - Counting requires no memory allocation
3. **Minimal Allocations** - Splitting only allocates slice storage
4. **No Struct Creation** - Skips creating thousands of Go objects
5. **Early Exit** - Stops parsing once count/split is complete

---

## Real-World Impact

### Rate Limiting 10,000 req/s

**Traditional Approach:**
- Counting: 77 μs × 10,000 = 770 ms CPU time/second
- **CPU usage: 77%** (single core)

**Wire Format Approach:**
- Counting: 2.2 μs × 10,000 = 22 ms CPU time/second
- **CPU usage: 2.2%** (single core)

**Savings:** **75% less CPU usage**

### Sharding 10,000 req/s

**Traditional Approach:**
- Splitting: 133 μs × 10,000 = 1,330 ms CPU time/second
- **CPU usage: 133%** (requires 2 cores)

**Wire Format Approach:**
- Splitting: 0.134 μs × 10,000 = 1.34 ms CPU time/second
- **CPU usage: 0.13%** (single core)

**Savings:** **99.9% less CPU usage**

---

## Running Benchmarks

To reproduce these results:

```bash
# Run all benchmarks
go test -bench=. -benchmem -benchtime=3s

# Run comparison benchmarks only
go test -bench='Count|Split' -benchmem -benchtime=3s

# Save results to file
go test -bench=. -benchmem -benchtime=3s > benchmark_results.txt
```

---

## Test Data Characteristics

All benchmarks use realistic OTLP data:

- **5 resources** with full resource attributes
- **100 data points** per resource (500 total)
- Full scope information (instrumentation library)
- Complete metadata (timestamps, attributes)
- Realistic attribute cardinality

This represents typical production telemetry data.
