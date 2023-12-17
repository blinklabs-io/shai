package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"

	"github.com/blinklabs-io/shai/internal/config"
	"github.com/blinklabs-io/shai/internal/logging"
	"github.com/blinklabs-io/shai/internal/version"
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

	// TODO: do something useful
}
