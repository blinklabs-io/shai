// Copyright 2025 Blink Labs Software
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

package spectrum_test

import (
	"testing"

	"github.com/blinklabs-io/shai/internal/spectrum"
)

// adaClass is the ADA / lovelace asset class (empty policy id and name).
var adaClass = spectrum.AssetClass{
	PolicyId: []byte{},
	Name:     []byte{},
}

// tokenClass is a non-ADA asset class used for the Y side of test pools.
var tokenClass = spectrum.AssetClass{
	PolicyId: testModelsDecodeHex(
		"f66d78b4a3cb3d37afa0ec36461e51ecbde00f26c8f0a68f94b69880",
	),
	Name: []byte("iBTC"),
}

var lqClass = spectrum.AssetClass{
	PolicyId: testModelsDecodeHex(
		"475362a850bf8d1f037794432cdea9fdbbf8d048a7c5115feeb7e91d",
	),
	Name: []byte("ibtc_ADA_LQ"),
}

var nftClass = spectrum.AssetClass{
	PolicyId: testModelsDecodeHex(
		"d8beceb1ac736c92df8e1210fb39803508533ae9573cffeb2b24a839",
	),
	Name: []byte("ibtc_ADA_NFT"),
}

// newTestPool builds a Pool with ADA on the X side and a token on the Y side.
func newTestPool(adaReserve, tokenReserve uint64) *spectrum.Pool {
	return &spectrum.Pool{
		Id: nftClass,
		X: spectrum.AssetAmount{
			Class:  adaClass,
			Amount: adaReserve,
		},
		Y: spectrum.AssetAmount{
			Class:  tokenClass,
			Amount: tokenReserve,
		},
		Lq: spectrum.AssetAmount{
			Class:  lqClass,
			Amount: 1_000_000,
		},
		FeeNum: 997,
	}
}

// TestCalculateReturnToPoolHappyPath verifies the existing correct behavior is
// preserved: a token-for-ADA swap returns the reduced ADA reserve plus the
// increased token reserve, with a nil error.
func TestCalculateReturnToPoolHappyPath(t *testing.T) {
	adaReserve := uint64(15_000_000_000)
	tokenReserve := uint64(150_000)
	pool := newTestPool(adaReserve, tokenReserve)

	// Trader sends tokens in (input), receives ADA out (reward).
	inputAsset := spectrum.AssetAmount{Class: tokenClass, Amount: 100}
	rewardAsset := spectrum.AssetAmount{Class: adaClass, Amount: 9_000_000}

	retAda, retUnits, err := pool.CalculateReturnToPool(
		inputAsset,
		rewardAsset,
	)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	// ADA reserve is the output asset, reduced by the reward amount.
	wantAda := adaReserve - rewardAsset.Amount
	if retAda != wantAda {
		t.Fatalf("expected retAda %d, got %d", wantAda, retAda)
	}
	// retUnits should always lead with the pool NFT and the LQ token.
	if len(retUnits) < 3 {
		t.Fatalf("expected at least 3 return units, got %d", len(retUnits))
	}
	if retUnits[0].Amount != 1 || !retUnits[0].IsAsset(nftClass) {
		t.Fatalf("expected first unit to be pool NFT, got %+v", retUnits[0])
	}
	// The LQ token must follow the NFT, with its reserve passed through
	// unchanged (the swap does not mint or burn LQ tokens).
	if !retUnits[1].IsAsset(lqClass) || retUnits[1].Amount != 1_000_000 {
		t.Fatalf(
			"expected second unit to be LQ token (amount 1000000), got %+v",
			retUnits[1],
		)
	}
	// The token side reserve should be increased by the input amount.
	var foundToken bool
	for _, u := range retUnits {
		if u.IsAsset(tokenClass) {
			foundToken = true
			wantToken := tokenReserve + inputAsset.Amount
			if u.Amount != wantToken {
				t.Fatalf(
					"expected token reserve %d, got %d",
					wantToken,
					u.Amount,
				)
			}
		}
	}
	if !foundToken {
		t.Fatalf("token asset not found in return units: %+v", retUnits)
	}
}

// TestCalculateReturnToPoolUnderflow verifies that a reward larger than the
// pool's reserve of the output asset produces an error instead of underflowing
// the uint64 subtraction.
func TestCalculateReturnToPoolUnderflow(t *testing.T) {
	adaReserve := uint64(5_000_000)
	tokenReserve := uint64(150_000)
	pool := newTestPool(adaReserve, tokenReserve)

	inputAsset := spectrum.AssetAmount{Class: tokenClass, Amount: 100}
	// Reward exceeds the ADA reserve -> would underflow.
	rewardAsset := spectrum.AssetAmount{
		Class:  adaClass,
		Amount: adaReserve + 1,
	}

	_, _, err := pool.CalculateReturnToPool(inputAsset, rewardAsset)
	if err == nil {
		t.Fatalf(
			"expected underflow error when reward (%d) > reserve (%d), got nil",
			rewardAsset.Amount,
			adaReserve,
		)
	}
}
