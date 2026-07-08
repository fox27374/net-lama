package probe

import "os"

// envEnabled reports whether an environment variable is set to a truthy
// value. An unset OR empty value ("", "0", "false", "off") counts as
// disabled, so passing the variable through empty (as compose does when
// its .env entry is removed) reliably turns a feature off.
func envEnabled(key string) bool {
	switch os.Getenv(key) {
	case "", "0", "false", "off", "no":
		return false
	default:
		return true
	}
}
