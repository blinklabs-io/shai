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

package oraclefeed

import (
	"bytes"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	orcfaxFixture  = "d8799fd8799f4d4345522f4144412d5553442f331b0000019edc8ac6a2d8799f1a00027e051a000f4240ffffd8799f581c3c12f6735ef87655c5b27bced3f828d857d0a27fd20f2cda18ebf2fbffff"
	charli3Fixture = "d8799fd87b9fa3001a0003a891011b0000019e67e17924021b0000019e692b1024ffff"
)

func mustHex(t *testing.T, value string) []byte {
	t.Helper()
	decoded, err := hex.DecodeString(value)
	require.NoError(t, err)
	return decoded
}

func TestOrcfaxADAUSDParserOnChainFixture(t *testing.T) {
	utxo := UTxO{
		Address: OrcfaxADAUSDAddress,
		Assets: []Asset{{
			PolicyID: OrcfaxFeedPolicyID,
			Name:     OrcfaxFeedAssetName,
			Quantity: 1,
		}},
		Datum:   mustHex(t, orcfaxFixture),
		TxHash:  "a16b451afbb11198a0f179d79c5dc6ebe863e2c348783e088a3d8c0f5952b91f",
		TxIndex: 0,
	}

	got, err := (OrcfaxADAUSDParser{}).Parse(utxo)
	require.NoError(t, err)
	require.Equal(t, SourceOrcfax, got.Source)
	require.Equal(t, PairADAUSD, got.Pair)
	require.Equal(t, "CER/ADA-USD/3", got.FeedID)
	require.Equal(t, uint64(163333), got.Numerator)
	require.Equal(t, uint64(1_000_000), got.Denominator)
	require.Equal(t, time.Date(2026, 6, 18, 21, 2, 22, 882_000_000, time.UTC), got.ObservedAt)
	require.InDelta(t, 0.163333, got.Float64(), 0.0000001)
	require.True(t, got.FreshAt(got.ObservedAt.Add(time.Hour), 2*time.Hour))
	require.False(t, got.FreshAt(got.ObservedAt.Add(3*time.Hour), 2*time.Hour))
}

func TestCharli3ADAUSDParserOnChainFixture(t *testing.T) {
	utxo := UTxO{
		Address: Charli3ADAUSDAddress,
		Assets: []Asset{{
			PolicyID: Charli3FeedPolicyID,
			Name:     Charli3FeedAssetName,
			Quantity: 1,
		}},
		Datum:   mustHex(t, charli3Fixture),
		TxHash:  "15700eca28190554ff3a435943faced334648d2f2485fd2938815c4c5fcfaab4",
		TxIndex: 1,
	}

	got, err := (Charli3ADAUSDParser{}).Parse(utxo)
	require.NoError(t, err)
	require.Equal(t, SourceCharli3, got.Source)
	require.Equal(t, uint64(239761), got.Numerator)
	require.Equal(t, uint64(1_000_000), got.Denominator)
	require.Equal(t, time.Date(2026, 5, 27, 5, 21, 30, 404_000_000, time.UTC), got.ObservedAt)
	require.Equal(t, time.Date(2026, 5, 27, 11, 21, 30, 404_000_000, time.UTC), got.ExpiresAt)
	require.InDelta(t, 0.239761, got.Float64(), 0.0000001)
	require.True(t, got.FreshAt(got.ObservedAt.Add(time.Hour), 7*time.Hour))
	require.False(t, got.FreshAt(got.ExpiresAt, 7*time.Hour))
}

func TestParserRejectsUnauthenticatedUTxO(t *testing.T) {
	_, err := (OrcfaxADAUSDParser{}).Parse(UTxO{
		Address: OrcfaxADAUSDAddress,
		Datum:   mustHex(t, orcfaxFixture),
	})
	require.ErrorIs(t, err, ErrMissingAuthAsset)

	_, err = (Charli3ADAUSDParser{}).Parse(UTxO{
		Address: "addr1wrong",
		Assets: []Asset{{
			PolicyID: Charli3FeedPolicyID,
			Name:     Charli3FeedAssetName,
			Quantity: 1,
		}},
		Datum: mustHex(t, charli3Fixture),
	})
	require.ErrorIs(t, err, ErrWrongAddress)
}

func TestParserRejectsMalformedDatum(t *testing.T) {
	_, err := (OrcfaxADAUSDParser{}).Parse(UTxO{
		Address: OrcfaxADAUSDAddress,
		Assets: []Asset{{
			PolicyID: OrcfaxFeedPolicyID,
			Quantity: 1,
		}},
		Datum: []byte{0xff},
	})
	require.True(t, errors.Is(err, ErrInvalidDatum))
}

func TestOrcfaxParserRejectsWrongFeedAndZeroDenominator(t *testing.T) {
	base := UTxO{
		Address: OrcfaxADAUSDAddress,
		Assets: []Asset{{
			PolicyID: OrcfaxFeedPolicyID,
			Quantity: 1,
		}},
		Datum: mustHex(t, orcfaxFixture),
	}

	wrongFeed := base
	wrongFeed.Datum = bytes.Replace(
		base.Datum,
		[]byte("CER/ADA-USD/3"),
		[]byte("CER/BTC-USD/3"),
		1,
	)
	_, err := (OrcfaxADAUSDParser{}).Parse(wrongFeed)
	require.ErrorIs(t, err, ErrUnexpectedFeed)

	zeroDenominator := base
	zeroDenominator.Datum = bytes.Replace(
		base.Datum,
		[]byte{0x1a, 0x00, 0x0f, 0x42, 0x40},
		[]byte{0x1a, 0x00, 0x00, 0x00, 0x00},
		1,
	)
	_, err = (OrcfaxADAUSDParser{}).Parse(zeroDenominator)
	require.ErrorIs(t, err, ErrInvalidPrice)
}

