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

// Package oraclefeed parses authenticated on-chain oracle feed UTxOs.
//
// The package is deliberately independent of a chain provider. Callers obtain
// UTxOs from their local ledger/index and pass them to a Parser. This keeps
// network access and trust decisions outside the datum decoder.
package oraclefeed

import (
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"
)

const (
	PairADAUSD = "ADA/USD"

	SourceOrcfax  = "orcfax"
	SourceCharli3 = "charli3"

	OrcfaxADAUSDAddress = "addr1wyvnaejjzxanknsw5hm4raq4y6f4tfjsut3hqmmztn035jc4rpcfn"
	OrcfaxFeedPolicyID  = "193ee65211bb3b4e0ea5f751f415269355a650e2e3706f625cdf1a4b"
	OrcfaxFeedAssetName = ""

	Charli3ADAUSDAddress = "addr1wyvxns52tsgz8ggvrh4np5gjyfk0g5fshqq2ytvu9t7pe8qp3adw6"
	Charli3FeedPolicyID  = "08c56c0fa73748a23c3bc1d9e6a60a4187416fc4ff8fe3475506990e"
	Charli3FeedAssetName = "4f7261636c6546656564"

	// The documented Charli3 ADA/USD deployment publishes six decimal places.
	// Its current datum carries price, timestamp, and expiry but omits an
	// explicit precision field, so precision is part of the deployment profile.
	Charli3ADAUSDPrecision = 6
)

var (
	ErrWrongAddress      = errors.New("oraclefeed: wrong feed address")
	ErrMissingAuthAsset  = errors.New("oraclefeed: authentication asset missing")
	ErrInvalidDatum      = errors.New("oraclefeed: invalid datum")
	ErrUnexpectedFeed    = errors.New("oraclefeed: unexpected feed")
	ErrInvalidPrice      = errors.New("oraclefeed: invalid price")
	ErrInvalidTimestamps = errors.New("oraclefeed: invalid timestamps")
)

// Asset is one native asset in an oracle UTxO.
type Asset struct {
	PolicyID string
	Name     string
	Quantity uint64
}

// UTxO is the chain-independent input accepted by a feed parser.
type UTxO struct {
	Address   string
	Assets    []Asset
	Datum     []byte
	TxHash    string
	TxIndex   uint32
	Slot      uint64
	BlockTime time.Time
}

// Observation is an authenticated price decoded from an oracle UTxO.
// Numerator/Denominator preserve the exact on-chain rational.
type Observation struct {
	Source      string    `json:"source"`
	Pair        string    `json:"pair"`
	FeedID      string    `json:"feedId"`
	Numerator   uint64    `json:"numerator"`
	Denominator uint64    `json:"denominator"`
	ObservedAt  time.Time `json:"observedAt"`
	ExpiresAt   time.Time `json:"expiresAt,omitempty"`
	TxHash      string    `json:"txHash"`
	TxIndex     uint32    `json:"txIndex"`
	Slot        uint64    `json:"slot"`
	BlockTime   time.Time `json:"blockTime,omitempty"`
}

// Rat returns the exact price as a rational number.
func (o Observation) Rat() *big.Rat {
	return new(big.Rat).SetFrac(
		new(big.Int).SetUint64(o.Numerator),
		new(big.Int).SetUint64(o.Denominator),
	)
}

// Float64 returns the nearest float64 representation of the price.
func (o Observation) Float64() float64 {
	value, _ := o.Rat().Float64()
	return value
}

// FreshAt applies both the feed's explicit expiry, when present, and the
// caller's maximum acceptable observation age.
func (o Observation) FreshAt(now time.Time, maxAge time.Duration) bool {
	if o.ObservedAt.IsZero() || now.Before(o.ObservedAt) {
		return false
	}
	if !o.ExpiresAt.IsZero() && !now.Before(o.ExpiresAt) {
		return false
	}
	return maxAge > 0 && now.Sub(o.ObservedAt) <= maxAge
}

// Parser authenticates and decodes one oracle deployment.
type Parser interface {
	Source() string
	Pair() string
	Address() string
	Parse(UTxO) (Observation, error)
}

func authenticate(utxo UTxO, address, policyID, assetName string) error {
	if utxo.Address != address {
		return fmt.Errorf("%w: got %q", ErrWrongAddress, utxo.Address)
	}
	for _, asset := range utxo.Assets {
		if strings.EqualFold(asset.PolicyID, policyID) &&
			strings.EqualFold(asset.Name, assetName) &&
			asset.Quantity == 1 {
			return nil
		}
	}
	return fmt.Errorf(
		"%w: policy=%s name=%s quantity=1",
		ErrMissingAuthAsset,
		policyID,
		assetName,
	)
}
