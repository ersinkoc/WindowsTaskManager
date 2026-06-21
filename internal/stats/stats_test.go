package stats

import (
	"math"
	"sync"
	"testing"
)

// =============================================================================
// LinearRegression tests
// =============================================================================

func TestLinearRegression_Empty(t *testing.T) {
	slope, intercept, r2 := LinearRegression([]float64{}, []float64{})
	if slope != 0 || intercept != 0 || r2 != 0 {
		t.Errorf("empty input: got (%v, %v, %v), want (0, 0, 0)", slope, intercept, r2)
	}
}

func TestLinearRegression_SingleElement(t *testing.T) {
	slope, intercept, r2 := LinearRegression([]float64{1}, []float64{2})
	if slope != 0 || intercept != 0 || r2 != 0 {
		t.Errorf("single element: got (%v, %v, %v), want (0, 0, 0)", slope, intercept, r2)
	}
}

func TestLinearRegression_LengthMismatch(t *testing.T) {
	// len(xs) < len(ys)
	slope, intercept, r2 := LinearRegression([]float64{1, 2}, []float64{3, 4, 5})
	if slope != 0 || intercept != 0 || r2 != 0 {
		t.Errorf("xs < ys: got (%v, %v, %v), want (0, 0, 0)", slope, intercept, r2)
	}

	// len(xs) > len(ys)
	slope, intercept, r2 = LinearRegression([]float64{1, 2, 3}, []float64{4, 5})
	if slope != 0 || intercept != 0 || r2 != 0 {
		t.Errorf("xs > ys: got (%v, %v, %v), want (0, 0, 0)", slope, intercept, r2)
	}

	// Both empty (no length mismatch, but n < 2)
	slope, intercept, r2 = LinearRegression(nil, nil)
	if slope != 0 || intercept != 0 || r2 != 0 {
		t.Errorf("both nil: got (%v, %v, %v), want (0, 0, 0)", slope, intercept, r2)
	}
}

func TestLinearRegression_ZeroDenominator(t *testing.T) {
	// All x values identical — denom = n*sumX2 - sumX*sumX = 0
	xs := []float64{5, 5, 5, 5}
	ys := []float64{1, 2, 3, 4}
	slope, intercept, r2 := LinearRegression(xs, ys)
	// Expected: slope=0, intercept=meanY=2.5, rSquared=0
	if slope != 0 {
		t.Errorf("zero denom slope: got %v, want 0", slope)
	}
	if math.Abs(intercept-2.5) > 1e-12 {
		t.Errorf("zero denom intercept: got %v, want 2.5", intercept)
	}
	if r2 != 0 {
		t.Errorf("zero denom r2: got %v, want 0", r2)
	}
}

func TestLinearRegression_NormalCase(t *testing.T) {
	// Perfect linear relationship: y = 2x + 1
	xs := []float64{1, 2, 3, 4, 5}
	ys := []float64{3, 5, 7, 9, 11}
	slope, intercept, r2 := LinearRegression(xs, ys)

	if math.Abs(slope-2.0) > 1e-12 {
		t.Errorf("slope: got %v, want 2.0", slope)
	}
	if math.Abs(intercept-1.0) > 1e-12 {
		t.Errorf("intercept: got %v, want 1.0", intercept)
	}
	if math.Abs(r2-1.0) > 1e-12 {
		t.Errorf("r2: got %v, want 1.0 (perfect fit)", r2)
	}
}

func TestLinearRegression_SSTotZero(t *testing.T) {
	// All y values identical but xs vary — ssTot = 0, rSquared should be 1
	xs := []float64{1, 2, 3, 4}
	ys := []float64{7, 7, 7, 7}
	slope, intercept, r2 := LinearRegression(xs, ys)
	// slope=0, intercept=7, r2=1 (ssTot == 0 branch)
	if slope != 0 {
		t.Errorf("constant y slope: got %v, want 0", slope)
	}
	if intercept != 7.0 {
		t.Errorf("constant y intercept: got %v, want 7", intercept)
	}
	if math.Abs(r2-1.0) > 1e-12 {
		t.Errorf("constant y r2: got %v, want 1", r2)
	}
}

