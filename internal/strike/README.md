# Strike Finance Perpetuals

This package contains the Strike Finance perpetuals integration boundary. It is
not connected to `cmd/shai/main.go`, profile defaults, or the oracle runtime.
On-chain parsing remains gated until script targets and datum schemas are
verified.

Integration target:

- Protocol: `strike-finance`
- Product: `perpetuals`
- Candidate runtime name: `strike-finance-perpetuals`
- External APIs: optional only, disabled by default, and not a source of truth

Public API targets:

- Mainnet REST base: `https://api.strikefinance.org`
- Mainnet price REST base: `https://api.strikefinance.org/price`
- Testnet REST base: `https://api-v2-testnet.strikefinance.org`
- Testnet price REST base:
  `https://api-v2-testnet.strikefinance.org/price`

Implemented external API calls:

- `GET /v2/ping`
- `GET /v2/time`
- `GET /price/v2/markPrice?symbol=BTC-USD` when using the REST base, or
  `GET /v2/markPrice?symbol=BTC-USD` when using the price REST base.
- Generic authenticated JSON requests using the documented Ed25519 signature
  payload `{METHOD}:{PATH}:{TIMESTAMP}:{NONCE}:{BODY_HASH}`.

External API usage requires constructing a client with `ExternalAPIConfig.Enabled`
set to `true`. The default targets keep it false, and tests use `httptest`
instead of live network calls.

Required before runtime support can be enabled:

- [ ] Verify script addresses for market state, orders, and positions.
- [ ] Verify the first safe intercept slot and block hash.
- [ ] Verify datum schemas for market and position state.
- [ ] Verify redeemer schema and action constructors.
- [ ] Verify state transitions for open, update, close, liquidation, and
      settlement flows.
- [ ] Add deterministic parser fixtures from verified on-chain transactions.
- [ ] Only then add an explicit profile and parser wiring.

Until those items are complete, on-chain datum and redeemer parsers return
unsupported verification errors and must remain unused by runtime profiles.
