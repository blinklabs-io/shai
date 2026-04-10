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
	"math/big"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	cswappkg "github.com/blinklabs-io/shai/internal/oracle/cswap"
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

	datum := cbor.NewConstructor(0, cbor.IndefLengthList{
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
