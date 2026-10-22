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

package node

import (
	"testing"
	"time"

	"github.com/blinklabs-io/adder/event"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/ouroboros-mock/fixtures"
	"github.com/stretchr/testify/require"
)

func TestChainsyncClientHandleBlockEventUsesDecodedBlockType(t *testing.T) {
	blocks, err := fixtures.GenerateConwayChain(
		1,
		common.Blake2b256{},
		100,
		1,
		1,
	)
	require.NoError(t, err)
	require.Len(t, blocks, 1)

	block := blocks[0]
	evt := event.New(
		event.TypeBlock,
		time.Time{},
		event.NewBlockContext(block, 0),
		event.NewBlockEvent(block, true),
	)
	n := &Node{chainsyncClientState: &chainsyncClientState{}}

	require.NoError(t, n.chainsyncClientHandleEvent(evt))
	require.Len(t, n.chainsyncClientState.recentBlocks, 1)
	require.Equal(
		t,
		uint(block.Type()),
		n.chainsyncClientState.recentBlocks[0].Type,
	)
	require.Equal(
		t,
		block.Cbor(),
		n.chainsyncClientState.recentBlocks[0].Cbor,
	)
}
