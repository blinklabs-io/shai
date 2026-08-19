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

// Package saturnswap records the SaturnSwap DEX oracle integration target and
// an optional client for SaturnSwap's public GraphQL API.
//
// Runtime support is intentionally disabled. Before adding a profile or
// registering a parser, verify:
//   - pool, order, and related script addresses/hashes for each supported network
//   - the earliest intercept slot and block hash for the verified pool script
//   - pool datum and order/redeemer schema constructors and field ordering
//   - pool ID rules, including the asset or datum fields that make an ID unique
//   - reserve extraction rules, including which UTxO assets to include/exclude
//
// The documented public GraphQL API can build ready-to-sign transactions and
// expose pool discovery for aggregators. It is modeled here as an explicit
// opt-in client and remains disabled by default. Runtime parsing must still be
// based on verified on-chain script, datum, and UTxO data.
package saturnswap
