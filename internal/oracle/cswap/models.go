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

// Package cswap provides datum types and parsing for the CSWAP DEX protocol.
package cswap

import (
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/shai/internal/common"
)

const (
	ProtocolName = "cswap"
	FeeDenom     = 10000
)

// PoolDatum is the on-chain pool datum defined in the Cardano datum registry.
type PoolDatum struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	TotalLpTokens uint64
	PoolFee       uint64
	QuotePolicy   []byte
	QuoteName     []byte
	BasePolicy    []byte
	BaseName      []byte
	LPTokenPolicy []byte
	LPTokenName   []byte
}

func (d *PoolDatum) UnmarshalCBOR(cborData []byte) error {
	var tmpConstr cbor.Constructor
	if _, err := cbor.Decode(cborData, &tmpConstr); err != nil {
		return err
	}
	if tmpConstr.Constructor() != 0 {
		return fmt.Errorf(
			"expected constructor 0, got %d",
			tmpConstr.Constructor(),
		)
	}

	type tPoolDatum PoolDatum
	var tmp tPoolDatum
	if _, err := cbor.Decode(tmpConstr.FieldsCbor(), &tmp); err != nil {
		return err
	}
	*d = PoolDatum(tmp)
	d.SetCbor(cborData)
	return nil
}

func (d PoolDatum) QuoteAsset() common.AssetClass {
	return common.AssetClass{
		PolicyId: d.QuotePolicy,
		Name:     d.QuoteName,
	}
}

func (d PoolDatum) BaseAsset() common.AssetClass {
	return common.AssetClass{
		PolicyId: d.BasePolicy,
		Name:     d.BaseName,
	}
}
