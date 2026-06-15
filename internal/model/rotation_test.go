package model

import (
	"testing"

	"github.com/amdrake93/eq2-eof-itemdex/internal/spell"
	"github.com/stretchr/testify/require"
)

func TestRotationCADPS_Priority(t *testing.T) {
	// A: 100 dmg, 3s recast. B: 50 dmg, 1s recast. cast 1s + recovery 0 = 1s slot, fight 10s.
	// Hand-trace (priority = bigger hit, fire A whenever up, B fills):
	//   t0 A, t1 B, t2 B, t3 A, t4 B, t5 B, t6 A, t7 B, t8 B, t9 A  → A×4=400, B×6=300 = 700 /10s = 70.
	cas := []spell.CombatArt{
		{Name: "A", MinDamage: 100, MaxDamage: 100, RecastSecs: 3, CastSecsHundredths: 100},
		{Name: "B", MinDamage: 50, MaxDamage: 50, RecastSecs: 1, CastSecsHundredths: 100},
	}
	require.InDelta(t, 70.0, RotationCADPS(StatBlock{RecoverySpeed: 100}, cas, 10), 0.01)
}

func TestRotationCADPS_LowPriorityNeverFires(t *testing.T) {
	cas := []spell.CombatArt{
		{Name: "Big", MinDamage: 100, MaxDamage: 100, RecastSecs: 1, CastSecsHundredths: 100}, // always up (recast == slot)
		{Name: "Weak", MinDamage: 1, MaxDamage: 1, RecastSecs: 1, CastSecsHundredths: 100},
	}
	// cast 1s + recovery 0 = 1s slot; 10 slots all fire Big (100) → 1000/10 = 100; Weak never wins a slot.
	require.InDelta(t, 100.0, RotationCADPS(StatBlock{RecoverySpeed: 100}, cas, 10), 0.01)
}

func TestRotationCADPS_ReuseHelps(t *testing.T) {
	// reuse 100 is clamped to the 50-stat cap, which still halves recast.
	cas := []spell.CombatArt{{Name: "Nuke", MinDamage: 1000, MaxDamage: 1000, RecastSecs: 10}}
	base := RotationCADPS(StatBlock{}, cas, 600)
	fast := RotationCADPS(StatBlock{Reuse: 100}, cas, 600) // reuse capped at 50 → recast still halved → ~2× casts
	require.Greater(t, fast, base*1.8)
}

func TestRotationPerArtCastTime(t *testing.T) {
	// Two always-up arts (recast 0), equal damage, different cast times.
	// fast: cast 0.5s + recovery 0.25 = 0.75s/cast → 800 casts → 100/0.75
	// slow: cast 2.0s + recovery 0.25 = 2.25s/cast → ~266 casts → 100/2.25
	fast := []spell.CombatArt{{Name: "Fast", MinDamage: 100, MaxDamage: 100, RecastSecs: 0, CastSecsHundredths: 50}}
	slow := []spell.CombatArt{{Name: "Slow", MinDamage: 100, MaxDamage: 100, RecastSecs: 0, CastSecsHundredths: 200}}

	fastDPS := RotationCADPS(StatBlock{RecoverySpeed: 50}, fast, 600)
	slowDPS := RotationCADPS(StatBlock{RecoverySpeed: 50}, slow, 600)
	require.Greater(t, fastDPS, slowDPS, "the slower-cast art should yield less CADPS")
	require.InDelta(t, 100.0/0.75, fastDPS, 1e-6)
	// 600s isn't a whole multiple of the 2.25s slot, so the discrete sim fires one
	// partial-slot cast more than the continuous rate; tolerate up to one cast.
	require.InDelta(t, 100.0/2.25, slowDPS, 100.0/600)
}

