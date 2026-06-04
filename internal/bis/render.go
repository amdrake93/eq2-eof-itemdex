package bis

import (
	"fmt"
	"sort"
	"strings"

	"github.com/amdrake93/eq2-eof-itemdex/internal/constants"
)

// BaselineReport is one baseline's converged weights + per-slot reports.
type BaselineReport struct {
	Name    string
	Weights map[string]float64
	Reports []SlotReport
}

func writeWeightTable(b *strings.Builder, weights map[string]float64) {
	keys := make([]string, 0, len(weights))
	for k := range weights {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return weights[keys[i]] > weights[keys[j]] })
	b.WriteString("| stat | weight |\n|---|---:|\n")
	for _, k := range keys {
		fmt.Fprintf(b, "| %s | %.4f |\n", k, weights[k])
	}
	b.WriteString("\n")
}

func writeScored(b *strings.Builder, s ScoredItem) {
	fmt.Fprintf(b, "- **%s** — +%.1f DPS", s.Item.Name, s.Delta)
	if s.Item.GameLink != "" {
		fmt.Fprintf(b, " ([item](%s))", s.Item.GameLink)
	}
	b.WriteString("\n")
	for i, term := range s.Terms {
		if i >= 4 {
			break
		}
		fmt.Fprintf(b, "  - %s %.0f × %.2f = %.1f\n", term.Stat, term.ItemValue, term.Weight, term.Contribution)
	}
}

func writeTier(b *strings.Builder, label string, items []ScoredItem) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintf(b, "_%s_\n\n", label)
	for _, s := range items {
		writeScored(b, s)
	}
	b.WriteString("\n")
}

// Render produces the full markdown BiS report across all baselines.
func Render(reports []BaselineReport) string {
	var b strings.Builder
	b.WriteString("# Assassin EoF Best-in-Slot\n\n")
	b.WriteString("_Per-item numbers are in-context ΔDPS at the converged set; the `stat × weight` lines are the explainable breakdown at the converged-baseline weights._\n\n")
	for _, r := range reports {
		fmt.Fprintf(&b, "## %s\n\n", r.Name)
		b.WriteString("Converged stat weights (marginal DPS per +1 stat):\n\n")
		writeWeightTable(&b, r.Weights)
		for _, sr := range r.Reports {
			fmt.Fprintf(&b, "### %s\n\n", sr.Slot)
			if len(sr.Chosen) > 0 {
				names := make([]string, 0, len(sr.Chosen))
				for _, c := range sr.Chosen {
					names = append(names, c.Name)
				}
				fmt.Fprintf(&b, "BiS: **%s**\n\n", strings.Join(names, "**, **"))
			}
			writeTier(&b, "Mythical (ceiling)", sr.Mythical)
			writeTier(&b, "Fabled", sr.Fabled)
			writeTier(&b, "Legendary", sr.Legendary)
		}
	}
	writeAssumptions(&b)
	return b.String()
}

func writeAssumptions(b *strings.Builder) {
	b.WriteString("---\n\n## Assumptions & Constants\n\n")
	fmt.Fprintf(b, "- crit ×%.2f; flurry ×%.1f; ability-mod cap = %.0f%% of potency-adjusted CA base\n",
		constants.CritMultiplier, constants.FlurryMultiplier, constants.AbilityModCapFrac*100)
	fmt.Fprintf(b, "- haste & dps-mod: shared diminishing curve, hard cap %.0f stat → 125%%\n", constants.HasteStatCap)
	fmt.Fprintf(b, "- reuse halves recast at %.0f%%; CA cast+recovery = %.2fs; fight = %.0fs\n",
		constants.ReuseHalvesAt, constants.CACastTimeSecs+constants.CARecoverySecs, constants.FightDurationSecs)
	b.WriteString("- Set built by coordinate-ascent to convergence (caps/interactions resolved at the live set baseline).\n")
	b.WriteString("- Main-hand is fixed (Soulfire); its own gear stats are not folded into the baseline. See docs/design-plan2.md §3.1.\n")
}
