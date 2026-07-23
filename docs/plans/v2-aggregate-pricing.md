# Aggregate Pricing v2

## Goal

Provide auditable Cardano prices from locally synchronized ledger data. Ship a
narrow ADA/USD endpoint first, then generalize the same exact-arithmetic and
fail-closed model to other pairs.

Runtime price construction must not depend on a remote CEX, provider REST API,
or Bursa. Bursa is a later consumer of a released Shai interface.

## Decisions

- Local DEX state is the active ADA/USD source.
- USDM and USDCx are the initial authenticated stablecoin assets.
- Orcfax and Charli3 are excluded from runtime selection while their latest
  authenticated ADA/USD values remain stale or expired.
- External-feed decoder work is diagnostic until a fresh current deployment
  and fixture can be verified on-chain.
- Prices stay rational through parsing, qualification, comparison, and
  aggregation. Floating point is a presentation format only.
- An unavailable result is preferable to an unqualified price.

## Phase 0: ADA/USD vertical slice

### Stablecoin registry

Each entry contains:

- symbol;
- policy ID;
- asset name;
- decimals;
- network;
- issuer/source documentation.

Asset symbols alone never authenticate an input.

Initial mainnet entries:

| Symbol | Policy ID | Asset name | Decimals |
|---|---|---|---|
| USDM | `c48cbb3d5e57ed56e276bc45f99ab39abe94e6cd7ac39fb402da47ad` | `0014df105553444d` | 6 |
| USDCx | `1f3aec8bfe7ea4fe14c5f121e2a92e301afe414147860d557cac7e34` | `5553444378` | 6 |

### Pool qualification

For each locally parsed pool:

1. Require exactly one ADA side and one registered stablecoin side.
2. Select the latest deterministic pool snapshot.
3. Exclude mempool snapshots by default.
4. Reject zero reserves and decimal conversion overflow.
5. Enforce minimum ADA and normalized stablecoin reserves.
6. Preserve slot, block hash, transaction hash/index, and block timestamp.

Pool price is:

```text
stablecoin base units × 10^6
────────────────────────────
lovelace × 10^stablecoin_decimals
```

### Aggregate policy

The initial conservative policy is:

- minimum two observations;
- minimum two stablecoin symbols;
- minimum 1,000 ADA per pool;
- minimum 100 normalized USD per pool;
- maximum 75% contribution from one pool;
- maximum 5% high/low divergence.

Use normalized stablecoin liquidity as the initial weight. Compare exact
rational shares and divergence against rationalized configuration thresholds.
Return unavailable on insufficient liquidity, diversity, concentration, or
agreement.

This policy reduces single-pool and single-stablecoin dependence. It does not
prove a stablecoin's peg. Peg-health signals remain a separate input.

### API

`GET /api/v1/prices/ada-usd`

A successful response includes:

- pair and exact numerator/denominator;
- decimal presentation;
- source and aggregation method;
- validation status;
- conservative observation time and age;
- aggregate spread;
- each pool's protocol, identity, stablecoin, reserves, price, slot, block,
  transaction, time, age, and validation.

Qualification failure returns HTTP 503 with an unavailable result and the
qualified observations retained for diagnosis.

### Required validation

- Exact current mainnet CSWAP ADA/USDM and ADA/USDCx fixtures.
- Wrong policy or asset name.
- Decimal normalization and overflow.
- Zero and minimum reserves.
- Mempool exclusion.
- Duplicate and same-slot snapshots.
- Liquidity concentration.
- Exact divergence boundary and divergent pools.
- Stablecoin diversity.
- API success and explicit unavailable response.
- Rollback/restart behavior.
- Live mainnet sync and chain-tip freshness.

## External on-chain feeds

An external feed can become eligible only when all of the following are true:

1. A current provider deployment is independently documented.
2. Its latest authenticated UTxO is fresh on-chain.
3. Authentication uses the deployment identity and token/NFT, not address
   matching alone.
4. Feed identifier, rational bounds, timestamp, expiry, and future-time rules
   are enforced.
5. Rollback and restart persistence have tests.
6. Stale, malformed, and divergent values fail closed.

Until then, Orcfax and Charli3 checks belong in operational diagnostics, not
the production selection policy.

## Phase 1: source-neutral observations

Introduce a model shared by local and future authenticated sources:

```go
type Observation struct {
    Base       Asset
    Quote      Asset
    PriceNum   *big.Int
    PriceDen   *big.Int
    Source     string
    ObservedAt time.Time
    Slot       uint64
    BlockHash  string
    TxHash     string
    Validation ValidationStatus
}
```

Adapters own authentication and parsing. The aggregator receives only typed
observations plus explicit health; it does not discover or silently trust
sources.

## Phase 2: robust aggregation

- Weighted median across independent source groups.
- Per-source and per-asset weight caps.
- Configurable maximum age and future-time tolerance.
- Median absolute deviation or equivalent outlier rejection.
- Circuit breaker against the last accepted value.
- Confidence derived from diversity, liquidity, agreement, and age.
- Persist accepted windows and health across restart.

No circuit breaker may silently keep serving an expired value.

## Phase 3: time windows

- Slot/time-bucketed observations.
- Exact cumulative price arithmetic.
- Configurable TWAP windows.
- Minimum coverage and maximum-gap requirements.
- Rollback-safe window removal.
- Bounded retention and restart recovery.

## Phase 4: routed prices

Support native-asset/USD through explicit paths such as:

```text
TOKEN/ADA × ADA/USD = TOKEN/USD
```

Route selection must account for liquidity, age, confidence, hop count, and
source correlation. Apply precision and overflow bounds at every hop.

## Phase 5: interfaces

- `GET /api/v2/prices/{base}/{quote}`
- raw/diagnostic source-health endpoint;
- WebSocket aggregate updates;
- circuit-breaker alerts;
- stable versioned schemas for downstream consumers.

The v1 ADA/USD endpoint remains available during migration.

## Non-goals

- Runtime remote/CEX price fallback.
- Assuming a stablecoin is always worth one USD.
- Merging the historical issue-318 prototype wholesale.
- Publishing a Shai-owned on-chain oracle before consumption is production
  ready.
- Modifying Bursa before Shai releases a stable consumer interface.

## Release gates

1. Local ADA/USD code, current fixtures, and provenance merge.
2. Mainnet live-sync smoke test passes from the configured intercept.
3. Rollback, restart, and chain-tip freshness behavior is verified.
4. API schema and operational settings are documented.
5. A tagged Shai release establishes the downstream integration boundary.
6. Bursa integration is developed separately against that release.
