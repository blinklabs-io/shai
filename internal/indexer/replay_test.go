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

package indexer

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSetReplayPointValidatesAndStoresPoint(t *testing.T) {
	idx := New()
	require.Error(t, idx.SetReplayPoint(1, "00"))

	hash := "79fd4b75508af079ab01bcbaa68ba1fe0ff3776a087056f33239341a8532d92d"
	require.NoError(t, idx.SetReplayPoint(188293000, hash))
	require.NotNil(t, idx.replayPoint)
	require.Equal(t, uint64(188293000), idx.replayPoint.Slot)
	require.Equal(t, hash, hex.EncodeToString(idx.replayPoint.Hash))
}
