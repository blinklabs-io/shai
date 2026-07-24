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

// Package djed decodes and validates Open Djed on-chain oracle state.
package djed

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"math/big"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/shai/common"
)

const (
	MainnetOracleAddress = "addr1wxyc99q448xlkv4q2y3truxq7j2msr6hkqqg0wmzz9n9r6q8j7kpa"
	MainnetOraclePolicy  = "815aca02042ba9188a2ca4f8ce7b276046e2376b4bce56391342299e"
	OracleNFTName        = "446a65644f7261636c654e4654"
	QuoteCurrency        = "USD"

	oracleSignatureLength = 64
)

var (
	ErrInvalidDatum = errors.New("djed: invalid oracle datum")
	ErrInvalidRate  = errors.New("djed: invalid ADA/USD rate")
	ErrWrongAddress = errors.New("djed: wrong oracle address")
	ErrMissingNFT   = errors.New("djed: missing oracle NFT")
	ErrWrongPolicy  = errors.New("djed: wrong oracle policy")
	ErrWrongQuote   = errors.New("djed: wrong quote currency")
	ErrNotYetValid  = errors.New("djed: oracle value is not yet valid")
	ErrExpired      = errors.New("djed: oracle value is expired")
)

// OracleDatum is the authenticated price and validity interval stored with the
// Djed Oracle NFT.
type OracleDatum struct {
	Signature           []byte
	PriceNumerator      uint64
	PriceDenominator    uint64
	ValidFrom           time.Time
	ValidFromInclusive  bool
	ValidUntil          time.Time
	ValidUntilInclusive bool
	ExpressedIn         []byte
	OraclePolicy        []byte
}

// OracleUTxO contains the identity-bearing parts of the UTxO holding a datum.
type OracleUTxO struct {
	Address string
	Assets  []common.AssetAmount
	TxHash  string
	TxIndex uint32
}

// Observation is a validated local-ledger ADA/USD observation.
type Observation struct {
	Pair                string    `json:"pair"`
	Source              string    `json:"source"`
	PriceNumerator      uint64    `json:"priceNumerator"`
	PriceDenominator    uint64    `json:"priceDenominator"`
	Price               float64   `json:"price"`
	ValidFrom           time.Time `json:"validFrom"`
	ValidFromInclusive  bool      `json:"validFromInclusive"`
	ValidUntil          time.Time `json:"validUntil"`
	ValidUntilInclusive bool      `json:"validUntilInclusive"`
	TxHash              string    `json:"txHash"`
	TxIndex             uint32    `json:"txIndex"`
}

// ParseOracleDatum decodes the Open Djed OracleDatum schema.
func ParseOracleDatum(data []byte) (OracleDatum, error) {
	fields, err := constructorFields(data, 0, 3)
	if err != nil {
		return OracleDatum{}, fmt.Errorf("%w: %v", ErrInvalidDatum, err)
	}

	var datum OracleDatum
	if _, err := cbor.Decode(fields[0], &datum.Signature); err != nil {
		return OracleDatum{}, fmt.Errorf(
			"%w: signature: %v",
			ErrInvalidDatum,
			err,
		)
	}

	oracleFields, err := constructorFields(fields[1], 0, 3)
	if err != nil {
		return OracleDatum{}, fmt.Errorf(
			"%w: oracle fields: %v",
			ErrInvalidDatum,
			err,
		)
	}
	rateFields, err := constructorFields(oracleFields[0], 0, 2)
	if err != nil {
		return OracleDatum{}, fmt.Errorf(
			"%w: exchange rate: %v",
			ErrInvalidDatum,
			err,
		)
	}
	if _, err := cbor.Decode(
		rateFields[0],
		&datum.PriceDenominator,
	); err != nil {
		return OracleDatum{}, fmt.Errorf(
			"%w: rate denominator: %v",
			ErrInvalidDatum,
			err,
		)
	}
	if _, err := cbor.Decode(
		rateFields[1],
		&datum.PriceNumerator,
	); err != nil {
		return OracleDatum{}, fmt.Errorf(
			"%w: rate numerator: %v",
			ErrInvalidDatum,
			err,
		)
	}

	validityFields, err := constructorFields(oracleFields[1], 0, 2)
	if err != nil {
		return OracleDatum{}, fmt.Errorf(
			"%w: validity range: %v",
			ErrInvalidDatum,
			err,
		)
	}
	validFromMillis, validFromInclusive, err := finiteBound(
		validityFields[0],
	)
	if err != nil {
		return OracleDatum{}, fmt.Errorf(
			"%w: lower bound: %v",
			ErrInvalidDatum,
			err,
		)
	}
	validUntilMillis, validUntilInclusive, err := finiteBound(
		validityFields[1],
	)
	if err != nil {
		return OracleDatum{}, fmt.Errorf(
			"%w: upper bound: %v",
			ErrInvalidDatum,
			err,
		)
	}
	datum.ValidFrom = time.UnixMilli(validFromMillis).UTC()
	datum.ValidFromInclusive = validFromInclusive
	datum.ValidUntil = time.UnixMilli(validUntilMillis).UTC()
	datum.ValidUntilInclusive = validUntilInclusive

	if _, err := cbor.Decode(
		oracleFields[2],
		&datum.ExpressedIn,
	); err != nil {
		return OracleDatum{}, fmt.Errorf(
			"%w: quote currency: %v",
			ErrInvalidDatum,
			err,
		)
	}
	if _, err := cbor.Decode(fields[2], &datum.OraclePolicy); err != nil {
		return OracleDatum{}, fmt.Errorf(
			"%w: oracle policy: %v",
			ErrInvalidDatum,
			err,
		)
	}
	return datum, nil
}

