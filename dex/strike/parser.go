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

package strike

import (
	"fmt"
)

// Parser is a fail-closed placeholder for Strike Finance perpetual datums.
// It must not be wired into profiles until the on-chain targets are verified.
type Parser struct {
	targets OnChainTargets
}

func NewParser(targets OnChainTargets) *Parser {
	return &Parser{targets: targets}
}

func (p *Parser) Protocol() string {
	return IntegrationName
}

func (p *Parser) Targets() OnChainTargets {
	return p.targets
}

func (p *Parser) ValidateRuntimeEnablement() error {
	return p.targets.ValidateRuntimeEnablement()
}

func (p *Parser) ParseMarketDatum(
	datum []byte,
	txHash string,
	txIndex uint32,
	slot uint64,
) (*MarketState, error) {
	return nil, unverifiedSchemaError("market datum")
}

func (p *Parser) ParsePositionDatum(
	datum []byte,
	txHash string,
	txIndex uint32,
	slot uint64,
) (*PositionState, error) {
	return nil, unverifiedSchemaError("position datum")
}

func (p *Parser) ParseRedeemer(redeemer []byte) (*Redeemer, error) {
	return nil, unverifiedSchemaError("redeemer")
}

func unverifiedSchemaError(subject string) error {
	return fmt.Errorf(
		"%w: %s schema is unverified for %s; %w before enabling runtime support",
		ErrUnsupported,
		subject,
		IntegrationName,
		ErrVerificationRequired,
	)
}
