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

package saturnswap

import (
	"time"

	"github.com/blinklabs-io/shai/common"
)

const (
	ProtocolName                = "saturnswap"
	IntegrationStatusUnverified = "unverified"
	MainnetNetwork              = "mainnet"

	DefaultExternalAPIEnabled = false
)

// PoolState is the intended parsed output for SaturnSwap oracle support.
type PoolState struct {
	PoolId    string
	Protocol  string
	AssetX    common.AssetAmount
	AssetY    common.AssetAmount
	FeeNum    uint64
	FeeDenom  uint64
	Slot      uint64
	TxHash    string
	TxIndex   uint32
	Timestamp time.Time
}

// VerificationItem names one fact that must be verified before runtime support.
type VerificationItem struct {
	Key                   string
	Description           string
	RequiredBeforeRuntime bool
}

// ExternalAPIResearchPolicy documents how off-chain sources may be used.
type ExternalAPIResearchPolicy struct {
	Optional         bool
	EnabledByDefault bool
	Note             string
}

// IntegrationTarget summarizes the disabled SaturnSwap integration target.
type IntegrationTarget struct {
	Protocol          string
	Status            string
	Network           string
	ExternalAPIConfig APIConfig
	ExternalAPIs      ExternalAPIResearchPolicy
	VerificationItems []VerificationItem
}

var verificationChecklist = []VerificationItem{
	{
		Key:                   "script-addresses",
		Description:           "verify SaturnSwap pool, order, and related script addresses/hashes for each supported network",
		RequiredBeforeRuntime: true,
	},
	{
		Key:                   "intercept-point",
		Description:           "verify the earliest intercept slot and block hash for the first relevant pool-script transaction",
		RequiredBeforeRuntime: true,
	},
	{
		Key:                   "datum-redeemer-schema",
		Description:           "verify pool datum and order/redeemer schema constructors, field ordering, and asset encoding",
		RequiredBeforeRuntime: true,
	},
	{
		Key:                   "pool-id-rules",
		Description:           "verify pool ID rules, including the NFT/LP token or datum fields that make a pool unique",
		RequiredBeforeRuntime: true,
	},
	{
		Key:                   "reserve-extraction-rules",
		Description:           "verify reserve extraction rules from UTxO values, including which assets are reserves and which markers are excluded",
		RequiredBeforeRuntime: true,
	},
}

// VerificationChecklist returns the facts required before enabling runtime code.
func VerificationChecklist() []VerificationItem {
	ret := make([]VerificationItem, len(verificationChecklist))
	copy(ret, verificationChecklist)
	return ret
}

// Target returns the current SaturnSwap integration target.
func Target() IntegrationTarget {
	return IntegrationTarget{
		Protocol:          ProtocolName,
		Status:            IntegrationStatusUnverified,
		Network:           MainnetNetwork,
		ExternalAPIConfig: DefaultAPIConfig(),
		ExternalAPIs: ExternalAPIResearchPolicy{
			Optional:         true,
			EnabledByDefault: false,
			Note:             "the public GraphQL API may be enabled explicitly for pool discovery and transaction build/submit; on-chain parser support remains disabled until verification is complete",
		},
		VerificationItems: VerificationChecklist(),
	}
}