// ValidateMainnet authenticates a decoded datum against the mainnet deployment
// and checks that its rate is currently usable.
func (d OracleDatum) ValidateMainnet(
	utxo OracleUTxO,
	now time.Time,
) error {
	if utxo.Address != MainnetOracleAddress {
		return ErrWrongAddress
	}
	oracleAsset, err := common.NewAssetClass(
		MainnetOraclePolicy,
		OracleNFTName,
	)
	if err != nil {
		return fmt.Errorf("djed: invalid built-in oracle identity: %w", err)
	}
	hasNFT := false
	for _, asset := range utxo.Assets {
		if asset.IsAsset(oracleAsset) && asset.Amount == 1 {
			hasNFT = true
			break
		}
	}
	if !hasNFT {
		return ErrMissingNFT
	}
	if len(d.Signature) != oracleSignatureLength {
		return fmt.Errorf(
			"%w: signature must be %d bytes",
			ErrInvalidDatum,
			oracleSignatureLength,
		)
	}
	if !bytes.Equal(d.OraclePolicy, oracleAsset.PolicyId) {
		return ErrWrongPolicy
	}
	if string(d.ExpressedIn) != QuoteCurrency {
		return ErrWrongQuote
	}
	if d.PriceNumerator == 0 || d.PriceDenominator == 0 {
		return ErrInvalidRate
	}
	if d.ValidFrom.After(d.ValidUntil) ||
		(d.ValidFrom.Equal(d.ValidUntil) &&
			(!d.ValidFromInclusive || !d.ValidUntilInclusive)) {
		return fmt.Errorf("%w: invalid validity interval", ErrInvalidDatum)
	}
	now = now.UTC()
	if now.Before(d.ValidFrom) ||
		(now.Equal(d.ValidFrom) && !d.ValidFromInclusive) {
		return ErrNotYetValid
	}
	if now.After(d.ValidUntil) ||
		(now.Equal(d.ValidUntil) && !d.ValidUntilInclusive) {
		return ErrExpired
	}
	return nil
}

// Rat returns the exact ADA/USD exchange rate.
func (d OracleDatum) Rat() (*big.Rat, error) {
	if d.PriceNumerator == 0 || d.PriceDenominator == 0 {
		return nil, ErrInvalidRate
	}
	return new(big.Rat).SetFrac(
		new(big.Int).SetUint64(d.PriceNumerator),
		new(big.Int).SetUint64(d.PriceDenominator),
	), nil
}

// ParseMainnetObservation decodes and validates a mainnet Djed observation.
func ParseMainnetObservation(
	data []byte,
	utxo OracleUTxO,
	now time.Time,
) (Observation, error) {
	datum, err := ParseOracleDatum(data)
	if err != nil {
		return Observation{}, err
	}
	if err := datum.ValidateMainnet(utxo, now); err != nil {
		return Observation{}, err
	}
	rate, err := datum.Rat()
	if err != nil {
		return Observation{}, err
	}
	price, _ := rate.Float64()
	return Observation{
		Pair:                "ADA/USD",
		Source:              "djed",
		PriceNumerator:      datum.PriceNumerator,
		PriceDenominator:    datum.PriceDenominator,
		Price:               price,
		ValidFrom:           datum.ValidFrom,
		ValidFromInclusive:  datum.ValidFromInclusive,
		ValidUntil:          datum.ValidUntil,
		ValidUntilInclusive: datum.ValidUntilInclusive,
		TxHash:              utxo.TxHash,
		TxIndex:             utxo.TxIndex,
	}, nil
}

func constructorFields(
	data []byte,
	expectedTag uint,
	expectedCount int,
) ([]cbor.RawMessage, error) {
	var constructor cbor.ConstructorDecoder
	if _, err := cbor.Decode(data, &constructor); err != nil {
		return nil, err
	}
	if constructor.Tag() != expectedTag {
		return nil, fmt.Errorf(
			"expected constructor %d, got %d",
			expectedTag,
			constructor.Tag(),
		)
	}
	var fields []cbor.RawMessage
	if _, err := cbor.Decode(constructor.Fields(), &fields); err != nil {
		return nil, err
	}
	if len(fields) != expectedCount {
		return nil, fmt.Errorf(
			"expected %d fields, got %d",
			expectedCount,
			len(fields),
		)
	}
	return fields, nil
}

func finiteBound(data []byte) (int64, bool, error) {
	boundFields, err := constructorFields(data, 0, 2)
	if err != nil {
		return 0, false, err
	}
	valueFields, err := constructorFields(boundFields[0], 1, 1)
	if err != nil {
		return 0, false, err
	}
	var millis uint64
	if _, err := cbor.Decode(valueFields[0], &millis); err != nil {
		return 0, false, err
	}
	if millis > math.MaxInt64 {
		return 0, false, fmt.Errorf("timestamp overflows int64")
	}
	inclusive, err := plutusBool(boundFields[1])
	if err != nil {
		return 0, false, fmt.Errorf("closure: %w", err)
	}
	return int64(millis), inclusive, nil
}

func plutusBool(data []byte) (bool, error) {
	var constructor cbor.ConstructorDecoder
	if _, err := cbor.Decode(data, &constructor); err != nil {
		return false, err
	}
	if constructor.Tag() > 1 {
		return false, fmt.Errorf(
			"expected boolean constructor 0 or 1, got %d",
			constructor.Tag(),
		)
	}
	var fields []cbor.RawMessage
	if _, err := cbor.Decode(constructor.Fields(), &fields); err != nil {
		return false, err
	}
	if len(fields) != 0 {
		return false, fmt.Errorf(
			"expected boolean with no fields, got %d",
			len(fields),
		)
	}
	return constructor.Tag() == 1, nil
}
