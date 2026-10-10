//go:build pprof

package app

import (
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"runtime/pprof"
)

// startProfiling enables profiling for binaries built with the "pprof" tag:
//
//	go build -tags pprof -ldflags "-H windowsgui" -o GoCast.exe .
//
//	GOCAST_PPROF_ADDR=:6060        serve net/http/pprof
//	                               (go tool pprof http://localhost:6060/debug/pprof/profile)
//	GOCAST_CPU_PROFILE=cpu.pprof   write a CPU profile to a file on exit
//
// The returned function stops profiling.
func startProfiling() func() {
	var stop func()

	if addr := os.Getenv("GOCAST_PPROF_ADDR"); addr != "" {
		go func() {
			log.Printf("GoCast: pprof listening on %s", addr)
			_ = http.ListenAndServe(addr, nil)
		}()
	}

	if path := os.Getenv("GOCAST_CPU_PROFILE"); path != "" {
		f, err := os.Create(path)
		if err != nil {
			log.Printf("GoCast: cannot create CPU profile %q: %v", path, err)
		} else if err := pprof.StartCPUProfile(f); err != nil {
			log.Printf("GoCast: cannot start CPU profile: %v", err)
			_ = f.Close()
		} else {
			log.Printf("GoCast: writing CPU profile to %s", path)
			stop = func() {
				pprof.StopCPUProfile()
				_ = f.Close()
			}
		}
	}

	if stop == nil {
		return func() {}
	}
	return stop
}
