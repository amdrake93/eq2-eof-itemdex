// Package charconfig loads per-character TOML config (docs/SPEC.md §6):
// AA/innate stats, per-art AA modifiers, and buff-package contexts. Server-wide
// combat mechanics stay in internal/constants — config is everything about one
// player and their group. Validation is strict: unknown keys are errors, so a
// typo'd stat cannot silently vanish.
package charconfig

import (
	"fmt"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/amdrake93/eq2-eof-itemdex/internal/model"
	"github.com/amdrake93/eq2-eof-itemdex/internal/spell"
)

type Character struct {
	Name    string `toml:"name"`
	Class   string `toml:"class"`
	ArtTier string `toml:"art_tier"`
}

// StatGrants is a stat block as written in TOML ([stats] or one [contexts.X]).
type StatGrants struct {
	Haste         float64 `toml:"haste"`
	MultiAttack   float64 `toml:"multiattack"`
	CritChance    float64 `toml:"critchance"`
	Potency       float64 `toml:"potency"`
	PotencyBonus  float64 `toml:"potency_bonus"`
	DPSMod        float64 `toml:"dpsmod"`
	Reuse         float64 `toml:"reuse"`
	Flurry        float64 `toml:"flurry"`
	AbilityMod    float64 `toml:"abilitymod"`
	CastSpeed     float64 `toml:"cast_speed"`
	RecoverySpeed float64 `toml:"recovery_speed"`
	MainStat      float64 `toml:"mainstat"`
}

// nonNegative reports the first negative field, if any. Config stats are
// grants — debuffs are not a config concept, and a negative cast_speed at or
// below -100 would turn the rotation's divisor non-positive.
func (g StatGrants) nonNegative() error {
	fields := map[string]float64{
		"haste": g.Haste, "multiattack": g.MultiAttack, "critchance": g.CritChance,
		"potency": g.Potency, "potency_bonus": g.PotencyBonus, "dpsmod": g.DPSMod,
		"reuse": g.Reuse, "flurry": g.Flurry, "abilitymod": g.AbilityMod,
		"cast_speed": g.CastSpeed, "recovery_speed": g.RecoverySpeed,
		"mainstat": g.MainStat,
	}
	for name, v := range fields {
		if v < 0 {
			return fmt.Errorf("stat %q is negative (%v) — config stats are grants, not debuffs", name, v)
		}
	}
	return nil
}

// Block converts the grants to a model.StatBlock.
func (g StatGrants) Block() model.StatBlock {
	return model.StatBlock{
		Haste:         g.Haste,
		MultiAttack:   g.MultiAttack,
		CritChance:    g.CritChance,
		Potency:       g.Potency,
		PotencyBonus:  g.PotencyBonus,
		DPSMod:        g.DPSMod,
		Reuse:         g.Reuse,
		Flurry:        g.Flurry,
		AbilityMod:    g.AbilityMod,
		CastSpeed:     g.CastSpeed,
		RecoverySpeed: g.RecoverySpeed,
		MainStat:      g.MainStat,
	}
}

// ArtMod is a per-art AA effect ([art_mods."Name"]). RecastMult multiplies the
// art's base recast (0.5 = the AA halving); it counts against the art's shared
// 50% recast-reduction ceiling. PotencyAdd is an additive potency rider pooled
// with the character's displayed potency (e.g. the cooldown AA's +15).
type ArtMod struct {
	RecastMult float64 `toml:"recast_mult"`
	PotencyAdd float64 `toml:"potency_add"`
}

type Config struct {
	Character Character             `toml:"character"`
	Stats     StatGrants            `toml:"stats"`
	ArtMods   map[string]ArtMod     `toml:"art_mods"`
	Contexts  map[string]StatGrants `toml:"contexts"`
}

// ContextBlock is the model input for one context: [stats] + that context's
// buff package (gear is added by the caller/optimizer).
func (c Config) ContextBlock(name string) (model.StatBlock, error) {
	ctx, ok := c.Contexts[name]
	if !ok {
		return model.StatBlock{}, fmt.Errorf("context %q not found in character config", name)
	}
	return c.Stats.Block().Add(ctx.Block()), nil
}