func TestRotationPrioritizesByDPSPerCastTime(t *testing.T) {
	// Slow art has higher raw damage but worse per-second; fast art is lower raw
	// but better per-second. With DPS-per-cast-time priority the FAST art wins
	// every slot, so CADPS is its rate — not the slow art's higher-raw rate.
	cas := []spell.CombatArt{
		{Name: "Slow", MinDamage: 1287, MaxDamage: 1287, RecastSecs: 0, CastSecsHundredths: 150}, // slot 1.75 → 735/s
		{Name: "Fast", MinDamage: 891, MaxDamage: 891, RecastSecs: 0, CastSecsHundredths: 50},    // slot 0.75 → 1188/s
	}
	dps := RotationCADPS(StatBlock{RecoverySpeed: 50}, cas, 600)
	require.InDelta(t, 891.0/0.75, dps, 0.01, "picks higher DPS-per-cast-time (fast), not higher raw damage (slow)")
}

func TestCAEffectiveDamageMeasuredEquation(t *testing.T) {
	ca := spell.CombatArt{Name: "X", MinDamage: 800, MaxDamage: 1200}
	require.InDelta(t, 1000.0, CAEffectiveDamage(StatBlock{}, ca), 0.01)            // avg 1000, no stats
	require.InDelta(t, 1100.0, CAEffectiveDamage(StatBlock{Potency: 10}, ca), 0.01) // ×1.1

	// PotencyBonus pools additively with potency (⚠ §12: includes the unexplained innate).
	require.InDelta(t, 1300.0, CAEffectiveDamage(StatBlock{Potency: 10, PotencyBonus: 20}, ca), 0.01)

	// Per-art potency rider pools too (the cooldown AA's +15 to Assassinate/Mortal Blade).
	rider := spell.CombatArt{Name: "Y", MinDamage: 800, MaxDamage: 1200, PotencyAdd: 15}
	require.InDelta(t, 1450.0, CAEffectiveDamage(StatBlock{Potency: 10, PotencyBonus: 20}, rider), 0.01)

	// Main stat multiplies on top: mainstat 625 → 51.74% → ×1.5174.
	require.InDelta(t, 1517.4, CAEffectiveDamage(StatBlock{MainStat: 625}, ca), 0.01)

	// Ability mod adds IN FULL — the old 50%-of-adjusted-base cap is disproven.
	// Old rule would cap 738 at 0.5×1200=600; measured tooltips show the full add.
	require.InDelta(t, 1738.0, CAEffectiveDamage(StatBlock{AbilityMod: 738}, ca), 0.01)

	// Full stack, hand-computed: avg 1000 × (1+(57.7+24.6)/100) × (1+0.6406) + 738
	// = 1000 × 1.823 × 1.6406 + 738 = 2990.81 + 738 = 3728.81
	full := StatBlock{Potency: 57.7, PotencyBonus: 24.6, MainStat: 983, AbilityMod: 738}
	require.InDelta(t, 3728.81, CAEffectiveDamage(full, ca), 0.05)
}

func TestEffRecastMeasuredRules(t *testing.T) {
	evis := spell.CombatArt{Name: "Eviscerate V", RecastSecs: 60}
	// Full-strength conversion: 60 × (1 − 3.8/100) = 57.72 (live tooltip: 57.8 @ 3.8 reuse)
	require.InDelta(t, 57.72, effRecast(StatBlock{Reuse: 3.8}, evis), 1e-9)
	// Reuse stat caps at 50 → recast can halve but never beat the ceiling.
	require.InDelta(t, 30.0, effRecast(StatBlock{Reuse: 50}, evis), 1e-9)
	require.InDelta(t, 30.0, effRecast(StatBlock{Reuse: 100}, evis), 1e-9)
}

