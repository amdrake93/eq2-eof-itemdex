package bis

import (
	"fmt"
	"sort"
	"strings"

	"github.com/amdrake93/eq2-eof-itemdex/internal/constants"
	"github.com/amdrake93/eq2-eof-itemdex/internal/model"
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
func Render(reports []BaselineReport, fightLen float64) string {
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
	writeAssumptions(&b, fightLen)
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

func writeAssumptions(b *strings.Builder, fightLen float64) {
	b.WriteString("---\n\n## Assumptions & Constants\n\n")
	fmt.Fprintf(b, "- crit ×%.2f; flurry ×%.1f; ability-mod adds in full (50%% cap disproven by tooltip probes)\n",
		constants.CritMultiplier, constants.FlurryMultiplier)
	fmt.Fprintf(b, "- haste & dps-mod: fitted quadratic %.6f·s − %.8f·s², hard cap %.0f stat → %.0f%%\n",
		model.HasteDpsModA, model.HasteDpsModB, constants.HasteStatCap, model.HasteDpsModEffect(constants.HasteStatCap))
	fmt.Fprintf(b, "- main stat (AGI): interpolated 13-reading curve, hard cap 1100 → %.0f%%; multiplies BOTH CA and auto-attack damage (same curve)\n",
		model.MainStatEffect(1100))
	b.WriteString("- ⚠ CA potency pool includes a per-character calibrated `potency_bonus` whose source is UNEXPLAINED (~23.4 pts survive naked/AA-less/buff-less — spec §12 'potency-pool mystery', actively hunted)\n")
	fmt.Fprintf(b, "- reuse: 1%%/pt to the %.0f-stat cap, sharing each art's %.0f%%-of-base recast ceiling with AA art mods; cast speed divides cast times; recovery base %.2fs (reduced by recovery speed); fight target = %.0fs (smoothed)\n",
		constants.ReuseCapStat, constants.RecastReductionCeiling*100, constants.CARecoveryBaseSecs, fightLen)
	b.WriteString("- Set built by coordinate-ascent to convergence (caps/interactions resolved at the live set baseline).\n")
	b.WriteString("- Main-hand is fixed (Soulfire Sabre); its weapon damage AND full stat line are included in the baseline.\n")
}
