// fitcurve fits the shared haste/dps-mod conversion curve from
// data/curve-readings.csv and prints paste-ready constants for
// internal/model/curve.go. Append new readings to the CSV and re-run; the
// TestFittedConstantsMatchReadings sync test fails until the constants are
// updated.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/amdrake93/eq2-eof-itemdex/internal/fit"
)

func main() {
	path := flag.String("readings", "data/curve-readings.csv", "curve readings csv")
	flag.Parse()

	rs, err := fit.LoadReadings(*path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "load readings:", err)
		os.Exit(1)
	}

	subsets := []struct {
		name string
		rs   []fit.Reading
	}{
		{"joint", rs},
		{"haste", fit.Filter(rs, "haste")},
		{"dpsmod", fit.Filter(rs, "dpsmod")},
	}
	for _, s := range subsets {
		q, l := fit.FitQuad(s.rs), fit.FitLog(s.rs)
		fmt.Printf("%-7s (n=%2d)  quad a=%.6f b=%.8f rms=%.4f peak=%.1f f(300)=%.2f\n",
			s.name, len(s.rs), q.A, q.B, fit.RMS(q, s.rs), q.A/(2*q.B), q.Eval(300))
		fmt.Printf("                log  a=%.4f   b=%.3f      rms=%.4f f(300)=%.2f\n",
			l.A, l.B, fit.RMS(l, s.rs), l.Eval(300))
	}

	joint := fit.FitQuad(rs)
	jointLog := fit.FitLog(rs)
	winner := "quadratic"
	if fit.RMS(jointLog, rs) < fit.RMS(joint, rs) {
		winner = "logarithmic — model expects quadratic, investigate before recording"
	}
	fmt.Printf("\nwinner: %s\n", winner)
	fmt.Println("\n// paste into internal/model/curve.go:")
	fmt.Printf("HasteDpsModA = %.6f\n", joint.A)
	fmt.Printf("HasteDpsModB = %.8f\n", joint.B)
}
