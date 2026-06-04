package bis

import (
	"fmt"
	"sort"
	"strings"

	"github.com/amdrake93/eq2-eof-itemdex/internal/constants"
	"github.com/amdrake93/eq2-eof-itemdex/internal/store"
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

func rarityTag(it store.ScorableItem) string {
	if IsAvatar(it) {
		return it.Tier + " · avatar"
	}
	return it.Tier
}

func writeScored(b *strings.Builder, s ScoredItem) {
	fmt.Fprintf(b, "- **%s** [%s] — +%.1f DPS", s.Item.Name, rarityTag(s.Item), s.Delta)
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
				line := fmt.Sprintf("BiS: **%s**", strings.Join(names, "**, **"))
				if len(sr.Ranked) == 0 {
					line += " _(fixed)_"
				}
				fmt.Fprintf(&b, "%s\n\n", line)
			}
			for _, s := range sr.Ranked {
				writeScored(&b, s)
			}
			b.WriteString("\n")
		}
	}
	writeProgression(&b, reports)
	writeAssumptions(&b)
	return b.String()
}

func writeProgression(b *strings.Builder, reports []BaselineReport) {
	type pick struct {
		tier string
		item ScoredItem
	}
	bySlot := map[string][]pick{}
	var slotOrder []string
	for _, r := range reports {
		for _, sr := range r.Reports {
			if len(sr.Ranked) == 0 {
				continue
			}
			if _, seen := bySlot[sr.Slot]; !seen {
				slotOrder = append(slotOrder, sr.Slot)
			}
			bySlot[sr.Slot] = append(bySlot[sr.Slot], pick{tier: r.Name, item: sr.Ranked[0]})
		}
	}
	sort.Strings(slotOrder)

	b.WriteString("---\n\n## Progression (per slot)\n\n")
	b.WriteString("_Top pick per accessibility tier. ΔDPS is in each tier's own buff context, so values are not directly comparable across tiers._\n\n")
	for _, slot := range slotOrder {
		fmt.Fprintf(b, "### %s\n", slot)
		for _, p := range bySlot[slot] {
			fmt.Fprintf(b, "- %-12s **%s** [%s] (+%.1f)\n", p.tier, p.item.Item.Name, rarityTag(p.item.Item), p.item.Delta)
		}
		b.WriteString("\n")
	}
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
