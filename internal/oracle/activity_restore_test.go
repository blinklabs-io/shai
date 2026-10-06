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

package oracle

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOracleLoadsPersistedPoolActivity(t *testing.T) {
	storage := newTestOracleStorage(t)
	swap := activityStorageSwap(100, "swap", 100, 180)
	require.NoError(t, storage.SavePoolStateAndActivity(
		activityStoragePool(100),
		&swap,
		defaultPoolActivityWindowSlots,
	))
	activity, err := NewActivityTracker(defaultPoolActivityWindowSlots)
	require.NoError(t, err)
	o := &Oracle{
		pools:      make(map[string]*PoolState),
		cdps:       make(map[string]*CDPState),
		storage:    storage,
		mempoolMgr: NewMempoolStateManager(),
		activity:   activity,
	}

	require.NoError(t, o.loadPersistedStates())
	volume, ok, err := o.GetPoolVolume("pool", 100)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, uint64(1), volume.SwapCount)
	require.Equal(t, uint64(100), volume.VolumeX)
	require.Equal(t, uint64(180), volume.VolumeY)
}
