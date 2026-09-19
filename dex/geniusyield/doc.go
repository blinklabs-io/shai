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

// Package geniusyield decodes Genius Yield partial order datums and derives
// order state and fill amounts from them.
//
// Genius Yield is a limit order book, not a constant-product AMM, so this
// package deliberately stops at order state and is not registered as a
// dex.PoolParser. Two properties have to be settled before an order book can
// join the oracle's pool surface:
//
//   - Location. Orders sit at base addresses that share the payment credential
//     in OrderScriptHash and carry the maker's own staking credential, so the
//     set of order addresses is unbounded. The oracle matches monitored UTxOs
//     by exact bech32 address, which cannot express that; locating orders
//     requires matching on the payment script hash or on OrderNFTPolicy.
//   - Semantics. A resting order locks only the offered asset. There is no
//     asked-side reserve on chain, so dex.PoolState's second reserve, its
//     constant-product Quote, its TVL, and the opposite-direction reserve move
//     that dex.InferSwapTransition uses to recognise a swap all lack an
//     on-chain referent. Deriving the asked-side reserve from the maker's limit
//     price publishes every resting order as a pool quoting its own asking
//     price.
//
// Until both are resolved, consumers parse orders directly through Parser and
// treat the result as order state.
package geniusyield
