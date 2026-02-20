# AGENTS.md

This file provides guidance to AI Agents when working with code in this repository.

## Build Commands

```bash
# Build all binaries (shai, mk-script-address)
make build

# Run tests
make test

# Format code (runs go mod tidy, go fmt, gofmt -s)
make format

# Format with golines (80 char line limit)
make golines

# Clean build artifacts
make clean
```

## Project Overview

Shai is a Cardano Multi-DEX matcher bot and oracle. It monitors the blockchain for swap/deposit/redeem requests and executes matching transactions against liquidity pools. It also functions as an oracle, tracking pool state that can be served to other protocols even when not acting as a batcher. It acts as both a Cardano node (accepting NtN and NtC connections) and an indexer.

## Architecture

### Core Components

**cmd/shai/main.go** - Entry point. Initializes components in order: config → logging → storage → wallet → indexer → node → profiles → txsubmit.

**internal/node/** - Implements a partial Cardano node supporting both Node-to-Node (NtN) and Node-to-Client (NtC) protocols via gouroboros. Manages peer connections, chainsync, and transaction submission/mempool.

**internal/indexer/** - Chain synchronization using the adder pipeline. Tracks UTXOs for the bot wallet and maintains a cursor position in BadgerDB. Fires events to registered handlers (profiles).

**internal/spectrum/** - DEX matching logic. Handles Spectrum-compatible protocols. Monitors for swap/deposit/redeem requests in mempool, fetches pool state, builds and submits matching transactions.

**internal/storage/** - BadgerDB persistence layer. Stores chainsync cursor, UTXOs by address, and pool UTXOs by NFT asset.

**internal/config/** - Configuration via YAML files and environment variables (using envconfig). Profiles define DEX-specific parameters (script hashes, reference inputs, intercept points).

**internal/wallet/** - Wallet management using bursa. Auto-generates mnemonic to seed.txt if not provided.

**internal/txsubmit/** - Transaction submission via NtN peers or API endpoint.

### Data Flow

1. Indexer syncs chain via adder pipeline, starting from earliest profile intercept point
2. Pool UTXOs are stored when seen on-chain (by NFT identifier)
3. Mempool transactions are monitored for swap/deposit/redeem requests
4. When a matching request is found, the corresponding pool UTXO is fetched
5. A matching transaction is built (using apollo) and submitted

### Configuration

Environment variables use `DUMMY_` prefix internally but are mapped without it:
- `NETWORK` - Cardano network (mainnet, preview)
- `PROFILES` - Comma-separated list of enabled DEX profiles
- `MNEMONIC` - Wallet seed phrase
- `STORAGE_DIR` - BadgerDB storage location (default: ./.shai)

Profiles are defined in `internal/config/profiles.go` with network-specific parameters.