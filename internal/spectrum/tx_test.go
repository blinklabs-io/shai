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

package spectrum

import (
	"context"
	"math"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/ledger/common"
)

func TestSwapTxChainContextFixedMaxTxFee(t *testing.T) {
	chainContext := newSwapTxChainContext()

	maxFee, err := chainContext.MaxTxFee(context.Background())
	if err != nil {
		t.Fatalf("unexpected max fee error: %s", err)
	}
	if maxFee != swapTxFee {
		t.Fatalf("expected max fee %d, got %d", swapTxFee, maxFee)
	}

	requiredCollateral, err := requiredCollateralLovelace(
		context.Background(),
		chainContext,
		swapTxFee,
	)
	if err != nil {
		t.Fatalf("unexpected collateral sizing error: %s", err)
	}
	const expectedCollateral = 442_500
	if requiredCollateral != expectedCollateral {
		t.Fatalf(
			"expected required collateral %d, got %d",
			expectedCollateral,
			requiredCollateral,
		)
	}
}

func TestSwapTxChainContextReferenceFallback(t *testing.T) {
	chainContext := newSwapTxChainContext()
	var txHash common.Blake2b256
	txHash[0] = 0xab

	utxo, err := chainContext.UtxoByRef(context.Background(), txHash, 3)
	if err != nil {
		t.Fatalf("unexpected reference lookup error: %s", err)
	}
	if utxo.Id.Id() != txHash || utxo.Id.Index() != 3 {
		t.Fatalf(
			"unexpected fallback UTxO id: got %s#%d",
			utxo.Id.Id().String(),
			utxo.Id.Index(),
		)
	}
	if utxo.Output == nil {
		t.Fatal("expected fallback UTxO output")
	}
	if script := utxo.Output.ScriptRef(); script != nil {
		t.Fatalf("expected fallback UTxO without script ref, got %T", script)
	}
}

func TestSelectCollateralUtxoRequiresMinimumAndUsesSmallest(t *testing.T) {
	chainContext := newSwapTxChainContext()
	requiredCollateral, err := requiredCollateralLovelace(
		context.Background(),
		chainContext,
		swapTxFee,
	)
	if err != nil {
		t.Fatalf("unexpected collateral sizing error: %s", err)
	}

	small := testCollateralUtxo(t, 0x01, requiredCollateral-1)
	large := testCollateralUtxo(t, 0x02, requiredCollateral+1_000_000)
	exact := testCollateralUtxo(t, 0x03, requiredCollateral)

	selected, err := selectCollateralUtxo(
		[]common.Utxo{large, small, exact},
		requiredCollateral,
	)
	if err != nil {
		t.Fatalf("unexpected collateral selection error: %s", err)
	}
	if selected.Id.Id() != exact.Id.Id() {
		t.Fatalf(
			"expected smallest eligible collateral %s, got %s",
			exact.Id.Id().String(),
			selected.Id.Id().String(),
		)
	}

	if _, err := selectCollateralUtxo(
		[]common.Utxo{small},
		requiredCollateral,
	); err == nil {
		t.Fatal("expected insufficient collateral error")
	}
}

func TestUint64ToInt64RejectsOverflow(t *testing.T) {
	got, err := uint64ToInt64("test amount", math.MaxInt64)
	if err != nil {
		t.Fatalf("unexpected conversion error: %s", err)
	}
	if got != math.MaxInt64 {
		t.Fatalf("expected %d, got %d", math.MaxInt64, got)
	}

	if _, err := uint64ToInt64(
		"test amount",
		uint64(math.MaxInt64)+1,
	); err == nil {
		t.Fatal("expected overflow conversion error")
	}
}

func testCollateralUtxo(
	t *testing.T,
	txHashByte byte,
	lovelace uint64,
) common.Utxo {
	t.Helper()
	var rawAddr [57]byte
	rawAddr[0] = 0x00
	rawAddr[1] = 0xaa
	rawAddr[29] = 0xbb
	addr, err := common.NewAddressFromBytes(rawAddr[:])
	if err != nil {
		t.Fatal(err)
	}
	var txHash common.Blake2b256
	txHash[0] = txHashByte
	input := ledger.ShelleyTransactionInput{
		TxId:        txHash,
		OutputIndex: 0,
	}
	output := ledger.BabbageTransactionOutput{
		OutputAddress: addr,
		OutputAmount: ledger.MaryTransactionOutputValue{
			Amount: lovelace,
		},
	}
	return common.Utxo{
		Id:     input,
		Output: &output,
	}
}
