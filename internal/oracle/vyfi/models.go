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

// Package vyfi provides datum types and parsing for VyFi DEX protocol.
package vyfi

import (
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
)

// Protocol constants
const (
	ProtocolName = "vyfi"
	FeeDenom     = 1000
)

// PoolDatum represents the VyFi pool datum structure
// Based on CDDL schema from cardano-datum-registry:
// Pool = #6.121([treasuryA: int, treasuryB: int, issuedShares: int])
type PoolDatum struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	TreasuryA    uint64 // Reserve/treasury for asset A
	TreasuryB    uint64 // Reserve/treasury for asset B
	IssuedShares uint64 // Total LP shares issued
}

func (p *PoolDatum) UnmarshalCBOR(cborData []byte) error {
	p.SetCbor(cborData)
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
	return cbor.DecodeGeneric(tmpConstr.FieldsCbor(), p)
}

func (p PoolDatum) String() string {
	return fmt.Sprintf(
		"PoolDatum< treasuryA = %d, treasuryB = %d, issuedShares = %d >",
		p.TreasuryA,
		p.TreasuryB,
		p.IssuedShares,
	)
}

// OrderDatum represents a VyFi order datum
// Based on CDDL schema:
// Order = #6.121([owner: bytes, orderDetails: OrderDetails])
type OrderDatum struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	Owner        []byte
	OrderDetails OrderDetails
}

func (d *OrderDatum) UnmarshalCBOR(cborData []byte) error {
	d.SetCbor(cborData)
	var tmpConstr cbor.Constructor
	if _, err := cbor.Decode(cborData, &tmpConstr); err != nil {
		return err
	}
	if tmpConstr.Constructor() != 0 {
		return fmt.Errorf(
			"expected constructor 0 for Order, got %d",
			tmpConstr.Constructor(),
		)
	}
	return cbor.DecodeGeneric(tmpConstr.FieldsCbor(), d)
}

// OrderDetails represents the order type and parameters
// This is a sum type with multiple constructors:
// - AddLiquidity (#6.121): minWantedShares
// - RemoveLiquidity (#6.122): RemoveLiquidityDetails
// - Serve (#6.123): empty
// - TradeAToB (#6.124): minWantedTokens
// - TradeBToA (#6.125): minWantedTokens
// - ZapInA (#6.126): minWantedShares
// - ZapInB (#6.127): minWantedShares
type OrderDetails struct {
	Type OrderType

	// For AddLiquidity, ZapInA, ZapInB
	MinWantedShares uint64

	// For TradeAToB, TradeBToA
	MinWantedTokens uint64

	// For RemoveLiquidity
	MinWantedTokensA uint64
	MinWantedTokensB uint64
}

// OrderType represents the type of order
type OrderType int

const (
	OrderTypeAddLiquidity    OrderType = iota // Constructor 0 (#6.121)
	OrderTypeRemoveLiquidity                  // Constructor 1 (#6.122)
	OrderTypeServe                            // Constructor 2 (#6.123)
	OrderTypeTradeAToB                        // Constructor 3 (#6.124)
	OrderTypeTradeBToA                        // Constructor 4 (#6.125)
	OrderTypeZapInA                           // Constructor 5 (#6.126)
	OrderTypeZapInB                           // Constructor 6 (#6.127)
)

func (o *OrderDetails) UnmarshalCBOR(cborData []byte) error {
	var tmpConstr cbor.Constructor
	if _, err := cbor.Decode(cborData, &tmpConstr); err != nil {
		return err
	}

	// Map CBOR constructor tag to order type
	// CDDL uses #6.121-#6.127, which map to constructor indices 0-6
	o.Type = OrderType(tmpConstr.Constructor())

	switch o.Type {
	case OrderTypeAddLiquidity, OrderTypeZapInA, OrderTypeZapInB:
		// These have minWantedShares
		var wrapper struct {
			cbor.StructAsArray
			MinWantedShares uint64
		}
		if err := cbor.DecodeGeneric(tmpConstr.FieldsCbor(), &wrapper); err != nil {
			return err
		}
		o.MinWantedShares = wrapper.MinWantedShares

	case OrderTypeRemoveLiquidity:
		// RemoveLiquidityDetails = #6.122([minWantedTokensA: int, minWantedTokensB: int])
		var innerConstr cbor.Constructor
		if _, err := cbor.Decode(tmpConstr.FieldsCbor(), &innerConstr); err != nil {
			return err
		}
		var wrapper struct {
			cbor.StructAsArray
			MinWantedTokensA uint64
			MinWantedTokensB uint64
		}
		if err := cbor.DecodeGeneric(innerConstr.FieldsCbor(), &wrapper); err != nil {
			return err
		}
		o.MinWantedTokensA = wrapper.MinWantedTokensA
		o.MinWantedTokensB = wrapper.MinWantedTokensB

	case OrderTypeServe:
		// No fields

	case OrderTypeTradeAToB, OrderTypeTradeBToA:
		// These have minWantedTokens
		var wrapper struct {
			cbor.StructAsArray
			MinWantedTokens uint64
		}
		if err := cbor.DecodeGeneric(tmpConstr.FieldsCbor(), &wrapper); err != nil {
			return err
		}
		o.MinWantedTokens = wrapper.MinWantedTokens

	default:
		return fmt.Errorf("unknown order type constructor: %d", tmpConstr.Constructor())
	}

	return nil
}

// IsSwap returns true if this is a swap order
func (o *OrderDetails) IsSwap() bool {
	return o.Type == OrderTypeTradeAToB || o.Type == OrderTypeTradeBToA
}

// IsLiquidity returns true if this is a liquidity order
func (o *OrderDetails) IsLiquidity() bool {
	return o.Type == OrderTypeAddLiquidity ||
		o.Type == OrderTypeRemoveLiquidity ||
		o.Type == OrderTypeZapInA ||
		o.Type == OrderTypeZapInB
}
