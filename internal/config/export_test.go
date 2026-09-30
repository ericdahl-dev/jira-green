package config

import "time"

// SetTokenCommandTimeout shortens the token_command timeout for a test and
// returns a func that restores it.
func SetTokenCommandTimeout(d time.Duration) (restore func()) {
	old := tokenCommandTimeout
	tokenCommandTimeout = d
	return func() { tokenCommandTimeout = old }
}
