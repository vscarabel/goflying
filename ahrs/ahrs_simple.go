package ahrs

import (
	_ "log"
	"math"
	"github.com/skelterjohn/go.matrix"
)

const (
	MinDT float64 = 1e-6 // Below this time interval, don't recalculate
	MaxDT float64 = 10   // Above this time interval, re-initialize--too stale
	MinGS float64 = 10   // Below this GS, don't use any GPS data
	K     float64 = 0.9  // Reversion constant
)

type SimpleState struct {
	State
	rollGPS, pitchGPS, headingGPS float64 // GPS-derived attitude, Deg
	roll, pitch, heading          float64 // Fused attitude, Deg
	w1, w2, w3, gs                float64 // Groundspeed & ROC tracking, Kts
	tr                            float64 // turn rate, Rad/s
}

func InitializeSimple(m *Measurement) (s *SimpleState) {
	s = new(SimpleState)
	s.M = matrix.Zeros(32, 32)
	s.N = matrix.Zeros(32, 32)
	s.init(m)
	return
}

func (s *SimpleState) init(m *Measurement) {
	s.T = m.T
	if m.WValid {
		s.gs = math.Hypot(m.W1, m.W2)
		s.w1 = m.W1
		s.w2 = m.W2
		s.w3 = m.W3
	} else {
		s.gs = 0
		s.w1 = 0
		s.w2 = 0
		s.w3 = 0
	}

	s.tr = 0
	s.rollGPS = 0
	if s.gs > MinGS {
		s.headingGPS = math.Atan2(m.W1, m.W2)
		s.pitchGPS = math.Atan2(m.W3, s.gs)
	} else {
		s.headingGPS = Pi/2
		s.pitchGPS = 0
	}

	s.roll = s.rollGPS
	s.pitch = s.pitchGPS
	s.heading = s.headingGPS

	s.E0, s.E1, s.E2, s.E3 = ToQuaternion(s.roll, s.pitch, s.heading)
}

func (s *SimpleState) Compute(m *Measurement) {
	s.Predict(m.T)
	s.Update(m)
}

func (s *SimpleState) Predict(t float64) {
	return
}

func (s *SimpleState) Update(m *Measurement) {
	dt := m.T - s.T
	if dt < MinDT {
		return
	}
	if dt > MaxDT {
		s.init(m)
		return
	}

	if m.WValid {
		s.gs = math.Hypot(m.W1, m.W2)
	}

	if m.WValid && s.gs > MinGS {
		s.tr = 0.9*s.tr + 0.1*(m.W2*(m.W1-s.w1)-m.W1*(m.W2-s.w2))/(s.gs*s.gs)/dt
		s.rollGPS = math.Atan(s.gs*s.tr/G)
		s.pitchGPS = math.Atan2(m.W3, s.gs)
		s.headingGPS = math.Atan2(m.W1, m.W2)
		s.w1 = m.W1
		s.w2 = m.W2
		s.w3 = m.W3
	} else {
		s.tr = 0
		s.rollGPS = s.roll
		s.pitchGPS = s.pitch
		s.headingGPS = s.heading
		s.w1 = 0
		s.w2 = 0
		s.w3 = 0
	}

	q0, q1, q2, q3 := s.E0, s.E1, s.E2, s.E3
	dq0, dq1, dq2, dq3 := QuaternionRotate(q0, q1, q2, q3, m.B1*Deg*dt, m.B2*Deg*dt, m.B3*Deg*dt)
	dq0 -= q0
	dq1 -= q1
	dq2 -= q2
	dq3 -= q3
	//log.Printf(" q: %1.4f, %1.4f, %1.4f, %1.4f\n", q0, q1, q2, q3)
	//log.Printf("dq: %1.4f, %1.4f, %1.4f, %1.4f\n", dq0, dq1, dq2, dq3)

	rx := 2 * (q0*q1 + q2*q3)
	ry := q0*q0 - q1*q1 - q2*q2 - q3*q3 // TODO westphae: this is wrong
	drx := 2 * (q1*dq0 + q0*dq1 + q3*dq2 + q2*dq3)
	dry := 2 * (q0*dq0 - q1*dq1 - q2*dq2 + q3*dq3)
	dr := (ry*drx - rx*dry) / (rx*rx + ry*ry)
	//log.Printf(" rx,  ry: %f, %f\n", rx, ry)
	//log.Printf("drx, dry: %f, %f\n", drx, dry)
	//log.Printf("dr, db: %f, %f\n", dr, m.B1*dt)

	px := -2 * (q0*q2 - q3*q1)
	py := q0*q0 + q1*q1 + q2*q2 + q3*q3
	dpx := -2 * (q2*dq0 - q3*dq1 + q0*dq2 - q1*dq3)
	dpy := 2 * (q0*dq0 + q1*dq1 + q2*dq2 + q3*dq3)
	dp := (py*dpx - px*dpy) / (py * math.Sqrt(py*py-px*px))
	//log.Printf(" px,  py: %f, %f\n", px, py)
	//log.Printf("dpx, dpy: %f, %f\n", dpx, dpy)
	//log.Printf("dp, db: %f, %f\n", dp, -m.B2*dt)

	hx := -2 * (q0*q3 - q1*q2)
	hy := q0*q0 + q1*q1 - q2*q2 - q3*q3
	dhx := -2 * (q3*dq0 - q2*dq1 - q1*dq2 + q0*dq3)
	dhy := 2 * (q0*dq0 + q1*dq1 - q2*dq2 - q3*dq3)
	dh := (hy*dhx - hx*dhy) / (hx*hx + hy*hy)
	//log.Printf(" hx,  hy: %f, %f\n", hx, hy)
	//log.Printf("dhx, dhy: %f, %f\n", dhx, dhy)
	//log.Printf("dh, db: %f, %f\n", dh, -m.B3*dt)

	// This won't work around the poles
	if (s.pitch-s.pitchGPS)*dp > 0 {
		dp *= K
	}
	if (s.roll-s.rollGPS)*dr > 0 {
		dr *= K
	}
	ddh := s.heading - s.headingGPS
	if ddh > Pi {
		ddh -= 2*Pi
	} else if ddh < -Pi {
		ddh += 2*Pi
	}
	if ddh*dh > 0 {
		dh *= K
	}

	s.pitch += dp
	s.roll += dr
	s.heading += dh

	s.roll, s.pitch, s.heading = Regularize(s.roll, s.pitch, s.heading)

	s.E0, s.E1, s.E2, s.E3 = ToQuaternion(s.roll, s.pitch, s.heading)
	s.T = m.T
}

func (s *SimpleState) Valid() (ok bool) {
	return true
}

func (s *SimpleState) CalcRollPitchHeading() (roll float64, pitch float64, heading float64) {
	roll, pitch, heading = s.roll, s.pitch, s.heading
	return
}

func (s *SimpleState) CalcGPSRollPitchHeading() (roll float64, pitch float64, heading float64) {
	roll, pitch, heading = s.rollGPS, s.pitchGPS, s.headingGPS
	return
}

func (s *SimpleState) CalcRollPitchHeadingUncertainty() (droll float64, dpitch float64, dheading float64) {
	return
}

// GetState returns the State embedded in any object that implements AHRSProvider
func (s *SimpleState) GetState() (*State) {
	return &s.State
}


// PredictMeasurement doesn't do anything for the Simple method
func (s *SimpleState) PredictMeasurement() *Measurement {
	return NewMeasurement()
}