func TestEffRecastSharedCeiling(t *testing.T) {
	assassinate := spell.CombatArt{Name: "Assassinate II", RecastSecs: 300, RecastReduction: 0.5}
	// AA halving fills the entire 50% ceiling: reuse adds nothing (live: pinned 2m30s).
	require.InDelta(t, 150.0, effRecast(StatBlock{}, assassinate), 1e-9)
	require.InDelta(t, 150.0, effRecast(StatBlock{Reuse: 3.8}, assassinate), 1e-9)
	require.InDelta(t, 150.0, effRecast(StatBlock{Reuse: 50}, assassinate), 1e-9)

	// A partial art mod leaves headroom under the ceiling for reuse.
	partial := spell.CombatArt{Name: "Y", RecastSecs: 100, RecastReduction: 0.3}
	require.InDelta(t, 60.0, effRecast(StatBlock{Reuse: 10}, partial), 1e-9) // 0.3+0.1
	require.InDelta(t, 50.0, effRecast(StatBlock{Reuse: 30}, partial), 1e-9) // 0.3+0.3 → ceiling

	// An art mod alone can exceed the ceiling — still clamped.
	deep := spell.CombatArt{Name: "Z", RecastSecs: 100, RecastReduction: 0.75}
	require.InDelta(t, 50.0, effRecast(StatBlock{}, deep), 1e-9)
}

func TestSlotTiming(t *testing.T) {
	headShot := spell.CombatArt{Name: "Head Shot IV", RecastSecs: 60, CastSecsHundredths: 200}
	// Divisor cast speed (live: 2.0s → 1.46s @ 37.4%), recovery gone at 100.
	require.InDelta(t, 2.0/1.374, slotSecs(StatBlock{CastSpeed: 37.4, RecoverySpeed: 100}, headShot), 1e-9)
	// No timing stats: fallback cast 0.5 + full base recovery 0.5.
	quick := spell.CombatArt{Name: "Q", RecastSecs: 10}
	require.InDelta(t, 1.0, slotSecs(StatBlock{}, quick), 1e-9)
	// Recovery is subtractive: 50% → 0.25s left.
	require.InDelta(t, 0.75, slotSecs(StatBlock{RecoverySpeed: 50}, quick), 1e-9)
}

func TestCADPSSmoothingInvariantForShortRecast(t *testing.T) {
	// A single short-recast art has no big-cast boundary: the window [595,605]
	// straddles only a tiny 10s-boundary step, so the smoothed value stays within
	// 1% of the naive 1000/10=100.0 rate (vs 50%+ swings for a 150s-recast art).
	cas := []spell.CombatArt{{Name: "X", MinDamage: 800, MaxDamage: 1200, RecastSecs: 10}}
	got := CADPS(StatBlock{}, cas, 600)
	require.InDelta(t, 100.0, got, 1.0) // within 1% — no large boundary discontinuity
}

func TestCADPSSmoothsBigCastBoundary(t *testing.T) {
	// One big art on a 150s recast, fight target 150 (right on its 2nd-cast
	// boundary). Raw single-length is a coin-flip on the 2nd cast; smoothed
	// CADPS lands strictly between the 1-cast and 2-cast rates.
	cas := []spell.CombatArt{{Name: "Big", MinDamage: 10000, MaxDamage: 10000, RecastSecs: 150}}
	oneCast := 10000.0 / 150.0 // only the t=0 cast credited
	twoCast := 20000.0 / 150.0 // t=0 and t=150 both credited
	got := CADPS(StatBlock{}, cas, 150)
	require.Greater(t, got, oneCast)
	require.Less(t, got, twoCast)
}

func TestCumCAAt(t *testing.T) {
	starts := []float64{0, 150, 300}
	cum := []float64{100, 200, 300}
	require.InDelta(t, 0.0, cumCAAt(starts, cum, 0), 1e-9)     // nothing before t=0
	require.InDelta(t, 100.0, cumCAAt(starts, cum, 150), 1e-9) // only the t=0 cast (start<150)
	require.InDelta(t, 200.0, cumCAAt(starts, cum, 300), 1e-9)
	require.InDelta(t, 300.0, cumCAAt(starts, cum, 1000), 1e-9)
}

