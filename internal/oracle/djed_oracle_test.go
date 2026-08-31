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
	"encoding/hex"
	"errors"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/blinklabs-io/adder/event"
	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/blinklabs-io/shai/internal/indexer"
	"github.com/blinklabs-io/shai/internal/storage"
	"github.com/blinklabs-io/shai/price/djed"
	"github.com/stretchr/testify/require"
)

const djedDatumFixture = "d8799f584004ea10278c7b8c3c636536a8a1b831d8e193e8aca7df1ee2b83fe856f1fede93fb818e3453f135f37a68d464bf3c6e38d1e4e4750d60cba6dbc3a96132aa6507d8799fd8799f1a000f42401a00029463ffd8799fd8799fd87a9f1b0000019f90e8fcc0ffd87a80ffd8799fd87a9f1b0000019f90f6b860ffd87a80ffff43555344ff581c815aca02042ba9188a2ca4f8ce7b276046e2376b4bce56391342299eff"

type testDjedStorage struct {
	state   djed.TrackerState
	present bool
	saveErr error
	saves   int
}

func (s *testDjedStorage) SaveDjedState(
	_ string,
	state djed.TrackerState,
) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	s.state = state
	s.present = true
	s.saves++
	return nil
}

func (s *testDjedStorage) LoadDjedState(
	_ string,
) (djed.TrackerState, error) {
	if !s.present {
		return djed.TrackerState{}, storage.ErrDjedStateNotFound
	}
	return s.state, nil
}

func TestDjedOracleTracksSpendRollbackAndRestart(t *testing.T) {
	stateStorage := &testDjedStorage{}
	oracle := NewDjedOracle(
		indexer.New(),
		"mainnet",
		djed.MainnetOracleAddress,
		stateStorage,
	)
	require.NoError(t, oracle.Start())
	now := time.Unix(1_784_842_625, 0).UTC()
	txHash := strings.Repeat("ab", 32)
	require.NoError(t, oracle.HandleChainsyncEvent(event.Event{
		Timestamp: now,
		Context: event.TransactionContext{
			TransactionHash: txHash,
			TransactionIdx:  3,
			SlotNumber:      100,
		},
		Payload: event.TransactionEvent{
			BlockHash: "block-100",
			Outputs: []ledger.TransactionOutput{
				testDjedOutput(t),
			},
		},
	}))
	current, err := oracle.Current(now)
	require.NoError(t, err)
	require.Equal(t, txHash, current.TxHash)
	require.Equal(t, uint32(3), current.TransactionIndex)
	require.Equal(t, "block-100", current.BlockHash)

	require.NoError(t, oracle.HandleChainsyncEvent(event.Event{
		Timestamp: now.Add(time.Minute),
		Context: event.TransactionContext{
			TransactionHash: strings.Repeat("cd", 32),
			SlotNumber:      101,
		},
		Payload: event.TransactionEvent{
			Inputs: []ledger.TransactionInput{
				shelley.NewShelleyTransactionInput(txHash, 0),
			},
		},
	}))
	_, err = oracle.Current(now)
	require.ErrorIs(t, err, djed.ErrNoCurrentObservation)

	require.NoError(t, oracle.HandleChainsyncEvent(event.Event{
		Payload: event.RollbackEvent{SlotNumber: 100},
	}))
	current, err = oracle.Current(now)
	require.NoError(t, err)
	require.Equal(t, txHash, current.TxHash)
	saves := stateStorage.saves
	require.NoError(t, oracle.HandleChainsyncEvent(event.Event{
		Payload: event.RollbackEvent{SlotNumber: 100},
	}))
	require.Equal(t, saves, stateStorage.saves)

	restarted := NewDjedOracle(
		indexer.New(),
		"mainnet",
		djed.MainnetOracleAddress,
		stateStorage,
	)
	require.NoError(t, restarted.Start())
	current, err = restarted.Current(now)
	require.NoError(t, err)
	require.Equal(t, txHash, current.TxHash)
}

