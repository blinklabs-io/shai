package main

import (
	"flag"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"

	"github.com/blinklabs-io/shai/internal/config"
	"github.com/blinklabs-io/shai/internal/geniusyield"
	"github.com/blinklabs-io/shai/internal/indexer"
	"github.com/blinklabs-io/shai/internal/logging"
	"github.com/blinklabs-io/shai/internal/node"
	"github.com/blinklabs-io/shai/internal/oracle"
	"github.com/blinklabs-io/shai/internal/spectrum"
	"github.com/blinklabs-io/shai/internal/storage"
	"github.com/blinklabs-io/shai/internal/txsubmit"
	"github.com/blinklabs-io/shai/internal/version"
	"github.com/blinklabs-io/shai/internal/wallet"
	_ "go.uber.org/automaxprocs"
)

const (
	programName = "shai"
)

var cmdlineFlags struct {
	configFile string
	version    bool
}

func main() {
	flag.StringVar(
		&cmdlineFlags.configFile,
		"config",
		"",
		"path to config file to load",
	)
	flag.BoolVar(&cmdlineFlags.version, "version", false, "show version")
	flag.Parse()

	if cmdlineFlags.version {
		fmt.Printf("%s %s\n", programName, version.GetVersionString())
		os.Exit(0)
	}

	// Load config
	cfg, err := config.Load(cmdlineFlags.configFile)
	if err != nil {
		fmt.Printf("Failed to load config: %s\n", err)
		os.Exit(1)
	}

	// Configure logging
	logging.Configure()
	logger := logging.GetLogger()

	// Start debug listener
	if cfg.Debug.ListenPort > 0 {
		logger.Info(
			"starting debug listener",
			"address",
			cfg.Debug.ListenAddress,
			"port",
			cfg.Debug.ListenPort,
		)
		go func() {
			err := http.ListenAndServe(
				fmt.Sprintf(
					"%s:%d",
					cfg.Debug.ListenAddress,
					cfg.Debug.ListenPort,
				),
				nil,
			)
			if err != nil {
				logger.Error("failed to start debug listener", "error", err)
				os.Exit(1)
			}
		}()
	}

	// Load storage
	if err := storage.GetStorage().Load(); err != nil {
		logger.Error("failed to load storage", "error", err)
		os.Exit(1)
	}

	// Setup wallet
	wallet.Setup()
	bursa := wallet.GetWallet()
	logger.Info("loaded mnemonic for address", "address", bursa.PaymentAddress)

	// Initialize indexer and node
	idx := indexer.New()
	n := node.New(idx)

	// Setup profiles
	for _, profile := range config.GetProfiles() {
		switch profile.Type {
		case config.ProfileTypeSpectrum:
			logger.Info(
				"initializing profile",
				"name",
				profile.Name,
				"type",
				"Spectrum",
			)
			_ = spectrum.New(
				idx,
				n,
				profile.Name,
				profile.Config.(config.SpectrumProfileConfig),
			)
		case config.ProfileTypeGeniusYield:
			logger.Info(
				"initializing profile",
				"name",
				profile.Name,
				"type",
				"GeniusYield",
			)
			_ = geniusyield.New(
				idx,
				n,
				profile.Name,
				profile.Config.(config.GeniusYieldProfileConfig),
			)
		case config.ProfileTypeOracle:
			oracleCfg := profile.Config.(config.OracleProfileConfig)
			logger.Info(
				"initializing profile",
				"name",
				profile.Name,
				"type",
				"Oracle",
				"protocol",
				oracleCfg.Protocol,
			)
			parser := getOracleParser(oracleCfg.Protocol)
			if parser == nil {
				logger.Error(
					"unknown oracle protocol",
					"protocol",
					oracleCfg.Protocol,
				)
				os.Exit(1)
			}
			o := oracle.New(idx, n, &profile, parser)
			if err := o.Start(); err != nil {
				logger.Error(
					"failed to start oracle",
					"error",
					err,
					"profile",
					profile.Name,
				)
				os.Exit(1)
			}
		case config.ProfileTypeSynthetics:
			synthCfg := profile.Config.(config.SyntheticsProfileConfig)
			logger.Info(
				"initializing profile",
				"name",
				profile.Name,
				"type",
				"Synthetics",
				"protocol",
				synthCfg.Protocol,
			)
			parser := getSyntheticsParser(synthCfg.Protocol)
			if parser == nil {
				logger.Error(
					"unknown synthetics protocol",
					"protocol",
					synthCfg.Protocol,
				)
				os.Exit(1)
			}
			o := oracle.New(idx, n, &profile, parser)
			if err := o.Start(); err != nil {
				logger.Error(
					"failed to start synthetics oracle",
					"error",
					err,
					"profile",
					profile.Name,
				)
				os.Exit(1)
			}
		case config.ProfileTypeLending:
			lendingCfg := profile.Config.(config.LendingProfileConfig)
			logger.Info(
				"initializing profile",
				"name",
				profile.Name,
				"type",
				"Lending",
				"protocol",
				lendingCfg.Protocol,
			)
			parser := getLendingParser(lendingCfg.Protocol)
			if parser == nil {
				logger.Error(
					"unknown lending protocol",
					"protocol",
					lendingCfg.Protocol,
				)
				os.Exit(1)
			}
			o := oracle.NewLendingOracle(idx, &profile, parser)
			if err := o.Start(); err != nil {
				logger.Error(
					"failed to start lending oracle",
					"error",
					err,
					"profile",
					profile.Name,
				)
				os.Exit(1)
			}
		case config.ProfileTypeBonds:
			bondsCfg := profile.Config.(config.BondsProfileConfig)
			logger.Info(
				"initializing profile",
				"name",
				profile.Name,
				"type",
				"Bonds",
				"protocol",
				bondsCfg.Protocol,
			)
			parser := getBondsParser(bondsCfg.Protocol)
			if parser == nil {
				logger.Error(
					"unknown bonds protocol",
					"protocol",
					bondsCfg.Protocol,
				)
				os.Exit(1)
			}
			o := oracle.New(idx, n, &profile, parser)
			if err := o.Start(); err != nil {
				logger.Error(
					"failed to start bonds oracle",
					"error",
					err,
					"profile",
					profile.Name,
				)
				os.Exit(1)
			}
		case config.ProfileTypeFluidTokens:
			logger.Info(
				"initializing profile",
				"name",
				profile.Name,
				"type",
				"FluidTokens",
			)
			// FluidTokens liquidator - placeholder for future implementation
			logger.Warn(
				"FluidTokens profile configured but not yet fully implemented",
				"profile",
				profile.Name,
			)
		case config.ProfileTypeNone:
			logger.Error("profile type none given")
			os.Exit(1)
		default:
			logger.Error("unknown profile type", "name", profile.Name)
			os.Exit(1)
		}
	}

	// Start node
	if err := n.Start(); err != nil {
		logger.Error("failed to start node", "error", err)
		os.Exit(1)
	}

	// Start TxSubmit
	if err := txsubmit.Start(n); err != nil {
		logger.Error("failed to start TxSubmit", "error", err)
		os.Exit(1)
	}

	// Start indexer
	if err := idx.Start(); err != nil {
		logger.Error("failed to start indexer", "error", err)
		os.Exit(1)
	}

	// Wait forever
	select {}
}

