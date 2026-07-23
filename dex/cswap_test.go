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

package dex

import (
	"encoding/hex"
	"math/big"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	cswappkg "github.com/blinklabs-io/shai/dex/cswap"
)

func TestNewCSwapParser(t *testing.T) {
	parser := NewCSwapParser()
	if parser == nil {
		t.Fatal("expected non-nil parser")
	}
	if parser.Protocol() != "cswap" {
		t.Fatalf("expected protocol cswap, got %s", parser.Protocol())
	}
}

func TestCSwapPoolDatumUnmarshal(t *testing.T) {
	datum := newCSwapPoolDatum(
		t,
		uint64(123456),
		uint64(30),
		nil,
		nil,
		mustSizedBytes(28, 0x11),
		[]byte("CSWAP"),
		mustSizedBytes(28, 0x22),
		[]byte("LP"),
	)

	var poolDatum cswappkg.PoolDatum
	if _, err := cbor.Decode(datum, &poolDatum); err != nil {
		t.Fatalf("failed to decode pool datum: %v", err)
	}

	if poolDatum.TotalLpTokens != 123456 {
		t.Fatalf("expected total LP tokens 123456, got %d", poolDatum.TotalLpTokens)
	}
	if poolDatum.PoolFee != 30 {
		t.Fatalf("expected pool fee 30, got %d", poolDatum.PoolFee)
	}
	if string(poolDatum.BaseName) != "CSWAP" {
		t.Fatalf("expected base asset name CSWAP, got %q", string(poolDatum.BaseName))
	}
}

func TestCSwapParserParsePoolDatum(t *testing.T) {
	quotePolicy := []byte{}
	quoteName := []byte{}
	basePolicy := mustSizedBytes(28, 0x11)
	baseName := []byte("CSWAP")
	lpPolicy := mustSizedBytes(28, 0x22)
	lpName := []byte("LP")

	datum := newCSwapPoolDatum(
		t,
		uint64(555000),
		uint64(30),
		quotePolicy,
		quoteName,
		basePolicy,
		baseName,
		lpPolicy,
		lpName,
	)
	utxoValue := newBabbageOutputValue(
		t,
		75_000_000,
		[]assetAmount{
			{policy: basePolicy, name: baseName, amount: 250_000_000},
			{policy: lpPolicy, name: lpName, amount: 555_000},
		},
	)

	parser := NewCSwapParser()
	state, err := parser.ParsePoolDatum(
		datum,
		utxoValue,
		"abc123",
		1,
		12345,
		time.Unix(1700000000, 0).UTC(),
	)
	if err != nil {
		t.Fatalf("failed to parse CSWAP datum: %v", err)
	}

	if state.Protocol != "cswap" {
		t.Fatalf("expected protocol cswap, got %s", state.Protocol)
	}
	expectedPoolID := "cswap_22222222222222222222222222222222222222222222222222222222.4c50"
	if state.PoolId != expectedPoolID {
		t.Fatalf("expected pool ID %s, got %s", expectedPoolID, state.PoolId)
	}
	if state.AssetX.Amount != 75_000_000 {
		t.Fatalf("expected quote reserve 75000000, got %d", state.AssetX.Amount)
	}
	if state.AssetY.Amount != 250_000_000 {
		t.Fatalf("expected base reserve 250000000, got %d", state.AssetY.Amount)
	}
	if state.FeeNum != 9970 {
		t.Fatalf("expected effective fee numerator 9970, got %d", state.FeeNum)
	}
	if state.FeeDenom != 10000 {
		t.Fatalf("expected fee denominator 10000, got %d", state.FeeDenom)
	}
	if got := state.PriceXY(); got != (250_000_000.0 / 75_000_000.0) {
		t.Fatalf("unexpected priceXY %f", got)
	}
}

