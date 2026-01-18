package main

import (
	"flag"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"

	"github.com/blinklabs-io/shai/internal/config"
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
		case config.ProfileTypeOracle:
			oracleCfg, ok := profile.Config.(config.OracleProfileConfig)
			if !ok {
				logger.Error(
					"invalid oracle profile config",
					"profile",
					profile.Name,
				)
				os.Exit(1)
			}
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
			o := oracle.New(idx, &profile, parser)
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

// getOracleParser returns the appropriate parser for an oracle protocol
func getOracleParser(protocol string) oracle.PoolParser {
	switch protocol {
	case "minswap-v2", "minswap":
		return oracle.NewMinswapV2Parser()
	default:
		return nil
	}
}
