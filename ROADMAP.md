# Shai Roadmap

*Updated July 23, 2026*

Shai is a Cardano multi-DEX oracle, indexer, and matcher. The immediate goal
is a reliable ADA/USD price sourced solely from locally synchronized Cardano
state. Bursa integration is a later consumer milestone; Bursa is not part of
this repository's release scope.

## Priority 0: local ADA/USD

The first release path derives ADA/USD from authenticated Cardano stablecoin
assets in locally tracked DEX pools.

### Active delivery sequence

1. **Stablecoin registry**
   - Authenticate assets by policy ID and asset name.
   - Start with USDM and USDCx, both normalized to six decimal places.
   - Treat each stablecoin as a USD estimate, not an assumed perfect peg.
2. **Qualified pool observations**
   - Use confirmed ADA/stablecoin pool state already produced by chain sync.
   - Require minimum ADA and stablecoin reserves.
   - Exclude mempool state by default.
   - Deduplicate pool snapshots deterministically.
3. **Fail-closed aggregation**
   - Require multiple pools and multiple stablecoin symbols.
   - Cap one pool's liquidity share.
   - Reject excessive cross-pool divergence.
   - Keep arithmetic rational until the API boundary.
4. **Provenance and health**
   - Report contributing pools, stablecoins, reserves, slots, blocks,
     transactions, observation times, and validation status.
   - Return an explicit unavailable result when qualification fails.
5. **Operational validation**
   - Retain parser fixtures from current unspent mainnet pool UTxOs.
   - Run a mainnet live-sync smoke test.
   - Document deployment settings and the downstream consumer contract.

### Current sources

| Source | July 23 on-chain result | Runtime role |
|---|---|---|
| CSWAP ADA/USDM | Fresh qualified local pool | Initial ADA/USD input |
| CSWAP ADA/USDCx | Fresh qualified local pool | Initial ADA/USD input |
| Orcfax ADA/USD | Latest authenticated statement was stale | Diagnostics only |
| Charli3 ADA/USD | Documented authenticated feed was expired | Diagnostics only |
| Remote price APIs | Not local ledger data | Excluded |

Orcfax and Charli3 adapters must not enter the runtime price path merely
because their contracts can be decoded. They can be reconsidered only after a
fresh authenticated on-chain feed is observed and their identity, datum,
timestamp, rollback, and failure behavior are covered by fixtures.

### Acceptance criteria

- `GET /api/v1/prices/ada-usd` returns a typed local estimate or HTTP 503.
- No runtime price input requires an Internet or CEX price service.
- Asset identity, decimal normalization, liquidity, concentration, and
  divergence policies are tested.
- Responses contain enough chain provenance for a downstream consumer to
  audit every observation.
- Rollback, restart, chain-tip freshness, and live-sync behavior are validated
  before the endpoint is considered production-ready.

## Priority 1: aggregate pricing v2

After the ADA/USD vertical slice is stable:

- source-neutral rational observations;
- weighted-median and configurable source policy;
- TWAP windows;
- outlier rejection and circuit breakers;
- source-health and confidence reporting;
- arbitrary native-asset/USD routing through ADA;
- REST and WebSocket v2 endpoints.

The experimental `issue-318-aggregate-pricing` branch is a reference, not a
merge candidate. Extract small reviewed components against current `main`; do
not restore runtime CEX collection.

See [the v2 plan](docs/plans/v2-aggregate-pricing.md).

## Protocol coverage

### On `main`

- DEX pools: Minswap v1/v2, SundaeSwap v1/v3, Splash v1, WingRiders v2,
  VyFi, CSWAP, and SaturnSwap.
- Synthetics: Indigo.
- Lending/protocol feeds: Liqwid.
- Batchers: Spectrum and TeddySwap.
- Strike Finance has a merged integration boundary but remains runtime
  disabled pending deployment verification.

### Next protocol work

| Work | Next gate |
|---|---|
| CSWAP | Complete mainnet live-sync validation |
| Djed | Verify contracts, datum formats, reserve rules, and intercept |
| GeniusYield oracle/SOR | Rebase and extract independently |
| Butane | Verify deployment before parser extraction |
| Optim bonds | Verify deployment before parser extraction |
| FluidTokens | Write a current extraction plan and fixtures |

## Continuous hardening

- Mainnet fixtures for each parser and configured profile.
- No placeholder intercepts or unverified addresses.
- Overflow, rounding, and zero-reserve tests.
- Rollback, restart, staleness, and subscriber-backpressure tests.
- Explicit health for every local or external source.
- Periodic re-evaluation when `origin/main` advances.

## Publishing boundary

Shai currently consumes chain state and serves derived data off-chain. It does
not publish a Shai-owned oracle value on Cardano. On-chain publication remains
a post-v2 decision and must not block the local ADA/USD endpoint.