func TestLinearRegression_PartialFit(t *testing.T) {
	// Non-perfect fit: y = 2x + noise
	xs := []float64{1, 2, 3, 4, 5}
	ys := []float64{2.1, 4.9, 5.8, 8.1, 9.9}
	slope, intercept, r2 := LinearRegression(xs, ys)
	_ = intercept

	// Slope should be close to 2 but not exactly 2.
	if slope <= 0 {
		t.Errorf("expected positive slope, got %v", slope)
	}
	// R² should be high but less than 1.
	if r2 <= 0 || r2 > 1 {
		t.Errorf("r2 should be in (0, 1], got %v", r2)
	}
	// And definitely not 1 since residuals exist.
	if math.Abs(r2-1.0) < 1e-12 {
		t.Errorf("expected r2 < 1 for noisy data, got %v", r2)
	}
}

// =============================================================================
// RingBuffer tests
// =============================================================================

func TestNewRingBuffer_CapacityClamp(t *testing.T) {
	// Zero capacity clamps to 1.
	rb := NewRingBuffer[int](0)
	if rb == nil {
		t.Fatal("NewRingBuffer(0) returned nil")
	}
	if rb.capacity != 1 {
		t.Errorf("zero capacity: got %d, want 1", rb.capacity)
	}

	// Negative capacity also clamps to 1.
	rb = NewRingBuffer[int](-5)
	if rb.capacity != 1 {
		t.Errorf("negative capacity: got %d, want 1", rb.capacity)
	}

	// Valid capacity.
	rb = NewRingBuffer[int](10)
	if rb.capacity != 10 {
		t.Errorf("valid capacity: got %d, want 10", rb.capacity)
	}
	if rb.count != 0 {
		t.Errorf("fresh count: got %d, want 0", rb.count)
	}
	if rb.head != 0 {
		t.Errorf("fresh head: got %d, want 0", rb.head)
	}
}

func TestRingBuffer_PartialFill(t *testing.T) {
	rb := NewRingBuffer[int](5)
	rb.Add(10)
	rb.Add(20)
	rb.Add(30)

	if got := rb.Len(); got != 3 {
		t.Errorf("Len: got %d, want 3", got)
	}

	got := rb.Slice()
	want := []int{10, 20, 30}
	if !equalSlice(got, want) {
		t.Errorf("Slice: got %v, want %v", got, want)
	}
}

func TestRingBuffer_FullBuffer(t *testing.T) {
	rb := NewRingBuffer[int](3)
	rb.Add(1)
	rb.Add(2)
	rb.Add(3)

	if got := rb.Len(); got != 3 {
		t.Errorf("Len: got %d, want 3", got)
	}

	got := rb.Slice()
	want := []int{1, 2, 3}
	if !equalSlice(got, want) {
		t.Errorf("Slice: got %v, want %v", got, want)
	}
}

func TestRingBuffer_WrapAround(t *testing.T) {
	rb := NewRingBuffer[int](3)

	// Fill it.
	rb.Add(1)
	rb.Add(2)
	rb.Add(3)

	// Now overwrite; should retain the last 3 in order.
	rb.Add(4)
	rb.Add(5)

	got := rb.Slice()
	want := []int{3, 4, 5}
	if !equalSlice(got, want) {
		t.Errorf("after wrap: got %v, want %v", got, want)
	}
	if l := rb.Len(); l != 3 {
		t.Errorf("Len after wrap: got %d, want 3", l)
	}

	// More wraps — push several more cycles.
	rb.Add(6)
	rb.Add(7)
	rb.Add(8)
	rb.Add(9)

	got = rb.Slice()
	want = []int{7, 8, 9}
	if !equalSlice(got, want) {
		t.Errorf("after multiple wraps: got %v, want %v", got, want)
	}
}