func TestDjedOracleReturnsPersistenceErrorsWithoutChangingMemory(t *testing.T) {
	saveErr := errors.New("disk unavailable")
	stateStorage := &testDjedStorage{saveErr: saveErr}
	oracle := NewDjedOracle(
		indexer.New(),
		"mainnet",
		djed.MainnetOracleAddress,
		stateStorage,
	)
	require.NoError(t, oracle.Start())
	now := time.Unix(1_784_842_625, 0).UTC()
	err := oracle.HandleChainsyncEvent(event.Event{
		Timestamp: now,
		Context: event.TransactionContext{
			TransactionHash: strings.Repeat("ef", 32),
			SlotNumber:      100,
		},
		Payload: event.TransactionEvent{
			Outputs: []ledger.TransactionOutput{
				testDjedOutput(t),
			},
		},
	})
	require.ErrorIs(t, err, saveErr)
	_, err = oracle.Current(now)
	require.ErrorIs(t, err, djed.ErrNoCurrentObservation)
}

func TestDjedOracleIgnoresMalformedChainOutputs(t *testing.T) {
	stateStorage := &testDjedStorage{}
	oracle := NewDjedOracle(
		indexer.New(),
		"mainnet",
		djed.MainnetOracleAddress,
		stateStorage,
	)
	require.NoError(t, oracle.Start())
	now := time.Unix(1_784_842_625, 0).UTC()
	txHashes := []string{
		strings.Repeat("01", 32),
		strings.Repeat("02", 32),
	}
	for i, output := range []ledger.TransactionOutput{
		testDjedOutputWithDatum(t, nil),
		testDjedOutputWithDatum(t, []byte{0x01}),
	} {
		require.NoError(t, oracle.HandleChainsyncEvent(event.Event{
			Timestamp: now,
			Context: event.TransactionContext{
				TransactionHash: txHashes[i],
				SlotNumber:      uint64(100 + i),
			},
			Payload: event.TransactionEvent{
				Outputs: []ledger.TransactionOutput{output},
			},
		}))
	}
	require.Equal(t, 0, stateStorage.saves)
	_, err := oracle.Current(now)
	require.ErrorIs(t, err, djed.ErrNoCurrentObservation)
}

func TestDjedOracleIgnoresOutputWithForeignAsset(t *testing.T) {
	stateStorage := &testDjedStorage{}
	oracle := NewDjedOracle(
		indexer.New(),
		"mainnet",
		djed.MainnetOracleAddress,
		stateStorage,
	)
	require.NoError(t, oracle.Start())
	now := time.Unix(1_784_842_625, 0).UTC()
	require.NoError(t, oracle.HandleChainsyncEvent(event.Event{
		Timestamp: now,
		Context: event.TransactionContext{
			TransactionHash: strings.Repeat("03", 32),
			SlotNumber:      100,
		},
		Payload: event.TransactionEvent{
			Outputs: []ledger.TransactionOutput{
				testDjedOutputWithForeignAsset(t),
			},
		},
	}))
	require.Equal(t, 0, stateStorage.saves)
	_, err := oracle.Current(now)
	require.ErrorIs(t, err, djed.ErrNoCurrentObservation)
}

