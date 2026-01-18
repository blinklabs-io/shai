# Shai

A Cardano Multi-DEX matcher bot and oracle. Shai monitors the blockchain for swap/deposit/redeem requests and executes matching transactions against liquidity pools. It also functions as an oracle, tracking pool state that can be served to other protocols.

## Features

- **Multi-DEX Support** - Batcher support for multiple Cardano DEXes
- **Oracle Mode** - Track pool/vault state and provide price feeds
- **Order Routing** - Smart order routing across multiple DEXes (Genius Yield)
- **Liquidation Bot** - Monitor positions and execute liquidations (FluidTokens)
- **Dual Node Mode** - Acts as both Cardano node (NtN/NtC) and indexer

## Supported Protocols

### DEX Batching

| Protocol | Version | Status |
|----------|---------|--------|
| Spectrum | v1, v2 | Ready |

### DEX Oracle

| Protocol | Version | Status |
|----------|---------|--------|
| Minswap | v1, v2 | Ready |
| SundaeSwap | v1, v3 | Ready |
| Splash | v1 | Ready |
| WingRiders | v2 | Ready |
| VyFi | - | Ready |

### Synthetics/CDP

| Protocol | Status |
|----------|--------|
| Butane | Ready |
| Indigo | Ready |

### Lending

| Protocol | Status |
|----------|--------|
| Liqwid | Ready |

### Bonds

| Protocol | Status |
|----------|--------|
| Optim | Ready |

### Order Routing

| Protocol | Status |
|----------|--------|
| Genius Yield | Ready |

### Liquidation

| Protocol | Status |
|----------|--------|
| FluidTokens | Ready |

## Building

```bash
# Build all binaries
make build

# Run tests
make test

# Format code
make format

# Clean build artifacts
make clean
```

## Configuration

Shai uses YAML configuration files and environment variables.

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `NETWORK` | Cardano network (mainnet, preview) | mainnet |
| `PROFILES` | Comma-separated list of enabled profiles | - |
| `MNEMONIC` | Wallet seed phrase | - |
| `STORAGE_DIR` | BadgerDB storage location | ./.shai |

### Example Configuration

```yaml
network: mainnet
profiles:
  - name: spectrum-mainnet
    type: spectrum
  - name: minswap-oracle
    type: oracle
    protocol: minswap-v2
```

## Architecture

```
cmd/shai/main.go           Entry point
internal/
├── node/                  Cardano node (NtN/NtC protocols)
├── indexer/               Chain synchronization
├── spectrum/              DEX matching logic
├── geniusyield/           Order-book DEX batcher (SOR)
├── fluidtokens/           Liquidation bot
├── oracle/                Price/state tracking
│   ├── minswap/           Minswap parser
│   ├── sundaeswap/        SundaeSwap parser
│   ├── splash/            Splash parser
│   ├── wingriders/        WingRiders parser
│   ├── vyfi/              VyFi parser
│   ├── butane/            Butane CDP parser
│   ├── indigo/            Indigo CDP parser
│   ├── liqwid/            Liqwid lending parser
│   ├── optim/             Optim bonds parser
│   └── geniusyield/       Genius Yield order parser
├── storage/               BadgerDB persistence
├── config/                Configuration handling
├── wallet/                Wallet management (bursa)
└── common/                Shared types
```

## Data Flow

1. Indexer syncs chain via adder pipeline, starting from earliest profile intercept point
2. Pool UTXOs are stored when seen on-chain (by NFT identifier)
3. Mempool transactions are monitored for swap/deposit/redeem requests
4. When a matching request is found, the corresponding pool UTXO is fetched
5. A matching transaction is built (using apollo) and submitted

## Dependencies

- [gouroboros](https://github.com/blinklabs-io/gouroboros) - Cardano protocol library
- [adder](https://github.com/blinklabs-io/adder) - Chain indexing pipeline
- [apollo](https://github.com/Salvionied/apollo) - Transaction building
- [bursa](https://github.com/blinklabs-io/bursa) - Wallet management

## License

Apache License 2.0
