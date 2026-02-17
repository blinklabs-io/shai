# Shai

Shai is a Cardano Multi-DEX oracle and matcher bot. It monitors the blockchain for swap/deposit/redeem requests and executes matching transactions against liquidity pools. It also functions as an oracle, tracking pool state that can be served to other protocols.

## Features

- DEX Oracle: Real-time pool state tracking from on-chain data
- Multi-DEX Support: Minswap v2, SundaeSwap v3, Splash v1, WingRiders v2, VyFi
- Spectrum Batching: Matcher bot for Spectrum-compatible DEXs
- Mempool Monitoring: Track pending transactions for faster matching
- Cardano Node: Acts as both NtN (Node-to-Node) and NtC (Node-to-Client) peer
- Persistent Storage: BadgerDB for chain cursor and pool state

## Installation

### Prerequisites

- Go 1.24 or later
- Access to a Cardano node (for topology/peer connections)

### Build

```bash
# Build all binaries
make build

# Run tests
make test

# Format code
make format
```

This produces two binaries:
- `shai` - Main application
- `mk-script-address` - Utility for generating script addresses

## Configuration

Shai can be configured via YAML file, environment variables, or both.

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `NETWORK` | Cardano network (`mainnet`, `preview`) | `mainnet` |
| `PROFILES` | Comma-separated list of profiles to enable | `spectrum,teddyswap` |
| `MNEMONIC` | Wallet seed phrase (auto-generates if not set) | - |
| `STORAGE_DIR` | BadgerDB storage location | `./.shai` |
| `PORT` | NtN listen port | `3000` |
| `PORT_NTC` | NtC listen port | `3099` |
| `LOGGING_LEVEL` | Log level (`debug`, `info`, `warn`, `error`) | `info` |
| `CARDANO_TOPOLOGY` | Path to Cardano topology file | - |
| `INDEXER_TCP_ADDRESS` | Upstream node TCP address | - |
| `INDEXER_SOCKET_PATH` | Upstream node Unix socket path | - |
| `SUBMIT_URL` | Transaction submission API URL | - |
| `DEBUG_PORT` | pprof debug server port (0 to disable) | `0` |

### YAML Configuration

```yaml
network: mainnet
profiles:
  - minswap-v2
  - sundaeswap-v3
  - splash-v1
  - wingriders-v2
  - vyfi

logging:
  level: info

storage:
  dir: ./.shai

indexer:
  address: "localhost:3001"
  # socketPath: /path/to/node.socket

topology:
  configFile: /path/to/topology.json

wallet:
  mnemonic: "your seed phrase here"
```

### Available Profiles

Oracle profiles (price tracking):
- `minswap-v2` - Minswap V2 pools
- `sundaeswap-v3` - SundaeSwap V3 pools
- `splash-v1` - Splash (formerly Spectrum) pools
- `wingriders-v2` - WingRiders V2 pools
- `vyfi` - VyFi pools

Spectrum batching profiles (matcher bot):
- `spectrum` - Spectrum DEX on mainnet
- `teddyswap` - TeddySwap on preview/mainnet

## Usage

```bash
# Run with default configuration
./shai

# Run with config file
./shai -config config.yaml

# Show version
./shai -version
```

### Example: Oracle Mode

Track pool prices across multiple DEXs:

```bash
export NETWORK=mainnet
export PROFILES=minswap-v2,sundaeswap-v3,splash-v1,wingriders-v2,vyfi
export INDEXER_TCP_ADDRESS=localhost:3001
./shai
```

### Example: Batcher Mode

Run as a Spectrum-compatible matcher bot:

```bash
export NETWORK=mainnet
export PROFILES=spectrum
export MNEMONIC="your wallet seed phrase"
export INDEXER_TCP_ADDRESS=localhost:3001
./shai
```

## Utilities

### mk-script-address

Generate a Cardano script address from a Plutus script:

```bash
# From hex-encoded script
./mk-script-address -network mainnet -script-data <hex>

# From script file
./mk-script-address -network mainnet -script-path script.plutus

# Specify Plutus version (default: 2)
./mk-script-address -network mainnet -script-path script.plutus -plutus-version 1
```

## Architecture

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│   Indexer   │────▶│   Oracle    │────▶│   Storage   │
│  (chainsync)│     │  (parsers)  │     │  (BadgerDB) │
└─────────────┘     └─────────────┘     └─────────────┘
       │                   │
       ▼                   ▼
┌─────────────┐     ┌─────────────┐
│    Node     │     │  Spectrum   │
│  (NtN/NtC)  │     │  (batcher)  │
└─────────────┘     └─────────────┘
```

- Indexer: Syncs chain via the adder pipeline, fires events to profiles
- Oracle: Parses pool datums, tracks state, calculates prices
- Node: Accepts peer connections, handles chainsync and tx submission
- Spectrum: Monitors mempool for swap requests, builds matching transactions
- Storage: Persists chain cursor, UTXOs, and pool state

## License

Apache License 2.0