func TestDjedOraclePrunesSpentHistoryBeyondStabilityWindow(t *testing.T) {
	stateStorage := &testDjedStorage{}
	oracle := NewDjedOracle(
		indexer.New(),
		"mainnet",
		djed.MainnetOracleAddress,
		stateStorage,
	)
	require.NoError(t, oracle.Start())
	now := time.Unix(1_784_842_625, 0).UTC()
	firstHash := strings.Repeat("ab", 32)
	require.NoError(t, oracle.HandleChainsyncEvent(event.Event{
		Timestamp: now,
		Context: event.TransactionContext{
			TransactionHash: firstHash,
			SlotNumber:      100,
		},
		Payload: event.TransactionEvent{
			Outputs: []ledger.TransactionOutput{testDjedOutput(t)},
		},
	}))
	require.NoError(t, oracle.HandleChainsyncEvent(event.Event{
		Timestamp: now,
		Context: event.TransactionContext{
			TransactionHash: strings.Repeat("cd", 32),
			SlotNumber:      101,
		},
		Payload: event.TransactionEvent{
			Inputs: []ledger.TransactionInput{
				shelley.NewShelleyTransactionInput(firstHash, 0),
			},
		},
	}))
	secondHash := strings.Repeat("ef", 32)
	require.NoError(t, oracle.HandleChainsyncEvent(event.Event{
		Timestamp: now,
		Context: event.TransactionContext{
			TransactionHash: secondHash,
			SlotNumber:      djedRollbackRetentionSlots + 102,
		},
		Payload: event.TransactionEvent{
			Outputs: []ledger.TransactionOutput{testDjedOutput(t)},
		},
	}))
	require.Len(t, stateStorage.state.Observations, 1)
	require.Equal(
		t,
		secondHash,
		stateStorage.state.Observations[0].Observation.TxHash,
	)
}

func testDjedOutput(t *testing.T) ledger.TransactionOutput {
	t.Helper()
	datum, err := hex.DecodeString(djedDatumFixture)
	require.NoError(t, err)
	return testDjedOutputWithDatum(t, datum)
}

func testDjedOutputWithForeignAsset(t *testing.T) ledger.TransactionOutput {
	t.Helper()
	address, err := lcommon.NewAddress(djed.MainnetOracleAddress)
	require.NoError(t, err)
	policyBytes, err := hex.DecodeString(strings.Repeat("11", 28))
	require.NoError(t, err)
	var policy lcommon.Blake2b224
	copy(policy[:], policyBytes)
	assets := lcommon.NewMultiAsset[lcommon.MultiAssetTypeOutput](
		map[lcommon.Blake2b224]map[cbor.ByteString]lcommon.MultiAssetTypeOutput{
			policy: {
				cbor.NewByteString([]byte("SPAM")): big.NewInt(1),
			},
		},
	)
	outputCbor, err := cbor.Encode(&map[uint64]any{
		0: address,
		1: mary.MaryTransactionOutputValue{
			Amount: 1_500_000,
			Assets: &assets,
		},
	})
	require.NoError(t, err)
	output, err := ledger.NewTransactionOutputFromCbor(outputCbor)
	require.NoError(t, err)
	return output
}

func testDjedOutputWithDatum(
	t *testing.T,
	datum []byte,
) ledger.TransactionOutput {
	t.Helper()
	address, err := lcommon.NewAddress(djed.MainnetOracleAddress)
	require.NoError(t, err)
	policyBytes, err := hex.DecodeString(djed.MainnetOraclePolicy)
	require.NoError(t, err)
	name, err := hex.DecodeString(djed.OracleNFTName)
	require.NoError(t, err)
	var policy lcommon.Blake2b224
	copy(policy[:], policyBytes)
	assets := lcommon.NewMultiAsset[lcommon.MultiAssetTypeOutput](
		map[lcommon.Blake2b224]map[cbor.ByteString]lcommon.MultiAssetTypeOutput{
			policy: {
				cbor.NewByteString(name): big.NewInt(1),
			},
		},
	)
	outputFields := map[uint64]any{
		0: address,
		1: mary.MaryTransactionOutputValue{
			Amount: 1_810_200,
			Assets: &assets,
		},
	}
	if datum != nil {
		outputFields[2] = []any{
			uint64(1),
			cbor.Tag{Number: 24, Content: datum},
		}
	}
	outputCbor, err := cbor.Encode(&outputFields)
	require.NoError(t, err)
	output, err := ledger.NewTransactionOutputFromCbor(outputCbor)
	require.NoError(t, err)
	if datum == nil {
		require.Nil(t, output.Datum())
	} else {
		require.NotNil(t, output.Datum())
	}
	return output
}
