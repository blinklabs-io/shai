package main

import (
	"flag"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"

	_ "go.uber.org/automaxprocs"

	"github.com/blinklabs-io/shai/internal/config"
	"github.com/blinklabs-io/shai/internal/indexer"
	"github.com/blinklabs-io/shai/internal/logging"
	"github.com/blinklabs-io/shai/internal/node"
	"github.com/blinklabs-io/shai/internal/spectrum"
	"github.com/blinklabs-io/shai/internal/storage"
	"github.com/blinklabs-io/shai/internal/txsubmit"
	"github.com/blinklabs-io/shai/internal/version"
	"github.com/blinklabs-io/shai/internal/wallet"
)

const (
	programName = "shai"
)

var cmdlineFlags struct {
	configFile string
	version    bool
}

func main() {
	flag.StringVar(&cmdlineFlags.configFile, "config", "", "path to config file to load")
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
	// Sync logger on exit
	defer func() {
		if err := logger.Sync(); err != nil {
			// We don't actually care about the error here, but we have to do something
			// to appease the linter
			return
		}
	}()

	// Start debug listener
	if cfg.Debug.ListenPort > 0 {
		logger.Infof("starting debug listener on %s:%d", cfg.Debug.ListenAddress, cfg.Debug.ListenPort)
		go func() {
			err := http.ListenAndServe(fmt.Sprintf("%s:%d", cfg.Debug.ListenAddress, cfg.Debug.ListenPort), nil)
			if err != nil {
				logger.Fatalf("failed to start debug listener: %s", err)
			}
		}()
	}

	// Load storage
	if err := storage.GetStorage().Load(); err != nil {
		logger.Fatalf("failed to load storage: %s", err)
	}

	// Setup wallet
	wallet.Setup()
	bursa := wallet.GetWallet()
	logger.Infof("loaded mnemonic for address: %s", bursa.PaymentAddress)

	// Initialize indexer and node
	idx := indexer.New()
	n := node.New(idx)

	// Setup profiles
	for _, profile := range config.GetProfiles() {
		switch profile.Type {
		case config.ProfileTypeSpectrum:
			logger.Infof("initializing profile '%s' of type Spectrum", profile.Name)
			_ = spectrum.New(idx, n, profile.Name, profile.Config.(config.SpectrumProfileConfig))
		default:
			logger.Fatalf("unknown profile type for '%s'", profile.Name)
		}
	}

	// Start node
	if err := n.Start(); err != nil {
		logger.Fatalf("failed to start node: %s", err)
	}

	// Start TxSubmit
	if err := txsubmit.Start(n); err != nil {
		logger.Fatalf("failed to start TxSubmit: %s", err)
	}

	// Start indexer
	if err := idx.Start(); err != nil {
		logger.Fatalf("failed to start indexer: %s", err)
	}

	// Wait forever
	select {}
}
