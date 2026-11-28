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
// package stops at order state and is not registered as a dex.PoolParser. A
// resting order locks only the offered asset. There is no asked-side reserve
// on chain, so dex.PoolState's second reserve, its constant-product Quote, its
// TVL, and the opposite-direction reserve move that dex.InferSwapTransition
// uses to recognise a swap all lack an on-chain referent. Deriving the
// asked-side reserve from the maker's limit price would publish every resting
// order as a pool quoting its own asking price.
//
// The oracle tracks these orders through its order-book profile instead, which
// locates them by the payment credential in OrderScriptHash: each maker's
// order sits at a base address combining that script hash with the maker's own
// staking credential, so no fixed address set enumerates them. Order outputs
// carry their datum by hash rather than inline, so the datum is resolved from
// the spending transaction's Plutus witness data.
package geniusyield