// Load parses and validates a character config file.
func Load(path string) (Config, error) {
	var cfg Config
	md, err := toml.DecodeFile(path, &cfg)
	if err != nil {
		return Config{}, err
	}
	if undec := md.Undecoded(); len(undec) > 0 {
		return Config{}, fmt.Errorf("%s: unknown config keys: %v", path, undec)
	}
	if cfg.Character.Class != "assassin" {
		return Config{}, fmt.Errorf("%s: unsupported class %q (only assassin is implemented)", path, cfg.Character.Class)
	}
	if cfg.Character.ArtTier != "expert" {
		return Config{}, fmt.Errorf("%s: unsupported art_tier %q (only expert is implemented)", path, cfg.Character.ArtTier)
	}
	if err := cfg.Stats.nonNegative(); err != nil {
		return Config{}, fmt.Errorf("%s: [stats]: %w", path, err)
	}
	for ctxName, ctx := range cfg.Contexts {
		if err := ctx.nonNegative(); err != nil {
			return Config{}, fmt.Errorf("%s: [contexts.%s]: %w", path, ctxName, err)
		}
	}
	for name, m := range cfg.ArtMods {
		if m.RecastMult <= 0 || m.RecastMult > 1 {
			return Config{}, fmt.Errorf("%s: art_mods[%q]: recast_mult %v out of range (0,1]", path, name, m.RecastMult)
		}
		if m.PotencyAdd < 0 {
			return Config{}, fmt.Errorf("%s: art_mods[%q]: potency_add %v is negative", path, name, m.PotencyAdd)
		}
	}
	if len(cfg.Contexts) == 0 {
		return Config{}, fmt.Errorf("%s: config must define at least one context", path)
	}
	return cfg, nil
}

// ClassData holds class-intrinsic constants (docs/SPEC.md §6): values
// identical for every character of a class but differing between classes.
// Uniform schema — every classes/<class>.toml defines the same fields.
type ClassData struct {
	AutoAttackMultiplier float64 `toml:"auto_attack_multiplier"`
}

// LoadClass reads classes/<class>.toml. Strict: unknown keys and a missing or
// non-positive auto_attack_multiplier are errors — the sim is incomplete
// without these class constants.
func LoadClass(dir, class string) (ClassData, error) {
	path := filepath.Join(dir, class+".toml")
	var cd ClassData
	md, err := toml.DecodeFile(path, &cd)
	if err != nil {
		return ClassData{}, err
	}
	if undec := md.Undecoded(); len(undec) > 0 {
		return ClassData{}, fmt.Errorf("%s: unknown class keys: %v", path, undec)
	}
	if cd.AutoAttackMultiplier <= 0 {
		return ClassData{}, fmt.Errorf("%s: auto_attack_multiplier must be > 0 (got %v)", path, cd.AutoAttackMultiplier)
	}
	return cd, nil
}

// ApplyArtMods returns a copy of the art pool with each config art mod applied
// to the art whose base name (rank-stripped) matches. Every mod must match an
// art — a typo'd name failing loudly beats silently un-halving Assassinate.
func ApplyArtMods(arts []spell.CombatArt, mods map[string]ArtMod) ([]spell.CombatArt, error) {
	out := make([]spell.CombatArt, len(arts))
	copy(out, arts)

	matched := make(map[string]bool, len(mods))
	for i := range out {
		if m, ok := mods[spell.BaseName(out[i].Name)]; ok {
			out[i].RecastReduction = 1 - m.RecastMult
			out[i].PotencyAdd = m.PotencyAdd
			matched[spell.BaseName(out[i].Name)] = true
		}
	}

	for name := range mods {
		if !matched[name] {
			return nil, fmt.Errorf("art_mods[%q] matches no loaded combat art", name)
		}
	}
	return out, nil
}
