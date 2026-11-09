package lid

// AdapterState is the documented IOPS adapter signal. A NULL result means
// "not attached" or "lookup failed"; IOPowerSources.h gives both the same
// value, and both must keep the low-battery protection enabled.
type AdapterState int

const (
	// AdapterUnknown means the symbol could not be bound or the call is
	// unavailable. It is never treated as "plugged in".
	AdapterUnknown AdapterState = iota
	AdapterAttached
	AdapterAbsent
)

// PowerSample is one read of the power sources. Percent is -1 when the
// capacity cannot be trusted.
type PowerSample struct {
	Kind    string // "ac" | "battery" | "unknown"
	Percent int
	Adapter AdapterState
}

// Plugged is the single v1 rule: AC power or an adapter description counts as
// plugged in. Everything else (NULL details, binding failures, unknown kind)
// keeps the conservative "not plugged in" behaviour.
func Plugged(s PowerSample) bool {
	return s.Kind == "ac" || s.Adapter == AdapterAttached
}

// AdapterLabel renders the diagnostic field of a response and of the CLI
// snapshot. Both sides use the same three values.
func AdapterLabel(s AdapterState) string {
	switch s {
	case AdapterAttached:
		return "yes"
	case AdapterAbsent:
		return "no"
	default:
		return "unknown"
	}
}
