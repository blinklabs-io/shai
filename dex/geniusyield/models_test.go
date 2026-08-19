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
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
)

func TestOptionalPOSIXUnmarshalCBORValidTags(t *testing.T) {
	present := cbor.NewConstructorEncoder(
		0,
		cbor.IndefLengthList{int64(123456)},
	)
	presentBytes, err := cbor.Encode(&present)
	if err != nil {
		t.Fatalf("failed to encode present POSIX: %v", err)
	}

	var got OptionalPOSIX
	if _, err := cbor.Decode(presentBytes, &got); err != nil {
		t.Fatalf("failed to decode present POSIX: %v", err)
	}
	if !got.IsPresent {
		t.Fatal("expected present POSIX")
	}
	if got.Time != 123456 {
		t.Fatalf("expected time 123456, got %d", got.Time)
	}

	none := cbor.NewConstructorEncoder(1, cbor.IndefLengthList{})
	noneBytes, err := cbor.Encode(&none)
	if err != nil {
		t.Fatalf("failed to encode none POSIX: %v", err)
	}

	got = OptionalPOSIX{IsPresent: true, Time: 1}
	if _, err := cbor.Decode(noneBytes, &got); err != nil {
		t.Fatalf("failed to decode none POSIX: %v", err)
	}
	if got.IsPresent {
		t.Fatal("expected none POSIX")
	}
	if got.Time != 0 {
		t.Fatalf("expected reset time 0, got %d", got.Time)
	}
}

func TestOptionalPOSIXUnmarshalCBORRejectsInvalidTag(t *testing.T) {
	invalid := cbor.NewConstructorEncoder(2, cbor.IndefLengthList{})
	invalidBytes, err := cbor.Encode(&invalid)
	if err != nil {
		t.Fatalf("failed to encode invalid POSIX: %v", err)
	}

	var got OptionalPOSIX
	if _, err := cbor.Decode(invalidBytes, &got); err == nil {
		t.Fatal("expected invalid tag error")
	}
}

func TestOrderCredentialUnmarshalCBORRejectsInvalidTag(t *testing.T) {
	invalid := cbor.NewConstructorEncoder(
		2,
		cbor.IndefLengthList{[]byte{0x01}},
	)
	invalidBytes, err := cbor.Encode(&invalid)
	if err != nil {
		t.Fatalf("failed to encode invalid credential: %v", err)
	}

	var got OrderCredential
	if _, err := cbor.Decode(invalidBytes, &got); err == nil {
		t.Fatal("expected invalid credential tag error")
	}
}

func TestOptionalCredentialUnmarshalCBORValidatesTags(t *testing.T) {
	present := cbor.NewConstructorEncoder(
		0,
		cbor.IndefLengthList{
			cbor.NewConstructorEncoder(
				0,
				cbor.IndefLengthList{[]byte{0x01, 0x02}},
			),
		},
	)
	presentBytes, err := cbor.Encode(&present)
	if err != nil {
		t.Fatalf("failed to encode present credential: %v", err)
	}

	var got OptionalCredential
	if _, err := cbor.Decode(presentBytes, &got); err != nil {
		t.Fatalf("failed to decode present credential: %v", err)
	}
	if !got.IsPresent || got.Credential == nil {
		t.Fatal("expected present credential")
	}

	absent := cbor.NewConstructorEncoder(1, cbor.IndefLengthList{})
	absentBytes, err := cbor.Encode(&absent)
	if err != nil {
		t.Fatalf("failed to encode absent credential: %v", err)
	}

	got = OptionalCredential{
		IsPresent:  true,
		Credential: &OrderCredential{Hash: []byte{0xff}},
	}
	if _, err := cbor.Decode(absentBytes, &got); err != nil {
		t.Fatalf("failed to decode absent credential: %v", err)
	}
	if got.IsPresent || got.Credential != nil {
		t.Fatal("expected absent credential to reset state")
	}

	invalid := cbor.NewConstructorEncoder(2, cbor.IndefLengthList{})
	invalidBytes, err := cbor.Encode(&invalid)
	if err != nil {
		t.Fatalf("failed to encode invalid credential: %v", err)
	}
	if _, err := cbor.Decode(invalidBytes, &got); err == nil {
		t.Fatal("expected invalid optional credential tag error")
	}
}

func TestOrderRationalUnmarshalCBORRejectsZeroDenominator(t *testing.T) {
	invalid := cbor.NewConstructorEncoder(
		0,
		cbor.IndefLengthList{int64(1), int64(0)},
	)
	invalidBytes, err := cbor.Encode(&invalid)
	if err != nil {
		t.Fatalf("failed to encode invalid rational: %v", err)
	}

	var got OrderRational
	if _, err := cbor.Decode(invalidBytes, &got); err == nil {
		t.Fatal("expected zero denominator error")
	}
}

func TestOrderConfigToStatePreservesPartialFillFields(t *testing.T) {
	cfg := &OrderConfig{
		OwnerKey: []byte{0x01, 0x02},
		OwnerAddr: OrderAddress{
			PaymentCredential: OrderCredential{
				Type: 0,
				Hash: []byte{0x03, 0x04},
			},
		},
		OfferedOriginalAmount: 1000,
		OfferedAmount:         800,
		Price: OrderRational{
			Numerator:   2,
			Denominator: 1,
		},
		NFT:                  []byte("order-nft"),
		PartialFills:         3,
		MakerLovelaceFlatFee: 100,
		MakerOfferedPercentFee: OrderRational{
			Numerator:   1,
			Denominator: 100,
		},
		MakerOfferedPercentFeeMax: 50,
		ContainedFee: ContainedFee{
			LovelaceFee: 1,
			OfferedFee:  2,
			AskedFee:    3,
		},
		ContainedPayment: 4,
	}

	state := OrderConfigToState(cfg, "tx", 1, 2)

	if string(state.NFT) != "order-nft" {
		t.Fatalf("unexpected NFT: %q", state.NFT)
	}
	if string(state.OwnerAddr.PaymentCredential.Hash) != "\x03\x04" {
		t.Fatalf("unexpected owner address: %+v", state.OwnerAddr)
	}
	if state.MakerLovelaceFlatFee != 100 {
		t.Fatalf("unexpected flat fee: %d", state.MakerLovelaceFlatFee)
	}
	if state.MakerFeeNum != 1 || state.MakerFeeDenom != 100 {
		t.Fatalf(
			"unexpected maker fee rational: %d/%d",
			state.MakerFeeNum,
			state.MakerFeeDenom,
		)
	}
	if state.MakerFeeMax != 50 {
		t.Fatalf("unexpected maker fee max: %d", state.MakerFeeMax)
	}
	if state.ContainedLovelaceFee != 1 ||
		state.ContainedOfferedFee != 2 ||
		state.ContainedAskedFee != 3 {
		t.Fatalf("unexpected contained fee fields: %+v", state)
	}
	if state.ContainedPayment != 4 {
		t.Fatalf("unexpected contained payment: %d", state.ContainedPayment)
	}
}
