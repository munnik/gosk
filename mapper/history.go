package mapper

import (
	"math"
	"time"

	"github.com/munnik/gosk/message"
)

// The functions in this file read a path's history - the []SingleValueMapped
// an aggregate expression gets as history['some_path'], oldest first, the
// newest entry being the value that just arrived - and say something about
// how the value moves. They exist so a notification can ask whether a new
// reading is one the recent past makes plausible, instead of only whether
// it is inside a fixed range.
//
// Non-numeric entries are skipped rather than failing the whole window:
// history is shared with everything else that reads the path, and one
// malformed sample should not silence a check for the length of the
// retention window.

// historyNumbers returns the timestamps and values of the numeric entries
// of values, in the order they are stored.
func historyNumbers(values []message.SingleValueMapped) ([]time.Time, []float64) {
	ts := make([]time.Time, 0, len(values))
	xs := make([]float64, 0, len(values))
	for _, v := range values {
		if x, ok := asFloat64(v.Value); ok {
			ts = append(ts, v.Timestamp)
			xs = append(xs, x)
		}
	}
	return ts, xs
}

// asFloat64 converts the numeric types a mapped value can carry. expr
// hands integer literals to a Go function as int, and a mapper may well
// have produced an int, so a float64 type assertion alone is not enough.
func asFloat64(value any) (float64, bool) {
	switch x := value.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int8:
		return float64(x), true
	case int16:
		return float64(x), true
	case int32:
		return float64(x), true
	case int64:
		return float64(x), true
	case uint:
		return float64(x), true
	case uint8:
		return float64(x), true
	case uint16:
		return float64(x), true
	case uint32:
		return float64(x), true
	case uint64:
		return float64(x), true
	}
	return 0, false
}

// Delta is the change over the history window: the newest numeric value
// minus the oldest. With a retentionTime of five minutes on the path that
// is "how much did this move in the last five minutes", which is the
// question a rate-of-change bound asks. Fewer than two numeric samples give
// 0: nothing has moved yet.
func Delta(values []message.SingleValueMapped) float64 {
	_, xs := historyNumbers(values)
	if len(xs) < 2 {
		return 0
	}
	return xs[len(xs)-1] - xs[0]
}

// RateOfChange is Delta per second, from the oldest to the newest numeric
// sample. Two samples with the same timestamp, or fewer than two samples,
// give 0 rather than infinity.
func RateOfChange(values []message.SingleValueMapped) float64 {
	ts, xs := historyNumbers(values)
	if len(xs) < 2 {
		return 0
	}
	dt := ts[len(ts)-1].Sub(ts[0]).Seconds()
	if dt <= 0 {
		return 0
	}
	return (xs[len(xs)-1] - xs[0]) / dt
}

// KalmanResult is what Kalman says about the newest sample of a window.
//
// Prediction is where the filter expected the newest sample to be from the
// samples before it, Innovation is how far the sample landed from that,
// and Sigma is the standard deviation the filter attached to that
// prediction. Surprise is their ratio, abs(Innovation) / Sigma: a unitless
// z-score, so one notify threshold ("surprise above 5") serves a tank in
// m³ and a thermometer in K alike. Estimate and Rate are the filtered
// value and its slope after taking the newest sample in.
type KalmanResult struct {
	Estimate   float64
	Rate       float64
	Prediction float64
	Innovation float64
	Sigma      float64
	Surprise   float64
	Samples    int
}

// Kalman runs a constant-velocity Kalman filter over the history window and
// reports on its newest sample. The state is the value and its rate of
// change; processNoise is the spectral density of the acceleration the
// model allows (units²/s³: how quickly the rate itself may drift), and
// measurementNoise is the variance of a single reading (units²: the sensor's
// own jitter squared). A reading the model did not see coming - a tank
// that jumps two cubic metres between two polls - shows up as a Surprise
// well above the handful of sigma that noise explains, while a bunkering
// that starts is a rate change the process noise absorbs within a few
// samples.
//
// The filter restarts from the oldest sample on every call rather than
// carrying state between calls: the window is bounded by the path's
// retention time, a few hundred samples at most, and a stateless pass
// means a restart of gosk or a gap in the data cannot leave it believing
// a stale trend. It is initialised with the first sample's value and an
// unknown rate (a large variance on the rate, none on the value beyond
// the measurement noise), so the first few samples have a large Sigma and
// a Surprise near zero: the filter does not raise anything before it has
// seen enough to have an opinion.
//
// Fewer than two numeric samples give a result with Surprise 0 and Sigma
// sqrt(measurementNoise); a non-positive measurementNoise is clamped to a
// tiny positive value so the result stays finite.
func Kalman(values []message.SingleValueMapped, processNoise any, measurementNoise any) KalmanResult {
	q, _ := asFloat64(processNoise)
	r, _ := asFloat64(measurementNoise)
	if r <= 0 {
		r = 1e-12
	}
	if q < 0 {
		q = 0
	}
	ts, xs := historyNumbers(values)
	n := len(xs)
	result := KalmanResult{Samples: n, Sigma: math.Sqrt(r)}
	if n == 0 {
		return result
	}
	// State x = (value, rate), covariance P. The rate starts unknown: a
	// variance far above anything a real rate squared could be, scaled to
	// the measurement noise so it is large in the sensor's own units.
	x0, x1 := xs[0], 0.0
	p00, p01, p11 := r, 0.0, 1e6*r+1
	result.Estimate, result.Prediction = x0, x0
	for i := 1; i < n; i++ {
		dt := ts[i].Sub(ts[i-1]).Seconds()
		if dt > 0 {
			// Predict: x = F x, P = F P Fᵀ + Q with F = [1 dt; 0 1] and Q the
			// continuous white-noise-acceleration model q·[dt³/3 dt²/2; dt²/2 dt].
			x0 += dt * x1
			np00 := p00 + dt*(p01+p01) + dt*dt*p11 + q*dt*dt*dt/3
			np01 := p01 + dt*p11 + q*dt*dt/2
			np11 := p11 + q*dt
			p00, p01, p11 = np00, np01, np11
		}
		// Innovation of the newest sample against the prediction.
		y := xs[i] - x0
		s := p00 + r
		if i == n-1 {
			result.Prediction = x0
			result.Innovation = y
			result.Sigma = math.Sqrt(s)
			result.Surprise = math.Abs(y) / result.Sigma
		}
		// Update with Kalman gain K = P Hᵀ / S, H = [1 0].
		k0, k1 := p00/s, p01/s
		x0 += k0 * y
		x1 += k1 * y
		np00 := (1 - k0) * p00
		np01 := (1 - k0) * p01
		np11 := p11 - k1*p01
		p00, p01, p11 = np00, np01, np11
	}
	result.Estimate = x0
	result.Rate = x1
	return result
}
