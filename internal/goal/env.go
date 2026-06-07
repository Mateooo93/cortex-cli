// Package goal implements the /goal command autonomous agent loop.
//
// This file wires production dependencies (os.Getenv).
package goal

import "os"

func init() {
	getEnv = os.Getenv
}
