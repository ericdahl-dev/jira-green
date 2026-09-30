package ui

import (
	"reflect"
	"time"
)

// SetFlashFor sets how long a flash stays, returning the old value.
func SetFlashFor(d time.Duration) time.Duration { old := flashFor; flashFor = d; return old }

// UsesOpenBrowser reports whether d opens cards with OpenBrowser.
func UsesOpenBrowser(d Dashboard) bool {
	return reflect.ValueOf(d.openURL).Pointer() == reflect.ValueOf(OpenBrowser).Pointer()
}
