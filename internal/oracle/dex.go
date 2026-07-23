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

package oracle

import "github.com/blinklabs-io/shai/dex"

// The pool-datum parsing and pricing primitives now live in the public
// github.com/blinklabs-io/shai/dex package so external programs can reuse them
// without depending on the oracle service. The aliases and re-exports below
// keep the oracle service's internal API stable.

// PoolState is an alias for dex.PoolState.
type PoolState = dex.PoolState

// PriceUpdate is an alias for dex.PriceUpdate.
type PriceUpdate = dex.PriceUpdate

// PoolParser is an alias for dex.PoolParser.
type PoolParser = dex.PoolParser

// PoolVolume is an alias for dex.PoolVolume.
type PoolVolume = dex.PoolVolume

// ActivityTracker is an alias for dex.ActivityTracker.
type ActivityTracker = dex.ActivityTracker

// NewPriceUpdate re-exports dex.NewPriceUpdate.
var NewPriceUpdate = dex.NewPriceUpdate

// NewActivityTracker re-exports dex.NewActivityTracker.
var NewActivityTracker = dex.NewActivityTracker

// clonePoolState wraps dex.ClonePoolState for internal use.
func clonePoolState(state *PoolState) *PoolState {
	return dex.ClonePoolState(state)
}

// Re-exported protocol parser constructors.
var (
	NewMinswapV1Parser    = dex.NewMinswapV1Parser
	NewMinswapV2Parser    = dex.NewMinswapV2Parser
	NewSundaeSwapV1Parser = dex.NewSundaeSwapV1Parser
	NewSundaeSwapV3Parser = dex.NewSundaeSwapV3Parser
	NewSplashV1Parser     = dex.NewSplashV1Parser
	NewWingRidersV2Parser = dex.NewWingRidersV2Parser
	NewVyFiParser         = dex.NewVyFiParser
	NewCSwapParser        = dex.NewCSwapParser
)