func TestCSwapParserCurrentMainnetStablecoinPools(t *testing.T) {
	tests := []struct {
		name          string
		datumHex      string
		lovelace      uint64
		stablePolicy  string
		stableName    string
		stableReserve uint64
		txHash        string
		slot          uint64
		blockTime     int64
		wantFeeNum    uint64
	}{
		{
			name:          "ADA-USDCx",
			datumHex:      "d8799f1acfd6bdec054040581c1f3aec8bfe7ea4fe14c5f121e2a92e301afe414147860d557cac7e34455553444378581c37834c1d8615d6e5d2ec1c631f00d6f352ffd1ef16541204b971766151432d4c503a204144412078205553444378ff",
			lovelace:      8_547_275_688,
			stablePolicy:  "1f3aec8bfe7ea4fe14c5f121e2a92e301afe414147860d557cac7e34",
			stableName:    "5553444378",
			stableReserve: 1_439_463_431,
			txHash:        "a24fd5df3faebb06ba0aa815890d5c7e3907c27f428c906b792d81321254a8d1",
			slot:          193_253_908,
			blockTime:     1_784_820_199,
			wantFeeNum:    9_995,
		},
		{
			name:          "ADA-USDM",
			datumHex:      "d8799f1a6a3f71dc18554040581cc48cbb3d5e57ed56e276bc45f99ab39abe94e6cd7ac39fb402da47ad480014df105553444d581c16c66059d8ed65d35a64b229e34bfcd01899de45379a35c3aa74c7a556432d4c503a204144412078200014efbfbd105553444dff",
			lovelace:      4_579_285_253,
			stablePolicy:  "c48cbb3d5e57ed56e276bc45f99ab39abe94e6cd7ac39fb402da47ad",
			stableName:    "0014df105553444d",
			stableReserve: 774_654_393,
			txHash:        "1237c072b8d283e3ccc3b9956502825f11533973b5402ed1ef1df459ffca8bfc",
			slot:          193_255_027,
			blockTime:     1_784_821_318,
			wantFeeNum:    9_915,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			datum, err := hex.DecodeString(test.datumHex)
			if err != nil {
				t.Fatalf("decode datum fixture: %v", err)
			}
			policy, err := hex.DecodeString(test.stablePolicy)
			if err != nil {
				t.Fatalf("decode policy fixture: %v", err)
			}
			name, err := hex.DecodeString(test.stableName)
			if err != nil {
				t.Fatalf("decode name fixture: %v", err)
			}
			utxoValue := newBabbageOutputValue(
				t,
				test.lovelace,
				[]assetAmount{{
					policy: policy,
					name:   name,
					amount: test.stableReserve,
				}},
			)

			state, err := NewCSwapParser().ParsePoolDatum(
				datum,
				utxoValue,
				test.txHash,
				1,
				test.slot,
				time.Unix(test.blockTime, 0).UTC(),
			)
			if err != nil {
				t.Fatalf("parse current mainnet fixture: %v", err)
			}
			if !state.AssetX.IsLovelace() {
				t.Fatal("expected quote asset to be lovelace")
			}
			if state.AssetX.Amount != test.lovelace {
				t.Fatalf(
					"expected %d lovelace, got %d",
					test.lovelace,
					state.AssetX.Amount,
				)
			}
			if state.AssetY.Class.PolicyIdHex() != test.stablePolicy ||
				state.AssetY.Class.NameHex() != test.stableName {
				t.Fatalf(
					"unexpected stablecoin asset %s",
					state.AssetY.Class.Fingerprint(),
				)
			}
			if state.AssetY.Amount != test.stableReserve {
				t.Fatalf(
					"expected stable reserve %d, got %d",
					test.stableReserve,
					state.AssetY.Amount,
				)
			}
			if state.FeeNum != test.wantFeeNum ||
				state.FeeDenom != cswappkg.FeeDenom {
				t.Fatalf(
					"unexpected fee %d/%d",
					state.FeeNum,
					state.FeeDenom,
				)
			}
			if state.TxHash != test.txHash || state.Slot != test.slot {
				t.Fatalf(
					"unexpected chain reference %s@%d",
					state.TxHash,
					state.Slot,
				)
			}
			if state.Timestamp.Unix() != test.blockTime {
				t.Fatalf(
					"expected block time %d, got %d",
					test.blockTime,
					state.Timestamp.Unix(),
				)
			}
		})
	}
}

func TestCSwapParserRejectsMalformedLovelaceAsset(t *testing.T) {
	datum := newCSwapPoolDatum(
		t,
		uint64(1),
		uint64(30),
		nil,
		[]byte("bad"),
		mustSizedBytes(28, 0x11),
		[]byte("CSWAP"),
		mustSizedBytes(28, 0x22),
		[]byte("LP"),
	)
	utxoValue := newBabbageOutputValue(
		t,
		1_000_000,
		[]assetAmount{
			{policy: mustSizedBytes(28, 0x11), name: []byte("CSWAP"), amount: 1},
		},
	)

	parser := NewCSwapParser()
	_, err := parser.ParsePoolDatum(
		datum,
		utxoValue,
		"abc123",
		0,
		1,
		time.Now(),
	)
	if err == nil {
		t.Fatal("expected malformed lovelace asset error")
	}
}

type assetAmount struct {
	policy []byte
	name   []byte
	amount uint64
}

func newCSwapPoolDatum(
	t *testing.T,
	totalLP uint64,
	poolFee uint64,
	quotePolicy []byte,
	quoteName []byte,
	basePolicy []byte,
	baseName []byte,
	lpPolicy []byte,
	lpName []byte,
) []byte {
	t.Helper()

	datum := cbor.NewConstructorEncoder(0, cbor.IndefLengthList{
		totalLP,
		poolFee,
		quotePolicy,
		quoteName,
		basePolicy,
		baseName,
		lpPolicy,
		lpName,
	})
	data, err := cbor.Encode(&datum)
	if err != nil {
		t.Fatalf("failed to encode pool datum: %v", err)
	}
	return data
}

func newBabbageOutputValue(
	t *testing.T,
	lovelace uint64,
	assets []assetAmount,
) []byte {
	t.Helper()

	address, err := lcommon.NewAddressFromParts(
		lcommon.AddressTypeKeyNone,
		lcommon.AddressNetworkMainnet,
		mustSizedBytes(28, 0x99),
		nil,
	)
	if err != nil {
		t.Fatalf("failed to build test address: %v", err)
	}

	var multiAsset *lcommon.MultiAsset[lcommon.MultiAssetTypeOutput]
	if len(assets) > 0 {
		multiAssetData := make(map[lcommon.Blake2b224]map[cbor.ByteString]*big.Int)
		for _, asset := range assets {
			policy := lcommon.NewBlake2b224(asset.policy)
			if _, ok := multiAssetData[policy]; !ok {
				multiAssetData[policy] = make(map[cbor.ByteString]*big.Int)
			}
			multiAssetData[policy][cbor.NewByteString(asset.name)] = new(big.Int).SetUint64(asset.amount)
		}
		tmp := lcommon.NewMultiAsset[lcommon.MultiAssetTypeOutput](multiAssetData)
		multiAsset = &tmp
	}

	txOut := babbage.BabbageTransactionOutput{
		OutputAddress: address,
		OutputAmount: mary.MaryTransactionOutputValue{
			Amount: lovelace,
			Assets: multiAsset,
		},
	}
	data, err := cbor.Encode(&txOut)
	if err != nil {
		t.Fatalf("failed to encode output value: %v", err)
	}
	return data
}

func mustSizedBytes(size int, fill byte) []byte {
	ret := make([]byte, size)
	for i := range ret {
		ret[i] = fill
	}
	return ret
}
