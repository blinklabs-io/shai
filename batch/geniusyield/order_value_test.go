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

package geniusyield

import (
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	dexgy "github.com/blinklabs-io/shai/dex/geniusyield"
	"github.com/stretchr/testify/require"
)

func TestHasOfferedAmount(t *testing.T) {
	t.Parallel()

	policy := make([]byte, lcommon.Blake2b224Size)
	policy[0] = 0x42
	name := []byte("offered")
	assets := lcommon.NewMultiAsset[lcommon.MultiAssetTypeOutput](
		map[lcommon.Blake2b224]map[cbor.ByteString]*big.Int{
			lcommon.NewBlake2b224(policy): {
				cbor.NewByteString(name): big.NewInt(9),
			},
		},
	)
	output := &babbage.BabbageTransactionOutput{
		OutputAmount: mary.MaryTransactionOutputValue{
			Amount: 10,
			Assets: &assets,
		},
	}

	for _, test := range []struct {
		name  string
		order dexgy.OrderConfig
		want  bool
	}{
		{
			name: "funded lovelace",
			order: dexgy.OrderConfig{
				OfferedAmount: 10,
			},
			want: true,
		},
		{
			name: "underfunded lovelace",
			order: dexgy.OrderConfig{
				OfferedAmount: 11,
			},
		},
		{
			name: "funded native asset",
			order: dexgy.OrderConfig{
				OfferedAsset: dexgy.OrderAsset{
					PolicyId:  policy,
					AssetName: name,
				},
				OfferedAmount: 9,
			},
			want: true,
		},
		{
			name: "underfunded native asset",
			order: dexgy.OrderConfig{
				OfferedAsset: dexgy.OrderAsset{
					PolicyId:  policy,
					AssetName: name,
				},
				OfferedAmount: 10,
			},
		},
		{
			name: "malformed policy",
			order: dexgy.OrderConfig{
				OfferedAsset: dexgy.OrderAsset{
					PolicyId:  policy[:len(policy)-1],
					AssetName: name,
				},
				OfferedAmount: 1,
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, test.want, hasOfferedAmount(output, test.order))
		})
	}
}
