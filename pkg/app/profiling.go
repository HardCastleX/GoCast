//go:build !pprof

package app

// startProfiling is a no-op in normal builds. Compile with the "pprof" build
// tag to enable CPU/HTTP profiling.
func startProfiling() func() {
	return func() {}
}
