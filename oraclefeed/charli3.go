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
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
)

const (
	charli3PriceKey     uint64 = 0
	charli3TimestampKey uint64 = 1
	charli3ExpiryKey    uint64 = 2
)

// Charli3ADAUSDParser parses the documented Charli3 mainnet ADA/USD feed.
type Charli3ADAUSDParser struct{}

func (Charli3ADAUSDParser) Source() string  { return SourceCharli3 }
func (Charli3ADAUSDParser) Pair() string    { return PairADAUSD }
func (Charli3ADAUSDParser) Address() string { return Charli3ADAUSDAddress }

func (Charli3ADAUSDParser) Parse(utxo UTxO) (Observation, error) {
	if err := authenticate(
		utxo,
		Charli3ADAUSDAddress,
		Charli3FeedPolicyID,
		Charli3FeedAssetName,
	); err != nil {
		return Observation{}, err
	}

	_, outerFields, err := decodeConstructor(utxo.Datum, 0, 1)
	if err != nil {
		return Observation{}, fmt.Errorf("%w: outer constructor: %v", ErrInvalidDatum, err)
	}
	_, priceFields, err := decodeRawConstructor(outerFields[0], 2, 1)
	if err != nil {
		return Observation{}, fmt.Errorf("%w: price constructor: %v", ErrInvalidDatum, err)
	}
	var values map[uint64]uint64
	if _, err := cbor.Decode(priceFields[0], &values); err != nil {
		return Observation{}, fmt.Errorf("%w: price map: %v", ErrInvalidDatum, err)
	}
	rawPrice, priceOK := values[charli3PriceKey]
	observedMillis, observedOK := values[charli3TimestampKey]
	expiryMillis, expiryOK := values[charli3ExpiryKey]
	if !priceOK || !observedOK || !expiryOK {
		return Observation{}, fmt.Errorf("%w: price map requires keys 0, 1, and 2", ErrInvalidDatum)
	}
	if rawPrice == 0 {
		return Observation{}, ErrInvalidPrice
	}
	observedAt, err := unixMillis(observedMillis)
	if err != nil {
		return Observation{}, err
	}
	expiresAt, err := unixMillis(expiryMillis)
	if err != nil || !expiresAt.After(observedAt) {
		return Observation{}, ErrInvalidTimestamps
	}

	return Observation{
		Source:      SourceCharli3,
		Pair:        PairADAUSD,
		FeedID:      "OracleFeed",
		Numerator:   rawPrice,
		Denominator: 1_000_000,
		ObservedAt:  observedAt,
		ExpiresAt:   expiresAt,
		TxHash:      utxo.TxHash,
		TxIndex:     utxo.TxIndex,
		Slot:        utxo.Slot,
		BlockTime:   utxo.BlockTime,
	}, nil
}