// getOracleParser returns the appropriate parser for a DEX oracle protocol.
// Only parsers implementing the standard PoolParser interface are supported.
func getOracleParser(protocol string) oracle.PoolParser {
	switch protocol {
	case "minswap-v1":
		return oracle.NewMinswapV1Parser()
	case "minswap-v2", "minswap":
		return oracle.NewMinswapV2Parser()
	case "sundaeswap-v1":
		return oracle.NewSundaeSwapV1Parser()
	case "sundaeswap-v3", "sundaeswap":
		return oracle.NewSundaeSwapV3Parser()
	case "splash-v1", "splash":
		return oracle.NewSplashV1Parser()
	case "wingriders-v2", "wingriders":
		return oracle.NewWingRidersV2Parser()
	// VyFi requires additional context (asset info) - use specialized handler
	// GeniusYield is order-book based - use ProfileTypeGeniusYield instead
	default:
		return nil
	}
}

// getSyntheticsParser returns the appropriate parser for a synthetics protocol.
// These protocols track CDPs and synthetic assets.
func getSyntheticsParser(protocol string) oracle.PoolParser {
	switch protocol {
	case "butane":
		return oracle.NewButaneParser()
	case "indigo":
		return oracle.NewIndigoParser()
	default:
		return nil
	}
}

// getLendingParser returns the appropriate parser for a lending protocol.
// Lending protocols (Liqwid, Levvy) have specialized interfaces
// different from the standard PoolParser, so we use LendingParser adapters.
func getLendingParser(protocol string) oracle.LendingParser {
	return oracle.GetLendingParser(protocol)
}

// getBondsParser returns the appropriate parser for a bonds protocol.
func getBondsParser(protocol string) oracle.PoolParser {
	switch protocol {
	case "optim":
		return oracle.NewOptimParser()
	default:
		return nil
	}
}
