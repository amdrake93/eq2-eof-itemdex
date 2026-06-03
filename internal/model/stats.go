package model

// StatBlock holds the DPS-relevant, gear-variable combat stats the model uses.
// Values are in the units Census reports (percent/points).
type StatBlock struct {
	Haste       float64 // attackspeed
	MultiAttack float64 // doubleattackchance (legacy key; = Multi-Attack)
	CritChance  float64 // critchance
	Potency     float64 // basemodifier
	DPSMod      float64 // dps
	Reuse       float64 // spelltimereusepct
	Flurry      float64 // flurry
	AbilityMod  float64 // ability-mod / +combat-art damage (not an itemized stat in EoF dataset; supported as baseline input)
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
	"abilitymod":         func(s *StatBlock, v float64) { s.AbilityMod += v },
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
		Haste:       s.Haste + o.Haste,
		MultiAttack: s.MultiAttack + o.MultiAttack,
		CritChance:  s.CritChance + o.CritChance,
		Potency:     s.Potency + o.Potency,
		DPSMod:      s.DPSMod + o.DPSMod,
		Reuse:       s.Reuse + o.Reuse,
		Flurry:      s.Flurry + o.Flurry,
		AbilityMod:  s.AbilityMod + o.AbilityMod,
	}
}
