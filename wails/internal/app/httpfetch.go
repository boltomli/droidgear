// errString helpers for the remaining GUI-side fetch surface.
package app

import "fmt"

func errString(message string) error { return fmt.Errorf("%s", message) }
