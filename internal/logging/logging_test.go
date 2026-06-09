// Copyright 2026 Blink Labs Software
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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

func TestGetLoggerConcurrent(t *testing.T) {
	// Force the lazy-initialization path that GetLogger takes before Configure
	// has run. Concurrent callers must not race on the global logger.
	globalLogger = nil

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if GetLogger() == nil {
				t.Error("GetLogger returned nil")
			}
		}()
	}
	wg.Wait()
}
