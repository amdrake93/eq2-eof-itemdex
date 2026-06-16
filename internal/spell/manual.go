package spell

// manualArts are level-70 combat arts the census pull misses because they are
// learned below the level-57 floor (census reports their low-level damage, not
// the level-70 effective base). Bases recovered via tooltip calibration
// (attribute-divided → census-equivalent; backlog §9, spec §3.1 "Component
// bases"). Each is a single DirectHit; recast/cast are the census values.
var manualArts = []CombatArt{
	{
		Name: "Hilt Strike", Level: 70, RecastSecs: 20, CastSecsHundredths: 50,
		MinDamage: 262, MaxDamage: 315,
		Components: []Component{{Kind: DirectHit, DamageType: "melee", MinDamage: 262, MaxDamage: 315}},
	},
	{
		Name: "Strike of Consistency", Level: 70, RecastSecs: 12, CastSecsHundredths: 50,
		MinDamage: 199, MaxDamage: 199,
		Components: []Component{{Kind: DirectHit, DamageType: "melee", MinDamage: 199, MaxDamage: 199}},
	},
}

// ManualArts returns a deep copy of the recovered low-level-learned arts to
// append after the census pull (backlog §9). The copy keeps callers from
// mutating the package-level constants.
func ManualArts() []CombatArt {
	out := make([]CombatArt, len(manualArts))
	for i, a := range manualArts {
		a.Components = append([]Component(nil), a.Components...)
		out[i] = a
	}
	return out
}