func TestRingBuffer_ZeroCapacityThenAdd(t *testing.T) {
	// After capacity < 1 clamp, adding and reading should still work.
	rb := NewRingBuffer[string](0)
	rb.Add("hello")

	if got := rb.Len(); got != 1 {
		t.Errorf("Len: got %d, want 1", got)
	}

	got := rb.Slice()
	if len(got) != 1 || got[0] != "hello" {
		t.Errorf("Slice: got %v, want [hello]", got)
	}

	// Add another, previous should be overwritten.
	rb.Add("world")
	got = rb.Slice()
	if len(got) != 1 || got[0] != "world" {
		t.Errorf("Slice after overwrite: got %v, want [world]", got)
	}
}

func TestRingBuffer_SliceDoesNotMutate(t *testing.T) {
	rb := NewRingBuffer[int](3)
	rb.Add(1)
	rb.Add(2)

	s := rb.Slice()
	s[0] = 999

	again := rb.Slice()
	if again[0] != 1 {
		t.Errorf("Slice mutation leaked: got %v, want [1 2]", again)
	}
}

func TestRingBuffer_StringType(t *testing.T) {
	rb := NewRingBuffer[string](2)
	rb.Add("a")
	rb.Add("b")
	rb.Add("c") // overwrites "a"

	got := rb.Slice()
	want := []string{"b", "c"}
	if !equalSlice(got, want) {
		t.Errorf("string ring: got %v, want %v", got, want)
	}
}

func TestRingBuffer_ConcurrentSafe(t *testing.T) {
	// Hammer Add from many goroutines and confirm Slice returns a consistent
	// snapshot of length ≤ capacity. The race detector will flag any data race.
	rb := NewRingBuffer[int](50)

	const writers = 20
	const perWriter = 500

	var wg sync.WaitGroup
	wg.Add(writers)
	for i := 0; i < writers; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < perWriter; j++ {
				rb.Add(j)
			}
		}()
	}

	// Readers also race for snapshots.
	stop := make(chan struct{})
	var rwg sync.WaitGroup
	rwg.Add(4)
	for i := 0; i < 4; i++ {
		go func() {
			defer rwg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					s := rb.Slice()
					if len(s) > rb.capacity {
						t.Errorf("slice longer than capacity: %d", len(s))
					}
				}
			}
		}()
	}

	wg.Wait()
	close(stop)
	rwg.Wait()

	// After all writes, buffer should be full.
	if got := rb.Len(); got != 50 {
		t.Errorf("final Len: got %d, want 50", got)
	}
}

// =============================================================================
// Welford tests
// =============================================================================

func TestWelford_New(t *testing.T) {
	w := NewWelford()
	if w == nil {
		t.Fatal("NewWelford returned nil")
	}
	if w.Count() != 0 {
		t.Errorf("fresh count: got %d, want 0", w.Count())
	}
	if w.Mean() != 0 {
		t.Errorf("fresh mean: got %v, want 0", w.Mean())
	}
	if w.Variance() != 0 {
		t.Errorf("fresh variance: got %v, want 0", w.Variance())
	}
	if w.StdDev() != 0 {
		t.Errorf("fresh stddev: got %v, want 0", w.StdDev())
	}
}

func TestWelford_SingleValue(t *testing.T) {
	w := NewWelford()
	w.Add(42.0)

	if w.Count() != 1 {
		t.Errorf("count: got %d, want 1", w.Count())
	}
	if w.Mean() != 42.0 {
		t.Errorf("mean: got %v, want 42", w.Mean())
	}
	// count < 2 → Variance returns 0
	if w.Variance() != 0 {
		t.Errorf("variance with n=1: got %v, want 0", w.Variance())
	}
	if w.StdDev() != 0 {
		t.Errorf("stddev with n=1: got %v, want 0", w.StdDev())
	}
	// count < 10 → IsAnomaly always false regardless of nSigma
	if w.IsAnomaly(1000, 2.0) {
		t.Error("IsAnomaly with n<10 should return false")
	}
}

