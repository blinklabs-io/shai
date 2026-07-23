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

package djed

import (
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/shai/common"
	"github.com/stretchr/testify/require"
)

const currentMainnetDatum = "d8799f584004ea10278c7b8c3c636536a8a1b831d8e193e8aca7df1ee2b83fe856f1fede93fb818e3453f135f37a68d464bf3c6e38d1e4e4750d60cba6dbc3a96132aa6507d8799fd8799f1a000f42401a00029463ffd8799fd8799fd87a9f1b0000019f90e8fcc0ffd87a80ffd8799fd87a9f1b0000019f90f6b860ffd87a80ffff43555344ff581c815aca02042ba9188a2ca4f8ce7b276046e2376b4bce56391342299eff"

func TestParseMainnetObservationCurrentFixture(t *testing.T) {
	data := mustDecodeHex(t, currentMainnetDatum)
	utxo := currentMainnetUTxO(t)
	now := time.Unix(1_784_842_625, 0).UTC()

	observation, err := ParseMainnetObservation(data, utxo, now)
	require.NoError(t, err)
	require.Equal(t, "ADA/USD", observation.Pair)
	require.Equal(t, "djed", observation.Source)
	require.Equal(t, uint64(169_059), observation.PriceNumerator)
	require.Equal(t, uint64(1_000_000), observation.PriceDenominator)
	require.InDelta(t, 0.169059, observation.Price, 0.0000001)
	require.Equal(
		t,
		time.UnixMilli(1_784_842_616_000).UTC(),
		observation.ValidFrom,
	)
	require.True(t, observation.ValidFromInclusive)
	require.Equal(
		t,
		time.UnixMilli(1_784_843_516_000).UTC(),
		observation.ValidUntil,
	)
	require.True(t, observation.ValidUntilInclusive)
	require.Equal(
		t,
		"b8ec7d85670902edafdc73b1f2faa57e2aed867e3718851b2c1489ae7c0587d4",
		observation.TxHash,
	)
	require.Equal(t, uint32(0), observation.TxIndex)
}

func TestParseOfficialOpenDjedFixture(t *testing.T) {
	data := mustDecodeHex(
		t,
		"d8799f5840baf00a3eaa2919ef46bbdc67cfe6b50819a64781189d95317a8183c34bdce1cb32647a5bbe7950c97ec31c601064fbd255bb69a52d8b7c8b1f706e1aba3deb07d8799fd8799f19c350196ce7ffd8799fd8799fd87a9f1b0000019617d34560ffd87a80ffd8799fd87a9f1b0000019617e10100ffd87a80ffff43555344ff581c815aca02042ba9188a2ca4f8ce7b276046e2376b4bce56391342299eff",
	)
	datum, err := ParseOracleDatum(data)
	require.NoError(t, err)
	require.Equal(t, uint64(27_879), datum.PriceNumerator)
	require.Equal(t, uint64(50_000), datum.PriceDenominator)
	require.Equal(t, []byte("USD"), datum.ExpressedIn)
	require.Equal(
		t,
		time.UnixMilli(1_744_156_444_000).UTC(),
		datum.ValidFrom,
	)
	require.Equal(
		t,
		time.UnixMilli(1_744_157_344_000).UTC(),
		datum.ValidUntil,
	)
	require.True(t, datum.ValidFromInclusive)
	require.True(t, datum.ValidUntilInclusive)
}

