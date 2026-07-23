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
	"fmt"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
)

const orcfaxADAUSDFeedPrefix = "CER/ADA-USD/"

// OrcfaxADAUSDParser parses the current Orcfax mainnet ADA/USD deployment.
type OrcfaxADAUSDParser struct{}

func (OrcfaxADAUSDParser) Source() string  { return SourceOrcfax }
func (OrcfaxADAUSDParser) Pair() string    { return PairADAUSD }
func (OrcfaxADAUSDParser) Address() string { return OrcfaxADAUSDAddress }

func (OrcfaxADAUSDParser) Parse(utxo UTxO) (Observation, error) {
	if err := authenticate(
		utxo,
		OrcfaxADAUSDAddress,
		OrcfaxFeedPolicyID,
		OrcfaxFeedAssetName,
	); err != nil {
		return Observation{}, err
	}

	outer, fields, err := decodeConstructor(utxo.Datum, 0, 2)
	if err != nil {
		return Observation{}, fmt.Errorf("%w: outer constructor: %v", ErrInvalidDatum, err)
	}
	_ = outer

	_, statementFields, err := decodeRawConstructor(fields[0], 0, 3)
	if err != nil {
		return Observation{}, fmt.Errorf("%w: statement constructor: %v", ErrInvalidDatum, err)
	}
	var feedID []byte
	if _, err := cbor.Decode(statementFields[0], &feedID); err != nil {
		return Observation{}, fmt.Errorf("%w: feed id: %v", ErrInvalidDatum, err)
	}
	if !bytes.HasPrefix(feedID, []byte(orcfaxADAUSDFeedPrefix)) {
		return Observation{}, fmt.Errorf("%w: %q", ErrUnexpectedFeed, feedID)
	}

	var createdAtMillis uint64
	if _, err := cbor.Decode(statementFields[1], &createdAtMillis); err != nil {
		return Observation{}, fmt.Errorf("%w: created_at: %v", ErrInvalidDatum, err)
	}

	_, rationalFields, err := decodeRawConstructor(statementFields[2], 0, 2)
	if err != nil {
		return Observation{}, fmt.Errorf("%w: rational constructor: %v", ErrInvalidDatum, err)
	}
	var numerator, denominator uint64
	if _, err := cbor.Decode(rationalFields[0], &numerator); err != nil {
		return Observation{}, fmt.Errorf("%w: numerator: %v", ErrInvalidDatum, err)
	}
	if _, err := cbor.Decode(rationalFields[1], &denominator); err != nil {
		return Observation{}, fmt.Errorf("%w: denominator: %v", ErrInvalidDatum, err)
	}
	if numerator == 0 || denominator == 0 {
		return Observation{}, ErrInvalidPrice
	}

	observedAt, err := unixMillis(createdAtMillis)
	if err != nil {
		return Observation{}, err
	}
	return Observation{
		Source:      SourceOrcfax,
		Pair:        PairADAUSD,
		FeedID:      string(feedID),
		Numerator:   numerator,
		Denominator: denominator,
		ObservedAt:  observedAt,
		TxHash:      utxo.TxHash,
		TxIndex:     utxo.TxIndex,
		Slot:        utxo.Slot,
		BlockTime:   utxo.BlockTime,
	}, nil
}

func decodeConstructor(data []byte, wantTag uint, wantFields int) (cbor.ConstructorDecoder, []cbor.RawMessage, error) {
	var constructor cbor.ConstructorDecoder
	if _, err := cbor.Decode(data, &constructor); err != nil {
		return constructor, nil, err
	}
	fields, err := constructorFields(constructor, wantTag, wantFields)
	return constructor, fields, err
}

func decodeRawConstructor(data cbor.RawMessage, wantTag uint, wantFields int) (cbor.ConstructorDecoder, []cbor.RawMessage, error) {
	return decodeConstructor([]byte(data), wantTag, wantFields)
}

func constructorFields(constructor cbor.ConstructorDecoder, wantTag uint, wantFields int) ([]cbor.RawMessage, error) {
	if constructor.Tag() != wantTag {
		return nil, fmt.Errorf("constructor=%d, want %d", constructor.Tag(), wantTag)
	}
	var fields []cbor.RawMessage
	if err := constructor.DecodeFields(&fields); err != nil {
		return nil, err
	}
	if len(fields) != wantFields {
		return nil, fmt.Errorf("fields=%d, want %d", len(fields), wantFields)
	}
	return fields, nil
}

func unixMillis(value uint64) (time.Time, error) {
	const maxInt64 = uint64(^uint64(0) >> 1)
	if value > maxInt64 {
		return time.Time{}, ErrInvalidTimestamps
	}
	return time.UnixMilli(int64(value)).UTC(), nil
}