func TestArtCadence_HoldOnTermination(t *testing.T) {
	dot := spell.CombatArt{
		Name: "Bleed Art", RecastSecs: 10, DurationSecs: 24,
		Components: []spell.Component{
			{Kind: spell.DoT, MinDamage: 70, MaxDamage: 117, IntervalSecs: 4, HasInstant: true},
		},
	}
	// No termination → clip → cadence is the plain effRecast (10s, no reuse).
	require.InDelta(t, 10.0, artCadence(StatBlock{}, dot), 1e-9)

	term := dot
	term.Components = append([]spell.Component{{Kind: spell.Termination, MinDamage: 300, MaxDamage: 500}}, dot.Components...)
	// Has termination → hold → cadence = max(effRecast 10, duration 24) = 24.
	require.InDelta(t, 24.0, artCadence(StatBlock{}, term), 1e-9)
	// Reuse can pull effRecast below duration, but a held art still waits for termination.
	require.InDelta(t, 24.0, artCadence(StatBlock{Reuse: 50}, term), 1e-9) // effRecast 5 < 24 → 24
}

func TestDotTicks(t *testing.T) {
	// instant + every 4s over a 24s window → 1 + floor(24/4) = 7.
	c := spell.Component{Kind: spell.DoT, IntervalSecs: 4, HasInstant: true}
	require.InDelta(t, 7.0, dotTicks(c, 24), 1e-9)
	// periodic-only (no instant) over 12s / 4s → 3.
	c2 := spell.Component{Kind: spell.DoT, IntervalSecs: 4, HasInstant: false}
	require.InDelta(t, 3.0, dotTicks(c2, 12), 1e-9)
	// clipped to a 5s window: instant + floor(5/4)=1 → 2.
	require.InDelta(t, 2.0, dotTicks(c, 5), 1e-9)
	// zero interval guards against div-by-zero.
	require.InDelta(t, 0.0, dotTicks(spell.Component{Kind: spell.DoT}, 24), 1e-9)
}

func TestCAEffectiveDamage_PerComponent(t *testing.T) {
	// No-stats StatBlock → scaling = 1, crit = 1, so damage is just the base math.
	sb := StatBlock{AbilityMod: 100}

	// DirectHit gets full abmod.
	dh := spell.CombatArt{Components: []spell.Component{
		{Kind: spell.DirectHit, MinDamage: 800, MaxDamage: 1200},
	}}
	require.InDelta(t, 1100.0, CAEffectiveDamage(sb, dh), 0.01) // avg 1000 + 100 abmod

	// DoT held to full duration: instant + 6 ticks = 7 applications, NO abmod; detonate fires.
	gw := spell.CombatArt{
		RecastSecs: 30, DurationSecs: 24,
		Components: []spell.Component{
			{Kind: spell.DirectHit, MinDamage: 800, MaxDamage: 1200},
			{Kind: spell.DoT, MinDamage: 50, MaxDamage: 50, IntervalSecs: 4, HasInstant: true},
			{Kind: spell.Termination, MinDamage: 300, MaxDamage: 300},
		},
	}
	// 1100 + 50×7 + 300 = 1100 + 350 + 300 = 1750.
	require.InDelta(t, 1750.0, CAEffectiveDamage(sb, gw), 0.01)

	// Clipped DoT (no termination): effRecast 10 < duration 24 → window 10 → instant + floor(10/4)=2 → 3 ticks.
	clip := spell.CombatArt{
		RecastSecs: 10, DurationSecs: 24,
		Components: []spell.Component{
			{Kind: spell.DirectHit, MinDamage: 800, MaxDamage: 1200},
			{Kind: spell.DoT, MinDamage: 50, MaxDamage: 50, IntervalSecs: 4, HasInstant: true},
		},
	}
	// 1100 + 50×3 = 1250 (no detonate — none present, never terminates).
	require.InDelta(t, 1250.0, CAEffectiveDamage(sb, clip), 0.01)

	// TriggerProc: (base + 0.5×abmod) × triggers, no DirectHit.
	dm := spell.CombatArt{
		RecastSecs: 30, DurationSecs: 72,
		Components: []spell.Component{
			{Kind: spell.TriggerProc, MinDamage: 400, MaxDamage: 600, Triggers: 5},
		},
	}
	// (500 + 0.5×100) × 5 = 550 × 5 = 2750.
	require.InDelta(t, 2750.0, CAEffectiveDamage(sb, dm), 0.01)

	// RateProc is not scored.
	rp := spell.CombatArt{Components: []spell.Component{
		{Kind: spell.RateProc, MinDamage: 100, MaxDamage: 100, PerMinute: 3},
	}}
	require.InDelta(t, 0.0, CAEffectiveDamage(sb, rp), 0.01)

	// Backward-compat: no components → legacy single line (full abmod on the one line).
	legacy := spell.CombatArt{MinDamage: 800, MaxDamage: 1200}
	require.InDelta(t, 1100.0, CAEffectiveDamage(sb, legacy), 0.01)
}