func TestTrackerUsesOnlyFreshAuthenticatedLocalObservations(t *testing.T) {
	tracker := NewTracker()
	require.NoError(t, tracker.ValidateConfiguration())

	orcfax := UTxO{
		Address: OrcfaxADAUSDAddress,
		Assets: []Asset{{
			PolicyID: OrcfaxFeedPolicyID,
			Quantity: 1,
		}},
		Datum:   mustHex(t, orcfaxFixture),
		TxHash:  "a16b451afbb11198a0f179d79c5dc6ebe863e2c348783e088a3d8c0f5952b91f",
		TxIndex: 0,
		Slot:    1,
	}
	_, matched, err := tracker.Apply(orcfax)
	require.True(t, matched)
	require.NoError(t, err)

	now := time.Date(2026, 7, 23, 18, 4, 4, 0, time.UTC)
	_, err = tracker.ADAUSD(now)
	require.ErrorIs(t, err, ErrNoFreshObservation)
	statuses := tracker.Sources(now)
	require.Len(t, statuses, 2)
	require.Equal(t, SourceCharli3, statuses[0].Source)
	require.Equal(t, SourceOrcfax, statuses[1].Source)
	require.Contains(t, statuses[1].Error, "stale")

	freshAt := time.Date(2026, 6, 18, 22, 0, 0, 0, time.UTC)
	got, err := tracker.ADAUSD(freshAt)
	require.NoError(t, err)
	require.Equal(t, SourceOrcfax, got.Source)

	tracker.Consume(OutputRef{TxHash: orcfax.TxHash, TxIndex: orcfax.TxIndex})
	_, err = tracker.ADAUSD(freshAt)
	require.ErrorIs(t, err, ErrNoFreshObservation)
}

func TestTrackerRollbackRestoresPreviousObservation(t *testing.T) {
	tracker := NewTracker()
	older := UTxO{
		Address: OrcfaxADAUSDAddress,
		Assets: []Asset{{
			PolicyID: OrcfaxFeedPolicyID,
			Quantity: 1,
		}},
		Datum:   mustHex(t, orcfaxFixture),
		TxHash:  "older",
		TxIndex: 0,
		Slot:    10,
	}
	newer := older
	newer.TxHash = "newer"
	newer.Slot = 20
	require.NoError(t, applyMatched(tracker, older))
	require.NoError(t, applyMatched(tracker, newer))

	tracker.Rollback(20)
	statuses := tracker.Sources(time.Date(2026, 6, 18, 22, 0, 0, 0, time.UTC))
	require.Equal(t, "older", statuses[1].Observation.TxHash)
}

func TestTrackerRollbackRestoresRolledBackSpend(t *testing.T) {
	tracker := NewTracker()
	utxo := UTxO{
		Address: OrcfaxADAUSDAddress,
		Assets: []Asset{{
			PolicyID: OrcfaxFeedPolicyID,
			Quantity: 1,
		}},
		Datum:   mustHex(t, orcfaxFixture),
		TxHash:  "feed",
		TxIndex: 0,
		Slot:    10,
	}
	require.NoError(t, applyMatched(tracker, utxo))
	tracker.ConsumeAt(OutputRef{TxHash: "feed", TxIndex: 0}, 20)
	now := time.Date(2026, 6, 18, 22, 0, 0, 0, time.UTC)
	_, err := tracker.ADAUSD(now)
	require.ErrorIs(t, err, ErrNoFreshObservation)

	tracker.Rollback(20)
	got, err := tracker.ADAUSD(now)
	require.NoError(t, err)
	require.Equal(t, "feed", got.TxHash)
}

func TestTrackerRejectsDivergentFreshSources(t *testing.T) {
	observedAt := time.Date(2026, 7, 23, 18, 0, 0, 0, time.UTC)
	first := staticParser{
		source:  "first",
		address: "addr_first",
		value: Observation{
			Source: "first", Pair: PairADAUSD,
			Numerator: 40, Denominator: 100, ObservedAt: observedAt,
		},
	}
	second := staticParser{
		source:  "second",
		address: "addr_second",
		value: Observation{
			Source: "second", Pair: PairADAUSD,
			Numerator: 50, Denominator: 100, ObservedAt: observedAt,
		},
	}
	tracker := NewTrackerWithParsers(map[Parser]time.Duration{
		first: time.Hour, second: time.Hour,
	})
	_, _, err := tracker.Apply(UTxO{Address: first.address, TxHash: "first"})
	require.NoError(t, err)
	_, _, err = tracker.Apply(UTxO{Address: second.address, TxHash: "second"})
	require.NoError(t, err)

	_, err = tracker.ADAUSD(observedAt.Add(time.Minute))
	require.ErrorIs(t, err, ErrDivergentObservations)
}

func applyMatched(tracker *Tracker, utxo UTxO) error {
	_, matched, err := tracker.Apply(utxo)
	if !matched {
		return errors.New("fixture did not match a parser")
	}
	return err
}

type staticParser struct {
	source  string
	address string
	value   Observation
}

func (p staticParser) Source() string  { return p.source }
func (p staticParser) Pair() string    { return PairADAUSD }
func (p staticParser) Address() string { return p.address }
func (p staticParser) Parse(utxo UTxO) (Observation, error) {
	value := p.value
	value.TxHash = utxo.TxHash
	return value, nil
}
