//go:build e2e

package e2e

import "runtime"

// goroutineCount reports the current goroutine count, used to detect leaks after
// aborted requests.
func goroutineCount() int { return runtime.NumGoroutine() }
