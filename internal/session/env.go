package session

import "os"

// osGetenv is a thin wrapper around os.Getenv to make it easy to swap
// in tests.
func osGetenv(k string) string { return os.Getenv(k) }
