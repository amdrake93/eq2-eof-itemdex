package store

import (
	"database/sql"
	"sort"
	"strings"

	"github.com/amdrake93/eq2-eof-itemdex/internal/catalog"
	"github.com/amdrake93/eq2-eof-itemdex/internal/census"
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
  PRIMARY KEY (item_id, stat)
);
CREATE TABLE IF NOT EXISTS combat_arts (
  name TEXT PRIMARY KEY, level INTEGER, min_dmg REAL, max_dmg REAL,
  recast_secs REAL, cast_secs_hundredths INTEGER
);`

// Init creates the schema tables if they do not already exist.
func (d *DB) Init() error {
	_, err := d.db.Exec(schema)
	return err
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
			`INSERT OR REPLACE INTO combat_arts (name,level,min_dmg,max_dmg,recast_secs,cast_secs_hundredths)
			 VALUES (?,?,?,?,?,?)`,
			a.Name, a.Level, a.MinDamage, a.MaxDamage, a.RecastSecs, a.CastSecsHundredths,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
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
				`INSERT OR REPLACE INTO item_stats (item_id, stat, value) VALUES (?, ?, ?)`,
				it.ID, stat, m.Value,
			); err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}
