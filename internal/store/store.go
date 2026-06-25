package store

import (
	"database/sql"
	"sort"
	"strings"

	"github.com/amdrake93/eq2-eof-itemdex/internal/catalog"
	"github.com/amdrake93/eq2-eof-itemdex/internal/census"
	"github.com/amdrake93/eq2-eof-itemdex/internal/model"
	"github.com/amdrake93/eq2-eof-itemdex/internal/spell"
	_ "modernc.org/sqlite" // pure-Go driver, registers "sqlite"
)

// DB wraps a sql.DB connection to the SQLite analysis database.
type DB struct{ db *sql.DB }

// Open opens (or creates) a SQLite database at path. Use ":memory:" for tests.
func Open(path string) (*DB, error) {
	d, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	return &DB{db: d}, nil
}

// SQL returns the underlying *sql.DB for direct query access.
func (d *DB) SQL() *sql.DB { return d.db }

// Close closes the underlying database connection.
func (d *DB) Close() error { return d.db.Close() }

const schema = `
CREATE TABLE IF NOT EXISTS items (
  id INTEGER PRIMARY KEY, name TEXT, slot TEXT, tier TEXT, itemlevel INTEGER,
  armor_type TEXT, skill TEXT, wieldstyle TEXT, classes TEXT, gamelink TEXT,
  weapon_min_dmg REAL, weapon_max_dmg REAL, delay REAL, damage_rating REAL
);
CREATE TABLE IF NOT EXISTS item_stats (
  item_id INTEGER, stat TEXT, value REAL,
  source TEXT NOT NULL DEFAULT 'modifier',
  PRIMARY KEY (item_id, stat, source)
);
CREATE TABLE IF NOT EXISTS item_procs (
  item_id INTEGER, trigger TEXT, per_minute REAL,
  dmg_type TEXT, min_dmg REAL, max_dmg REAL, raw TEXT
);
CREATE TABLE IF NOT EXISTS combat_arts (
  name TEXT PRIMARY KEY, level INTEGER, min_dmg REAL, max_dmg REAL,
  recast_secs REAL, cast_secs_hundredths INTEGER, duration_secs REAL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS combat_art_components (
  art_name TEXT, idx INTEGER, kind INTEGER, dmg_type TEXT,
  min_dmg REAL, max_dmg REAL, interval_secs REAL, has_instant INTEGER,
  aoe INTEGER, triggered_spell TEXT, triggers INTEGER, per_minute REAL,
  PRIMARY KEY (art_name, idx)
);
CREATE TABLE IF NOT EXISTS scores (
  item_id INTEGER, baseline TEXT, dps_score REAL, slot TEXT,
  PRIMARY KEY (item_id, baseline)
);`

