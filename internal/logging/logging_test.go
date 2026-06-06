package logging

import (
	"sync"
	"testing"
)

// TestConcurrentGetLoggerAndConfigureRaceFree hammers the global logger from
// many goroutines at once, mixing lazy initialization (GetLogger) with explicit
// reconfiguration (Configure). Run with -race, it fails if access to the global
// logger is unsynchronized.
func TestConcurrentGetLoggerAndConfigureRaceFree(t *testing.T) {
	// Force the lazy-initialization path so first-use is exercised too. This is
	// sequenced before the goroutines below, so it does not itself race.
	globalLogger = nil

	const goroutines = 64
	var wg sync.WaitGroup
	wg.Add(goroutines * 2)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			if GetLogger() == nil {
				t.Error("GetLogger returned nil")
			}
		}()
		go func() {
			defer wg.Done()
			Configure()
		}()
	}
	wg.Wait()
}
