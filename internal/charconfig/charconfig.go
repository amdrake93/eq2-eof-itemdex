// Package charconfig loads per-character TOML config (docs/design-plan2.md §4):
// AA/innate stats, per-art AA modifiers, and buff-package contexts. Server-wide
// combat mechanics stay in internal/constants — config is everything about one
// player and their group. Validation is strict: unknown keys are errors, so a
// typo'd stat cannot silently vanish.
package charconfig

import (
	"fmt"

	"github.com/BurntSushi/toml"
	"github.com/amdrake93/eq2-eof-itemdex/internal/model"
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
	DPSMod        float64 `toml:"dpsmod"`
	Reuse         float64 `toml:"reuse"`
	Flurry        float64 `toml:"flurry"`
	AbilityMod    float64 `toml:"abilitymod"`
	CastSpeed     float64 `toml:"cast_speed"`
	RecoverySpeed float64 `toml:"recovery_speed"`
}

// Block converts the grants to a model.StatBlock.
func (g StatGrants) Block() model.StatBlock {
	return model.StatBlock{
		Haste:         g.Haste,
		MultiAttack:   g.MultiAttack,
		CritChance:    g.CritChance,
		Potency:       g.Potency,
		DPSMod:        g.DPSMod,
		Reuse:         g.Reuse,
		Flurry:        g.Flurry,
		AbilityMod:    g.AbilityMod,
		CastSpeed:     g.CastSpeed,
		RecoverySpeed: g.RecoverySpeed,
	}
}

// ArtMod is a per-art AA effect ([art_mods."Name"]). RecastMult multiplies the
// art's base recast (0.5 = the AA halving); it counts against the art's shared
// 50% recast-reduction ceiling.
type ArtMod struct {
	RecastMult float64 `toml:"recast_mult"`
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
	for name, m := range cfg.ArtMods {
		if m.RecastMult <= 0 || m.RecastMult > 1 {
			return Config{}, fmt.Errorf("%s: art_mods[%q]: recast_mult %v out of range (0,1]", path, name, m.RecastMult)
		}
	}
	if len(cfg.Contexts) == 0 {
		return Config{}, fmt.Errorf("%s: config must define at least one context", path)
	}
	return cfg, nil
}