// Init creates the schema tables if they do not already exist.
func (d *DB) Init() error {
	_, err := d.db.Exec(schema)
	return err
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func firstSlot(it census.Item) string {
	if len(it.Slots) > 0 {
		return it.Slots[0].Name
	}
	return ""
}

func classList(it census.Item) string {
	names := make([]string, 0, len(it.TypeInfo.Classes))
	for k := range it.TypeInfo.Classes {
		names = append(names, k)
	}
	sort.Strings(names)
	return strings.Join(names, "|")
}

// LoadCombatArts inserts the Assassin's combat arts.
func (d *DB) LoadCombatArts(arts []spell.CombatArt) (err error) {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	for _, a := range arts {
		if _, err = tx.Exec(
			`INSERT OR REPLACE INTO combat_arts (name,level,min_dmg,max_dmg,recast_secs,cast_secs_hundredths,duration_secs)
			 VALUES (?,?,?,?,?,?,?)`,
			a.Name, a.Level, a.MinDamage, a.MaxDamage, a.RecastSecs, a.CastSecsHundredths, a.DurationSecs,
		); err != nil {
			return err
		}
		if _, err = tx.Exec(`DELETE FROM combat_art_components WHERE art_name = ?`, a.Name); err != nil {
			return err
		}
		for i, c := range a.Components {
			if _, err = tx.Exec(
				`INSERT INTO combat_art_components
				 (art_name,idx,kind,dmg_type,min_dmg,max_dmg,interval_secs,has_instant,aoe,triggered_spell,triggers,per_minute)
				 VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
				a.Name, i, int(c.Kind), c.DamageType, c.MinDamage, c.MaxDamage,
				c.IntervalSecs, boolToInt(c.HasInstant), boolToInt(c.AoE), c.TriggeredSpell, c.Triggers, c.PerMinute,
			); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

// CombatArts loads every combat art with its parsed damage components attached.
func (d *DB) CombatArts() ([]spell.CombatArt, error) {
	rows, err := d.db.Query(`SELECT name, level, min_dmg, max_dmg, recast_secs, cast_secs_hundredths, duration_secs FROM combat_arts`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var arts []spell.CombatArt
	for rows.Next() {
		var a spell.CombatArt
		if err := rows.Scan(&a.Name, &a.Level, &a.MinDamage, &a.MaxDamage, &a.RecastSecs, &a.CastSecsHundredths, &a.DurationSecs); err != nil {
			return nil, err
		}
		arts = append(arts, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	comps, err := d.loadComponents()
	if err != nil {
		return nil, err
	}
	for i := range arts {
		arts[i].Components = comps[arts[i].Name]
	}
	return arts, nil
}

func (d *DB) loadComponents() (map[string][]spell.Component, error) {
	rows, err := d.db.Query(
		`SELECT art_name, kind, dmg_type, min_dmg, max_dmg, interval_secs, has_instant, aoe, triggered_spell, triggers, per_minute
		 FROM combat_art_components ORDER BY art_name, idx`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[string][]spell.Component{}
	for rows.Next() {
		var (
			name       string
			kind       int
			hasInstant int
			aoe        int
			c          spell.Component
		)
		if err := rows.Scan(&name, &kind, &c.DamageType, &c.MinDamage, &c.MaxDamage,
			&c.IntervalSecs, &hasInstant, &aoe, &c.TriggeredSpell, &c.Triggers, &c.PerMinute); err != nil {
			return nil, err
		}
		c.Kind = spell.ComponentKind(kind)
		c.HasInstant = hasInstant != 0
		c.AoE = aoe != 0
		out[name] = append(out[name], c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// ScoreRow is one item's in-context ΔDPS under one baseline.
type ScoreRow struct {
	ItemID   int
	Baseline string
	DPSScore float64
	Slot     string
}

// WriteScores upserts score rows in a single transaction.
func (d *DB) WriteScores(rows []ScoreRow) (err error) {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	for _, r := range rows {
		if _, err = tx.Exec(
			`INSERT OR REPLACE INTO scores (item_id, baseline, dps_score, slot) VALUES (?, ?, ?, ?)`,
			r.ItemID, r.Baseline, r.DPSScore, r.Slot,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Loadout is the fixed main-hand + collapsed combat arts the model scores against.
// The off-hand is chosen from the Secondary candidate pool, not fixed here.
type Loadout struct {
	Main     model.Weapon
	MainName string
	Arts     []spell.CombatArt
}

func (d *DB) loadWeapon(query string, args ...any) (model.Weapon, string, error) {
	var name string
	var mn, mx, delay float64
	if err := d.db.QueryRow(query, args...).Scan(&name, &mn, &mx, &delay); err != nil {
		return model.Weapon{}, "", err
	}
	return model.Weapon{AvgDamage: (mn + mx) / 2, MinDamage: mn, MaxDamage: mx, DelaySecs: delay}, name, nil
}

// LoadLoadout reads the Soulfire main-hand and the Assassin combat arts collapsed
// to highest rank.
func (d *DB) LoadLoadout() (Loadout, error) {
	main, mainName, err := d.loadWeapon(
		`SELECT name, weapon_min_dmg, weapon_max_dmg, delay FROM items
		 WHERE name LIKE 'Soulfire%' AND classes LIKE '%assassin%'
		 ORDER BY (name = 'Soulfire Sabre') DESC, weapon_max_dmg DESC LIMIT 1`)
	if err != nil {
		return Loadout{}, err
	}
	arts, err := d.CombatArts()
	if err != nil {
		return Loadout{}, err
	}
	return Loadout{Main: main, MainName: mainName, Arts: spell.HighestRanks(arts)}, nil
}

// ScorableItem is one Assassin-usable item with its DPS-relevant stats resolved
// into a StatBlock plus weapon damage fields (0 for non-weapons).
type ScorableItem struct {
	ID          int
	Name        string
	Slot        string
	Tier        string
	WieldStyle  string
	GameLink    string
	WeaponAvg   float64
	WeaponMin   float64
	WeaponMax   float64
	WeaponDelay float64
	Stats       model.StatBlock
	Mods        map[string]float64
}

// IsWeapon reports whether the item swings as a weapon (has an attack delay).
func (it ScorableItem) IsWeapon() bool { return it.WeaponDelay > 0 }

// LoadScorableItems loads every Assassin-usable item with its modifier stats.
func (d *DB) LoadScorableItems() ([]ScorableItem, error) {
	rows, err := d.db.Query(
		`SELECT id, name, slot, tier, wieldstyle, gamelink, weapon_min_dmg, weapon_max_dmg, delay
		 FROM items WHERE classes LIKE '%assassin%'`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var items []ScorableItem
	for rows.Next() {
		var it ScorableItem
		var mn, mx, delay float64
		if err := rows.Scan(&it.ID, &it.Name, &it.Slot, &it.Tier, &it.WieldStyle, &it.GameLink, &mn, &mx, &delay); err != nil {
			return nil, err
		}
		if delay > 0 {
			it.WeaponAvg = (mn + mx) / 2
			it.WeaponMin = mn
			it.WeaponMax = mx
			it.WeaponDelay = delay
		}
		it.Mods = map[string]float64{}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range items {
		sr, err := d.db.Query(`SELECT stat, value, source FROM item_stats WHERE item_id = ?`, items[i].ID)
		if err != nil {
			return nil, err
		}
		for sr.Next() {
			var stat, source string
			var val float64
			if err := sr.Scan(&stat, &val, &source); err != nil {
				_ = sr.Close()
				return nil, err
			}
			if stat == "attackspeed" && source == "effect" {
				if val > items[i].Stats.HasteEffect {
					items[i].Stats.HasteEffect = val
				}
				continue
			}
			items[i].Mods[stat] += val
		}
		if err := sr.Err(); err != nil {
			_ = sr.Close()
			return nil, err
		}
		_ = sr.Close()
		items[i].Stats.AddModifiers(items[i].Mods)
	}
	return items, nil
}

// LoadGear inserts items and their modifier stats in a single transaction.
// armor_type is the derived label (Cloth/Leather/Chain/Plate) from catalog.ArmorType.
func (d *DB) LoadGear(items []census.Item) (err error) {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	for _, it := range items {
		if _, err = tx.Exec(
			`INSERT OR REPLACE INTO items
			 (id, name, slot, tier, itemlevel, armor_type, skill, wieldstyle, classes, gamelink,
			  weapon_min_dmg, weapon_max_dmg, delay, damage_rating)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			it.ID, string(it.DisplayName), firstSlot(it), it.Tier, it.ItemLevel,
			catalog.ArmorType(it.TypeInfo.SkillType), it.TypeInfo.Skill, it.TypeInfo.WieldStyle, classList(it), it.GameLink,
			it.TypeInfo.MinBaseDamage, it.TypeInfo.MaxBaseDamage, it.TypeInfo.Delay, it.TypeInfo.DamageRating,
		); err != nil {
			return err
		}

		for stat, m := range it.Modifiers {
			if _, err = tx.Exec(
				`INSERT OR REPLACE INTO item_stats (item_id, stat, value, source) VALUES (?, ?, ?, 'modifier')`,
				it.ID, stat, m.Value,
			); err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}

// LoadItemEffects inserts effect-derived stats tagged source='effect' so they
// coexist with (and sum alongside) the modifier rows for the same item+stat.
func (d *DB) LoadItemEffects(stats []catalog.EffectStat) (err error) {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	for _, s := range stats {
		if _, err = tx.Exec(
			`INSERT OR REPLACE INTO item_stats (item_id, stat, value, source) VALUES (?, ?, ?, 'effect')`,
			s.ItemID, s.Stat, s.Value,
		); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// LoadItemProcs inserts triggered item procs into the item_procs catalog.
func (d *DB) LoadItemProcs(procs []catalog.ItemProc) (err error) {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	for _, p := range procs {
		if _, err = tx.Exec(
			`INSERT INTO item_procs (item_id, trigger, per_minute, dmg_type, min_dmg, max_dmg, raw)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			p.ItemID, p.Trigger, p.PerMinute, p.DmgType, p.MinDmg, p.MaxDmg, p.Raw,
		); err != nil {
			return err
		}
	}

	return tx.Commit()
}
