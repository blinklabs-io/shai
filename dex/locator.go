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

package dex

import "slices"

// PoolLocator describes where a protocol's pool UTxOs can be found on-chain.
//
// Shai (and these parsers) identify pool UTxOs by the script address that holds
// them: a caller querying its own node should ask for the UTxOs sitting at one
// of these addresses, then feed each output's datum and value CBOR to the
// matching parser's ParsePoolDatum.
//
// PoolNFTPolicy is provided for protocols whose pool UTxOs additionally carry a
// pool-identifier NFT under a known minting policy; it may be empty when the
// protocol is located purely by address (which is how shai's oracle matches
// them today).
type PoolLocator struct {
	// Protocol is the protocol identifier, matching PoolParser.Protocol()
	// (e.g. "minswap-v2", "sundaeswap-v3", "vyfi", "cswap").
	Protocol string
	// Network is the Cardano network these addresses apply to (e.g. "mainnet").
	Network string
	// Addresses are the bech32 script addresses that hold this protocol's pool
	// UTxOs. Query your node for UTxOs at these addresses.
	Addresses []string
	// PoolNFTPolicy is the hex-encoded minting policy id of the pool-identifier
	// NFT, when applicable. Empty if the protocol is located by address only.
	PoolNFTPolicy string
}

// mainnetLocators is the canonical mapping of protocol -> mainnet pool
// locator. This is the single source of truth for shai's service config and
// for external library consumers.
var mainnetLocators = map[string]PoolLocator{
	"minswap-v1": {
		Protocol: "minswap-v1",
		Network:  "mainnet",
		Addresses: []string{
			"addr1z8snz7c4974vzdpxu65ruphl3zjdvtxw8strf2c2tmqnxzfgf2ypu62xjxel6aqdmr333p0ds377t4phv8098c8s8fmqffc3l3",
		},
	},
	"minswap-v2": {
		Protocol: "minswap-v2",
		Network:  "mainnet",
		Addresses: []string{
			"addr1z8snz7c4974vzdpxu65ruphl3zjdvtxw8strf2c2tmqnxz2j2c79gy9l76sdg0xwhd7r0c0kna0tycz4y5s6mlenh8pq0xmsha",
		},
	},
	"sundaeswap-v1": {
		Protocol: "sundaeswap-v1",
		Network:  "mainnet",
		Addresses: []string{
			"addr1wyx22z2s4kasd3w976pnjf9xdty88epjqfvgkmfnfpsdacqe7utc8",
		},
	},
	"sundaeswap-v3": {
		Protocol: "sundaeswap-v3",
		Network:  "mainnet",
		Addresses: []string{
			"addr1x8srqftqemf0mjlukfszd97ljuxdp44r372txfcr75wrz26rnxqnmtv3hdu2t6chcfhl2zzjh36a87nmd6dwsu3jenqsslnz7e",
		},
	},
	"splash-v1": {
		Protocol: "splash-v1",
		Network:  "mainnet",
		Addresses: []string{
			"addr1w94ec3t25egvhqy2n265xfhq882jxhkknurfe9ny4rl9k6g03d4zz",
		},
	},
	"wingriders-v2": {
		Protocol: "wingriders-v2",
		Network:  "mainnet",
		Addresses: []string{
			"addr1w8nvjzjeydcn4atcd93aac8allvrpjn7pjr2qsweukpnayghhwcpj",
		},
	},
	"vyfi": {
		Protocol: "vyfi",
		Network:  "mainnet",
		Addresses: []string{
			"addr1z9vgl40qezca5s8ajz6wnpuwevt98l3jqx2ce5nlu8h8nnw60wckas4haxwwclas0g39cc8cvt2r8yalrfa9e8vxx92qsss9sx",
		},
	},
	"cswap": {
		Protocol: "cswap",
		Network:  "mainnet",
		Addresses: []string{
			"addr1z8ke0c9p89rjfwmuh98jpt8ky74uy5mffjft3zlcld9h7ml3lmln3mwk0y3zsh3gs3dzqlwa9rjzrxawkwm4udw9axhs6fuu6e",
		},
	},
	"geniusyield": {
		Protocol: "geniusyield",
		Network:  "mainnet",
		Addresses: []string{
			"addr1w9zr09hgj7z6vz3d7wnxw0u4x30arsp5k8avlcm84utptls8uqd0z",
		},
		PoolNFTPolicy: "fae686ea8f21d567841d703dea4d4221c2af071a6f2b433ff07c0af2",
	},
}

// Locator returns the mainnet pool locator for the given protocol identifier
// (as returned by PoolParser.Protocol()). The bool result is false if the
// protocol is unknown.
func Locator(protocol string) (PoolLocator, bool) {
	loc, ok := mainnetLocators[protocol]
	if !ok {
		return PoolLocator{}, false
	}
	loc.Addresses = slices.Clone(loc.Addresses)
	return loc, true
}

// Locators returns every known protocol locator (mainnet).
func Locators() []PoolLocator {
	out := make([]PoolLocator, 0, len(mainnetLocators))
	for _, loc := range mainnetLocators {
		loc.Addresses = slices.Clone(loc.Addresses)
		out = append(out, loc)
	}
	return out
}

// PoolAddresses returns the mainnet script addresses that hold pool UTxOs for
// the given protocol identifier. Returns nil if the protocol is unknown.
func PoolAddresses(protocol string) []string {
	loc, ok := mainnetLocators[protocol]
	if !ok {
		return nil
	}
	return slices.Clone(loc.Addresses)
}
