package fit

import "math"

// QuadParams is the quadratic diminishing-returns form f(s) = A·s − B·s².
type QuadParams struct{ A, B float64 }

func (q QuadParams) Eval(s float64) float64 { return q.A*s - q.B*s*s }

// Curve is any fitted form evaluable at a raw stat value.
type Curve interface{ Eval(s float64) float64 }

// FitQuad least-squares fits the quadratic to (Raw, FitTarget) pairs. Both
// parameters are linear, so the 2×2 normal equations solve in closed form:
//
//	a·Σs² − b·Σs³ = Σs·y
//	a·Σs³ − b·Σs⁴ = Σs²·y
func FitQuad(rs []Reading) QuadParams {
	var s2, s3, s4, sy, s2y float64
	for _, r := range rs {
		s, y := r.Raw, r.FitTarget()
		s2 += s * s
		s3 += s * s * s
		s4 += s * s * s * s
		sy += s * y
		s2y += s * s * y
	}

	det := -s2*s4 + s3*s3
	return QuadParams{
		A: (-sy*s4 + s3*s2y) / det,
		B: (s2*s2y - s3*sy) / det,
	}
}

// RMS is the root-mean-square residual of a fitted curve against the readings'
// fit targets.
func RMS(c Curve, rs []Reading) float64 {
	var rss float64
	for _, r := range rs {
		d := c.Eval(r.Raw) - r.FitTarget()
		rss += d * d
	}
	return math.Sqrt(rss / float64(len(rs)))
}