func TestRotationHoldsTerminationArt(t *testing.T) {
	// A termination DoT with effRecast 10 but duration 24 must recast on 24 (held),
	// not 10. Over a 48s fight it fires at t=0 and t=24 → exactly 2 casts.
	term := spell.CombatArt{
		Name: "Held", RecastSecs: 10, DurationSecs: 24, CastSecsHundredths: 100,
		Components: []spell.Component{
			{Kind: spell.DirectHit, MinDamage: 1000, MaxDamage: 1000},
			{Kind: spell.Termination, MinDamage: 500, MaxDamage: 500},
		},
	}
	starts, _ := rotationTimeline(StatBlock{RecoverySpeed: 100}, []spell.CombatArt{term}, 48)
	require.Len(t, starts, 2)
	require.InDelta(t, 0.0, starts[0], 1e-9)
	require.InDelta(t, 24.0, starts[1], 1e-9) // held to duration, not 10
}

func TestGushingWoundCalibration(t *testing.T) {
	// Ground-truth naked-recovered bases (spec §3.1). Validates the full per-component
	// sum: DirectHit (full abmod) + DoT (7 applications, no abmod) + detonate (held → fires).
	gw := spell.CombatArt{
		Name: "Gushing Wound VI", RecastSecs: 30, DurationSecs: 24,
		Components: []spell.Component{
			{Kind: spell.Termination, DamageType: "piercing", MinDamage: 326.1, MaxDamage: 543.2, TriggeredSpell: "Untreated Bleeding"},
			{Kind: spell.DirectHit, DamageType: "melee", MinDamage: 49.2, MaxDamage: 82.5},
			{Kind: spell.DoT, DamageType: "piercing", MinDamage: 69.6, MaxDamage: 116.5, IntervalSecs: 4, HasInstant: true},
		},
	}
	// Full-gear state: pool 58.7+24.6, AGI via MainStat 983 (~64%), abmod 694, crit 0.
	sb := StatBlock{Potency: 58.7, PotencyBonus: 24.6, MainStat: 983, AbilityMod: 694}
	scaling := (1 + (58.7+24.6)/100) * (1 + MainStatEffect(983)/100)
	avg := func(a, b float64) float64 { return (a + b) / 2 }

	directHit := avg(49.2, 82.5)*scaling + 694      // full abmod
	dot := avg(69.6, 116.5) * scaling * 7           // instant + 6 ticks, no abmod
	detonate := avg(326.1, 543.2) * scaling         // held → fires, no abmod
	want := directHit + dot + detonate              // crit 0 → ×1

	require.InDelta(t, want, CAEffectiveDamage(sb, gw), 0.01)
	require.InDelta(t, 30.0, artCadence(sb, gw), 1e-9) // max(effRecast 30, duration 24)
}
