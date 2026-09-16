package mapper

import (
	"time"

	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/logger"
	"go.uber.org/zap"
)

// maxPlausibleSpeedRatio bounds how much a candidate peakToPeakRpm reading
// may differ from the last accepted one before it's treated as noise: a
// shaft's rotational speed can't physically double, or halve, within a
// single revolution, so - unlike a magnitude threshold on the raw signal -
// this needs no calibration against a particular sensor's noise floor.
const maxPlausibleSpeedRatio = 2.0

// peakToPeakRpm estimates a shaft's rotational speed from a signal that
// completes one sinusoidal cycle per revolution - e.g. the difference
// between two torque sensors mounted 180 degrees apart, which isolates the
// once-per-revolution bending component (see
// notificationsForMannerTcpSensorHealth's doc comment in ../nix for why) -
// by timing consecutive local peaks and troughs, which are half a
// revolution apart. A candidate reading implying an implausible jump in
// speed since the last accepted one (see maxPlausibleSpeedRatio) is
// discarded as noise instead of updating the estimate, and the timing
// baseline for the next candidate is left at the last accepted extremum,
// so a run of noise-driven blips near a true peak or trough gets skipped
// over rather than compounding into repeated bad readings.
//
// It's shared by BinaryMapper and CanBusMapper: both decode a Manner shaft
// power meter, just over different transports, and both derive a speed
// estimate from the same physical signal.
type peakToPeakRpm struct {
	havePrevious  bool
	previousValue float64
	previousTime  time.Time

	haveTrend bool
	rising    bool // meaningful only once haveTrend is true

	haveExtremum bool
	extremumTime time.Time

	estimate float64 // revolutions per second, 0 until a plausible reading exists
}

// update feeds the latest sample of the periodic signal and its timestamp,
// and returns the current speed estimate in revolutions per second (0
// until a plausible reading exists).
func (p *peakToPeakRpm) update(value float64, t time.Time) float64 {
	if !p.havePrevious {
		p.havePrevious = true
		p.previousValue, p.previousTime = value, t
		return p.estimate
	}

	rising, falling := value > p.previousValue, value < p.previousValue
	reversed := p.haveTrend && ((p.rising && falling) || (!p.rising && rising))

	if reversed {
		// previousValue, at previousTime, was a local peak or trough
		if p.haveExtremum {
			if halfPeriod := p.previousTime.Sub(p.extremumTime).Seconds(); halfPeriod > 0 {
				if candidate := 0.5 / halfPeriod; p.isPlausible(candidate) {
					p.estimate = candidate
					p.extremumTime = p.previousTime
				}
				// else: noise - leave extremumTime where it is, so the
				// next candidate is still timed from the last good one
			}
		} else {
			p.haveExtremum = true
			p.extremumTime = p.previousTime
		}
	}

	if rising || falling {
		p.rising, p.haveTrend = rising, true
	}
	p.previousValue, p.previousTime = value, t
	return p.estimate
}

func (p *peakToPeakRpm) isPlausible(candidate float64) bool {
	if p.estimate <= 0 {
		return true // nothing accepted yet: anything is a plausible start
	}
	ratio := candidate / p.estimate
	return ratio >= 1/maxPlausibleSpeedRatio && ratio <= maxPlausibleSpeedRatio
}

// evaluatePeakToPeak evaluates mc against env, and if it produces a number,
// feeds it into pp and publishes the resulting estimate into env under
// envVar. Errors are logged and otherwise ignored - env keeps whatever
// pp's last accepted estimate already was (or its zero value).
func evaluatePeakToPeak(env ExpressionEnvironment, mc *config.MappingConfig, pp *peakToPeakRpm, envVar string, now time.Time) {
	output, err := runExpr(env, mc)
	if err != nil {
		logger.GetLogger().Warn(
			"Could not evaluate a peak-to-peak estimate's expression",
			zap.String("EnvVar", envVar),
			zap.String("Error", err.Error()),
		)
		return
	}
	value, ok := output.(float64)
	if !ok {
		logger.GetLogger().Warn(
			"A peak-to-peak estimate's expression did not evaluate to a number",
			zap.String("EnvVar", envVar),
		)
		return
	}
	env[envVar] = pp.update(value, now)
}
