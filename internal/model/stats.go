package model

// StatBlock holds the DPS-relevant, gear-variable combat stats the model uses.
// Values are in the units Census reports (percent/points).
type StatBlock struct {
	Haste         float64 // attackspeed
	MultiAttack   float64 // doubleattackchance (legacy key; = Multi-Attack)
	CritChance    float64 // critchance
	Potency       float64 // basemodifier
	DPSMod        float64 // dps
	Reuse         float64 // spelltimereusepct
	Flurry        float64 // flurry
	AbilityMod    float64 // all (displayname "All") = Ability Modifier
	CastSpeed     float64 // spelltimecastpct — divides CA cast times (gear + AAs)
	RecoverySpeed float64 // AA-only (no EoF gear carries it) — shrinks the 0.5s post-cast recovery
	MainStat      float64 // strength key = "+N primary attributes" → AGI for a scout; multiplies CA damage via its curve
	PotencyBonus  float64 // calibrated hidden potency-pool points (config-only; ⚠ spec §12 open mystery)
	CritBonus     float64 // buff/gear bonus added to the 1.50 base crit factor (percent points; 0 today, raid-context future — §16)
	HasteEffect   float64 // non-stacking named "Haste" item effect — max-wins, NOT summed (spec §11)
}

// modifierToField maps a Census modifier key to the StatBlock field it feeds.
// Unlisted keys (critbonus, resists, mitigation, attributes, …) are ignored:
// critbonus is stripped on the server; resists/hp are defensive; attributes
// don't discriminate and are held constant.
var modifierToField = map[string]func(*StatBlock, float64){
	"attackspeed":        func(s *StatBlock, v float64) { s.Haste += v },
	"doubleattackchance": func(s *StatBlock, v float64) { s.MultiAttack += v },
	"critchance":         func(s *StatBlock, v float64) { s.CritChance += v },
	"basemodifier":       func(s *StatBlock, v float64) { s.Potency += v },
	"dps":                func(s *StatBlock, v float64) { s.DPSMod += v },
	"spelltimereusepct":  func(s *StatBlock, v float64) { s.Reuse += v },
	"flurry":             func(s *StatBlock, v float64) { s.Flurry += v },
	"all":                func(s *StatBlock, v float64) { s.AbilityMod += v }, // displayname "All" = Ability Modifier
	"spelltimecastpct":   func(s *StatBlock, v float64) { s.CastSpeed += v },
	"strength":           func(s *StatBlock, v float64) { s.MainStat += v }, // "+N primary attributes" (explicit agility/wisdom/intelligence keys excluded — data-suspicious, spec §11)
}

// AddModifiers folds a set of Census modifiers into the StatBlock (additive).
func (s *StatBlock) AddModifiers(mods map[string]float64) {
	for k, v := range mods {
		if apply, ok := modifierToField[k]; ok {
			apply(s, v)
		}
	}
}

// Add returns the sum of two StatBlocks (baseline + an item's stats).
func (s StatBlock) Add(o StatBlock) StatBlock {
	return StatBlock{
		Haste:         s.Haste + o.Haste,
		MultiAttack:   s.MultiAttack + o.MultiAttack,
		CritChance:    s.CritChance + o.CritChance,
		Potency:       s.Potency + o.Potency,
		DPSMod:        s.DPSMod + o.DPSMod,
		Reuse:         s.Reuse + o.Reuse,
		Flurry:        s.Flurry + o.Flurry,
		AbilityMod:    s.AbilityMod + o.AbilityMod,
		CastSpeed:     s.CastSpeed + o.CastSpeed,
		RecoverySpeed: s.RecoverySpeed + o.RecoverySpeed,
		MainStat:      s.MainStat + o.MainStat,
		PotencyBonus:  s.PotencyBonus + o.PotencyBonus,
		CritBonus:     s.CritBonus + o.CritBonus,
		HasteEffect:   max(s.HasteEffect, o.HasteEffect),
	}
}

// EffectiveHaste is the total haste used by the model: stackable haste (AA +
// modifier-block, in Haste) plus the non-stacking "Haste" item effect (HasteEffect,
// already resolved to the max across a set by Add). See spec §11.
func (s StatBlock) EffectiveHaste() float64 { return s.Haste + s.HasteEffect }
