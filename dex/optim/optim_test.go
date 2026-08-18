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

package optim

import (
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// constructor builds a Plutus constructor datum with the given tag and fields,
// following the encoding pattern used by the other dex package tests.
func constructor(t *testing.T, tag uint, fields ...any) []byte {
	t.Helper()
	enc := cbor.NewConstructorEncoder(tag, cbor.IndefLengthList(fields))
	encoded, err := cbor.Encode(&enc)
	require.NoError(t, err, "encode constructor %d", tag)
	return encoded
}

// TestAddressRejectsUnsupportedConstructor covers a nested Address whose
// constructor is not the expected one. Returning nil there left a zero-valued
// Address that reads downstream as a real credential, so a bond would report an
// empty lender instead of failing to decode.
func TestAddressRejectsUnsupportedConstructor(t *testing.T) {
	var addr Address
	err := addr.UnmarshalCBOR(constructor(t, 1))
	require.Error(t, err, "unsupported address constructor must not decode to a zero Address")
	assert.Contains(t, err.Error(), "unsupported address constructor")
}

// TestRationalRejectsUnsupportedConstructor covers an exchange rate carried by
// an unrelated datum shape. ParseOADADatum uses the result directly, so
// accepting any two-field constructor would surface a fabricated rate.
func TestRationalRejectsUnsupportedConstructor(t *testing.T) {
	var r Rational
	err := r.UnmarshalCBOR(constructor(t, 2, uint64(1), uint64(2)))
	require.Error(t, err, "unsupported rational constructor must not decode")
	assert.Contains(t, err.Error(), "unsupported rational constructor")
}

// TestOADADatumRejectsUnsupportedConstructor is the root-cause case: silently
// decoding a wrong constructor to zeros is what forced ParseOADADatum to treat
// 0/0 as "not an OADA datum".
func TestOADADatumRejectsUnsupportedConstructor(t *testing.T) {
	var d OADADatum
	err := d.UnmarshalCBOR(constructor(t, 3, uint64(1), uint64(2), uint64(3), uint64(4)))
	require.Error(t, err, "unsupported OADA constructor must not decode to zeros")
	assert.Contains(t, err.Error(), "unsupported OADA datum constructor")
}

// TestParseOADADatumReturnsInitialZeroState covers a genuine initial OADA
// state: nothing staked and nothing minted. The old zero-totals sentinel made
// that state invisible until the first stake landed.
func TestParseOADADatumReturnsInitialZeroState(t *testing.T) {
	rate := cbor.NewConstructorEncoder(0, cbor.IndefLengthList{
		uint64(1), // Numerator
		uint64(1), // Denominator
	})
	datum := constructor(
		t,
		0,
		uint64(0),   // TotalStaked
		&rate,       // ExchangeRate
		uint64(500), // LastUpdateEpoch
		uint64(0),   // TotalOADA
	)
	state, err := NewParser().ParseOADADatum(datum, "txhash", 0, 42, time.Time{})
	require.NoError(t, err)
	require.NotNil(t, state, "an initial 0/0 OADA state must be reported, not dropped")
	assert.Equal(t, uint64(0), state.TotalStaked)
	assert.Equal(t, uint64(0), state.TotalOADA)
	assert.Equal(t, uint64(500), state.LastUpdateEpoch)
}

// bondDatumCBOR builds a full bond datum at the given status.
func bondDatumCBOR(t *testing.T, status uint64) []byte {
	t.Helper()
	payment := cbor.NewConstructorEncoder(0, cbor.IndefLengthList{
		[]byte{0x01, 0x02, 0x03},
	})
	stake := cbor.NewConstructorEncoder(1, cbor.IndefLengthList{})
	addr := cbor.NewConstructorEncoder(0, cbor.IndefLengthList{&payment, &stake})
	return constructor(
		t,
		0,
		[]byte{0xaa, 0xbb}, // BondNFT
		&addr,              // LenderAddress
		[]byte{0xcc, 0xdd}, // BorrowerNFT
		uint64(1_000_000),  // PrincipalAmount
		uint64(500),        // InterestRate
		uint64(10),         // Duration
		uint64(100),        // StartEpoch
		uint64(110),        // EndEpoch
		make([]byte, 28),   // StakePool
		uint64(0),          // AccruedRewards
		status,             // Status
	)
}

// TestBondCanClaimExcludesClaimedBonds covers the settled case: a claimed bond
// must not advertise itself as claimable forever. It goes through
// ParseBondDatum so it exercises the field the parser actually sets, rather
// than re-asserting the helper the parser happens to call.
func TestBondCanClaimExcludesClaimedBonds(t *testing.T) {
	for _, tc := range []struct {
		name     string
		status   uint64
		canClaim bool
	}{
		{"active", BondStatusActive, false},
		{"matured", BondStatusMatured, true},
		{"claimed", BondStatusClaimed, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state, err := NewParser().ParseBondDatum(
				bondDatumCBOR(t, tc.status), "txhash", 0, 42, time.Time{},
			)
			require.NoError(t, err)
			require.NotNil(t, state)
			assert.Equal(t, tc.canClaim, state.CanClaim,
				"status %d: CanClaim should be %v", tc.status, tc.canClaim)
		})
	}
}

// TestBondDatumRejectsUnsupportedConstructor is the same silent-zero hazard as
// Address and OADADatum: a wrong constructor previously decoded to an active
// bond with zero principal instead of failing.
func TestBondDatumRejectsUnsupportedConstructor(t *testing.T) {
	var d BondDatum
	err := d.UnmarshalCBOR(constructor(t, 5))
	require.Error(t, err, "unsupported bond constructor must not decode to zeros")
	assert.Contains(t, err.Error(), "unsupported bond datum constructor")
}

// TestGenerateBondIdUsesFullNFT covers BondState.Key() uniqueness: two bonds
// whose NFTs share a 16-byte prefix must not collide into one state entry.
func TestGenerateBondIdUsesFullNFT(t *testing.T) {
	prefix := make([]byte, 16)
	for i := range prefix {
		prefix[i] = byte(i)
	}
	first := append(append([]byte{}, prefix...), 0xaa, 0xbb)
	second := append(append([]byte{}, prefix...), 0xcc, 0xdd)

	idFirst := GenerateBondId(first)
	idSecond := GenerateBondId(second)

	assert.NotEqual(t, idFirst, idSecond,
		"bonds sharing a 16-byte NFT prefix must not share a bond ID")
	assert.True(t, strings.HasSuffix(idFirst, hex.EncodeToString(first)),
		"bond ID must carry the full NFT, got %q", idFirst)

	keyFirst := (&BondState{BondId: idFirst}).Key()
	keySecond := (&BondState{BondId: idSecond}).Key()
	assert.NotEqual(t, keyFirst, keySecond, "state keys must stay distinct")
}
