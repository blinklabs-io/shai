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
	"errors"
	"fmt"
	"time"
)

var ErrIntegrationUnverified = errors.New("saturnswap integration unverified")

// Parser records the future SaturnSwap parser shape without enabling parsing.
type Parser struct{}

// NewParser creates a disabled SaturnSwap parser scaffold.
func NewParser() *Parser {
	return &Parser{}
}

// Protocol returns the SaturnSwap protocol name.
func (p *Parser) Protocol() string {
	return ProtocolName
}

// ParsePoolDatum refuses to parse until the on-chain protocol details are
// verified. Do not wire this parser into cmd/shai or default profiles until the
// verification checklist in this package is complete.
func (p *Parser) ParsePoolDatum(
	_ []byte,
	_ []byte,
	_ string,
	_ uint32,
	_ uint64,
	_ time.Time,
) (*PoolState, error) {
	return nil, fmt.Errorf(
		"%w: parser disabled until script addresses, intercept slot, datum/redeemer schema, pool ID rules, and reserve extraction rules are verified",
		ErrIntegrationUnverified,
	)
}