func TestWelford_FewValues(t *testing.T) {
	w := NewWelford()
	values := []float64{2, 4, 4, 4, 5, 5, 7, 9}
	for _, v := range values {
		w.Add(v)
	}

	// Known mean of [2,4,4,4,5,5,7,9] = 5
	if got := w.Mean(); math.Abs(got-5.0) > 1e-12 {
		t.Errorf("mean: got %v, want 5", got)
	}

	// Sample variance (n-1 denominator) of [2,4,4,4,5,5,7,9]:
	// mean=5, deviations squared: 9,1,1,1,0,0,4,16 → sum=32 → 32/(8-1) = 32/7 ≈ 4.5714
	expectedVar := 32.0 / 7.0
	if got := w.Variance(); math.Abs(got-expectedVar) > 1e-12 {
		t.Errorf("variance: got %v, want %v", got, expectedVar)
	}

	expectedStd := math.Sqrt(expectedVar)
	if got := w.StdDev(); math.Abs(got-expectedStd) > 1e-12 {
		t.Errorf("stddev: got %v, want %v", got, expectedStd)
	}

	if w.Count() != 8 {
		t.Errorf("count: got %d, want 8", w.Count())
	}

	// n < 10 → IsAnomaly always false.
	if w.IsAnomaly(100, 1.0) {
		t.Error("IsAnomaly should be false when count < 10")
	}
}

func TestWelford_ManyValues(t *testing.T) {
	w := NewWelford()
	// 100 values uniformly distributed from 1..100.
	var sumX, sumX2 float64
	const n = 100
	for i := 1; i <= n; i++ {
		v := float64(i)
		w.Add(v)
		sumX += v
		sumX2 += v * v
	}

	expectedMean := sumX / float64(n)
	if got := w.Mean(); math.Abs(got-expectedMean) > 1e-12 {
		t.Errorf("mean: got %v, want %v", got, expectedMean)
	}

	expectedVar := (sumX2 - sumX*sumX/float64(n)) / float64(n-1)
	if got := w.Variance(); math.Abs(got-expectedVar) > 1e-9 {
		t.Errorf("variance: got %v, want %v", got, expectedVar)
	}

	if w.Count() != n {
		t.Errorf("count: got %d, want %d", w.Count(), n)
	}
}

func TestWelford_IsAnomaly_NotAnomaly(t *testing.T) {
	w := NewWelford()
	// 20 values around 50 with small spread.
	for i := 0; i < 20; i++ {
		w.Add(50.0 + float64(i%3))
	}

	// 52 is within 1σ-ish — not anomalous.
	if w.IsAnomaly(52, 3.0) {
		t.Error("value close to mean should not be an anomaly at 3σ")
	}
}

func TestWelford_IsAnomaly_FarValue(t *testing.T) {
	w := NewWelford()
	for i := 0; i < 20; i++ {
		w.Add(50.0 + float64(i%3))
	}

	// 1000 is far from mean — should be anomaly even at huge nSigma when
	// nσ > |val-mean|/stddev. With small stddev this is clearly an outlier.
	if !w.IsAnomaly(1000, 2.0) {
		t.Error("value far from mean should be an anomaly")
	}
}

func TestWelford_IsAnomaly_NSigmaThreshold(t *testing.T) {
	w := NewWelford()
	// 50 stable values, mean ~50, stddev small.
	for i := 0; i < 50; i++ {
		w.Add(50.0 + float64(i%2))
	}

	mean := w.Mean()
	stddev := w.StdDev()

	// A value exactly at mean+stddev: anomaly at 1σ, not at 2σ.
	if !w.IsAnomaly(mean+stddev, 1.0) {
		t.Error("value at mean+1σ should be anomaly with nSigma=1 (strict >)")
	}
	if w.IsAnomaly(mean+stddev, 2.0) {
		t.Error("value at mean+1σ should not be anomaly with nSigma=2")
	}
}

// =============================================================================
// Helpers
// =============================================================================

func equalSlice[T comparable](a, b []T) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
