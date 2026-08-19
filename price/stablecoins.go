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

package price

import (
	"github.com/blinklabs-io/shai/common"
)

const (
	USDMPolicyID  = "c48cbb3d5e57ed56e276bc45f99ab39abe94e6cd7ac39fb402da47ad"
	USDMAssetName = "0014df105553444d"

	USDCxPolicyID  = "1f3aec8bfe7ea4fe14c5f121e2a92e301afe414147860d557cac7e34"
	USDCxAssetName = "5553444378"
)

// Stablecoin is an explicitly authenticated dollar-pegged Cardano asset.
// Decimals are display units; on-chain quantities remain integers.
type Stablecoin struct {
	Symbol   string
	Asset    common.AssetClass
	Decimals uint8
}

// MainnetStablecoins returns the reviewed stablecoin registry used for local
// ADA/USD pool observations.
func MainnetStablecoins() []Stablecoin {
	return []Stablecoin{
		mustStablecoin("USDM", USDMPolicyID, USDMAssetName, 6),
		mustStablecoin("USDCx", USDCxPolicyID, USDCxAssetName, 6),
	}
}

func mustStablecoin(
	symbol,
	policyID,
	assetName string,
	decimals uint8,
) Stablecoin {
	asset, err := common.NewAssetClass(policyID, assetName)
	if err != nil {
		panic(err)
	}
	return Stablecoin{
		Symbol:   symbol,
		Asset:    asset,
		Decimals: decimals,
	}
}
