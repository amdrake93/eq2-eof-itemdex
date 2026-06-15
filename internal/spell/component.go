package spell

// ComponentKind classifies how a damage component is delivered.
type ComponentKind int

const (
	DirectHit   ComponentKind = iota // single instant hit
	DoT                              // damage over time (periodic, optionally with an instant tick)
	Termination                      // fires once when a DoT/effect duration expires
	TriggerProc                      // cast on an event a fixed number of times
	RateProc                         // cast on an event at an approximate rate per minute
)

// Component is one parsed damage line of an ability. An ability's total damage
// is the sum over its components (each simmed with the per-component equation in
// Increment B). Fields beyond the kind's own are zero-valued.
type Component struct {
	Kind           ComponentKind
	DamageType     string // melee, piercing, ranged, slashing, poison, ...
	MinDamage      float64
	MaxDamage      float64
	IntervalSecs   float64 // DoT (or proc-cast DoT): tick interval in seconds
	HasInstant     bool    // DoT: "instantly and every" (true) vs bare "every" (false)
	AoE            bool    // "targets in Area of Effect"
	TriggeredSpell string  // Termination/Proc: name of the applied/cast spell
	Triggers       int     // TriggerProc: total trigger count
	PerMinute      float64 // RateProc: approximate procs per minute
}
