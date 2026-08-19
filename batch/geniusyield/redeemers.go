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

package geniusyield

import (
	"github.com/blinklabs-io/gouroboros/cbor"
)

// PartialFillRedeemer is the redeemer for partially filling an order
type PartialFillRedeemer struct {
	cbor.StructAsArray
	FillAmount uint64 // Amount of offered asset being taken
	// Note: Full structure depends on Genius Yield contract version
}

// MarshalCBOR encodes the redeemer
func (r *PartialFillRedeemer) MarshalCBOR() ([]byte, error) {
	// Constructor 0 = PartialFill
	tmpConstr := cbor.NewConstructorEncoder(
		0,
		cbor.IndefLengthList{
			r.FillAmount,
		},
	)
	return cbor.Encode(&tmpConstr)
}

// CompleteFillRedeemer is the redeemer for completely filling an order
type CompleteFillRedeemer struct {
	cbor.StructAsArray
}

// MarshalCBOR encodes the redeemer
func (r *CompleteFillRedeemer) MarshalCBOR() ([]byte, error) {
	// Constructor 1 = CompleteFill
	tmpConstr := cbor.NewConstructorEncoder(1, cbor.IndefLengthList{})
	return cbor.Encode(&tmpConstr)
}