func TestValidateMainnetRejectsInvalidCandidates(t *testing.T) {
	datum, err := ParseOracleDatum(mustDecodeHex(t, currentMainnetDatum))
	require.NoError(t, err)
	utxo := currentMainnetUTxO(t)
	validNow := time.Unix(1_784_842_625, 0).UTC()

	tests := []struct {
		name   string
		mutate func(*OracleDatum, *OracleUTxO, *time.Time)
		want   error
	}{
		{
			name: "wrong address",
			mutate: func(_ *OracleDatum, u *OracleUTxO, _ *time.Time) {
				u.Address = "addr1wrong"
			},
			want: ErrWrongAddress,
		},
		{
			name: "missing NFT",
			mutate: func(_ *OracleDatum, u *OracleUTxO, _ *time.Time) {
				u.Assets = nil
			},
			want: ErrMissingNFT,
		},
		{
			name: "NFT quantity",
			mutate: func(_ *OracleDatum, u *OracleUTxO, _ *time.Time) {
				u.Assets[0].Amount = 2
			},
			want: ErrMissingNFT,
		},
		{
			name: "signature length",
			mutate: func(d *OracleDatum, _ *OracleUTxO, _ *time.Time) {
				d.Signature = d.Signature[:63]
			},
			want: ErrInvalidDatum,
		},
		{
			name: "wrong policy",
			mutate: func(d *OracleDatum, _ *OracleUTxO, _ *time.Time) {
				d.OraclePolicy[0] ^= 0xff
			},
			want: ErrWrongPolicy,
		},
		{
			name: "wrong quote",
			mutate: func(d *OracleDatum, _ *OracleUTxO, _ *time.Time) {
				d.ExpressedIn = []byte("EUR")
			},
			want: ErrWrongQuote,
		},
		{
			name: "zero numerator",
			mutate: func(d *OracleDatum, _ *OracleUTxO, _ *time.Time) {
				d.PriceNumerator = 0
			},
			want: ErrInvalidRate,
		},
		{
			name: "zero denominator",
			mutate: func(d *OracleDatum, _ *OracleUTxO, _ *time.Time) {
				d.PriceDenominator = 0
			},
			want: ErrInvalidRate,
		},
		{
			name: "invalid interval",
			mutate: func(d *OracleDatum, _ *OracleUTxO, _ *time.Time) {
				d.ValidUntil = d.ValidFrom.Add(-time.Nanosecond)
			},
			want: ErrInvalidDatum,
		},
		{
			name: "exclusive lower endpoint",
			mutate: func(d *OracleDatum, _ *OracleUTxO, now *time.Time) {
				d.ValidFromInclusive = false
				*now = d.ValidFrom
			},
			want: ErrNotYetValid,
		},
		{
			name: "not yet valid",
			mutate: func(_ *OracleDatum, _ *OracleUTxO, now *time.Time) {
				*now = time.UnixMilli(1_784_842_615_999).UTC()
			},
			want: ErrNotYetValid,
		},
		{
			name: "exclusive upper endpoint",
			mutate: func(d *OracleDatum, _ *OracleUTxO, now *time.Time) {
				d.ValidUntilInclusive = false
				*now = d.ValidUntil
			},
			want: ErrExpired,
		},
		{
			name: "expired",
			mutate: func(d *OracleDatum, _ *OracleUTxO, now *time.Time) {
				*now = d.ValidUntil.Add(time.Nanosecond)
			},
			want: ErrExpired,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidateDatum := cloneDatum(datum)
			candidateUTxO := cloneUTxO(utxo)
			now := validNow
			test.mutate(&candidateDatum, &candidateUTxO, &now)
			err := candidateDatum.ValidateMainnet(candidateUTxO, now)
			require.ErrorIs(t, err, test.want)
		})
	}
}

func TestValidateMainnetAcceptsInclusiveEndpoints(t *testing.T) {
	datum, err := ParseOracleDatum(mustDecodeHex(t, currentMainnetDatum))
	require.NoError(t, err)
	utxo := currentMainnetUTxO(t)

	require.NoError(t, datum.ValidateMainnet(utxo, datum.ValidFrom))
	require.NoError(t, datum.ValidateMainnet(utxo, datum.ValidUntil))
}

func TestParseOracleDatumRejectsMalformedSchema(t *testing.T) {
	for _, data := range [][]byte{
		{0xff},
		mustEncode(t, cbor.NewConstructorEncoder(
			1,
			cbor.IndefLengthList{},
		)),
		mustEncode(t, cbor.NewConstructorEncoder(
			0,
			cbor.IndefLengthList{[]byte("short")},
		)),
	} {
		_, err := ParseOracleDatum(data)
		require.ErrorIs(t, err, ErrInvalidDatum)
	}
}

func TestPlutusBoolRejectsMalformedClosure(t *testing.T) {
	_, err := plutusBool(mustEncode(
		t,
		cbor.NewConstructorEncoder(2, cbor.IndefLengthList{}),
	))
	require.Error(t, err)

	_, err = plutusBool(mustEncode(
		t,
		cbor.NewConstructorEncoder(1, cbor.IndefLengthList{uint64(1)}),
	))
	require.Error(t, err)
}

func currentMainnetUTxO(t *testing.T) OracleUTxO {
	t.Helper()
	asset, err := common.NewAssetClass(MainnetOraclePolicy, OracleNFTName)
	require.NoError(t, err)
	return OracleUTxO{
		Address: MainnetOracleAddress,
		Assets: []common.AssetAmount{{
			Class:  asset,
			Amount: 1,
		}},
		TxHash: "b8ec7d85670902edafdc73b1f2faa57e2aed867e3718851b2c1489ae7c0587d4",
	}
}

func cloneDatum(datum OracleDatum) OracleDatum {
	datum.Signature = append([]byte(nil), datum.Signature...)
	datum.ExpressedIn = append([]byte(nil), datum.ExpressedIn...)
	datum.OraclePolicy = append([]byte(nil), datum.OraclePolicy...)
	return datum
}

func cloneUTxO(utxo OracleUTxO) OracleUTxO {
	utxo.Assets = append([]common.AssetAmount(nil), utxo.Assets...)
	return utxo
}

func mustDecodeHex(t *testing.T, value string) []byte {
	t.Helper()
	data, err := hex.DecodeString(value)
	require.NoError(t, err)
	return data
}

func mustEncode(t *testing.T, value any) []byte {
	t.Helper()
	data, err := cbor.Encode(value)
	require.NoError(t, err)
	return data
}

func TestRatRejectsInvalidRate(t *testing.T) {
	_, err := (OracleDatum{}).Rat()
	require.True(t, errors.Is(err, ErrInvalidRate))
}
