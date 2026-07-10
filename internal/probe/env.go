package probe

import "os"

// envEnabled reports whether an environment variable is set to a truthy
// value. An unset OR empty value ("", "0", "false", "off") counts as
// disabled, so passing the variable through empty (as compose does when
// its .env entry is removed) reliably turns a feature off.
// Uninterpolated ${...} compose placeholders are also treated as disabled.
func envEnabled(key string) bool {
	value := os.Getenv(key)
	// Treat uninterpolated ${...} compose placeholders as unset
	if len(value) > 0 && value[0] == '$' && len(value) > 1 && value[1] == '{' {
		return false
	}
	switch value {
	case "", "0", "false", "off", "no":
		return false
	default:
		return true
	}
}
