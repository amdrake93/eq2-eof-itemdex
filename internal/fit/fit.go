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
//
// Requires ≥2 readings with distinct raw values; returns NaN params otherwise.
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

// LogParams is the logarithmic diminishing-returns form f(s) = A·ln(1 + s/B).
type LogParams struct{ A, B float64 }

func (l LogParams) Eval(s float64) float64 { return l.A * math.Log(1+s/l.B) }

// FitLog scans B over a 1% geometric grid (1 → 20000); for each B the best A is
// linear least squares over g = ln(1+s/B). Deterministic and plenty precise for
// a residual bake-off against the quadratic. Degenerate input (no readings, or
// all-zero raw values) yields unusable params (zero-value or NaN) rather than an
// error, matching FitQuad's convention.
func FitLog(rs []Reading) LogParams {
	best := LogParams{}
	bestRSS := math.Inf(1)

	for b := 1.0; b < 20000; b *= 1.01 {
		var gg, gy float64
		for _, r := range rs {
			g := math.Log(1 + r.Raw/b)
			gg += g * g
			gy += g * r.FitTarget()
		}
		a := gy / gg

		var rss float64
		for _, r := range rs {
			d := a*math.Log(1+r.Raw/b) - r.FitTarget()
			rss += d * d
		}
		if rss < bestRSS {
			bestRSS = rss
			best = LogParams{A: a, B: b}
		}
	}
	return best
}
