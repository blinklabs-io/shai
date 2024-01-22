package txsubmit

import (
	"fmt"

	"github.com/blinklabs-io/shai/internal/config"
	"github.com/blinklabs-io/shai/internal/logging"

	ouroboros "github.com/blinklabs-io/gouroboros"
)

func SubmitTx(txRawBytes []byte) error {
	cfg := config.GetConfig()
	logger := logging.GetLogger()
	if len(cfg.Topology.Hosts) > 0 {
		for _, host := range cfg.Topology.Hosts {
			address := fmt.Sprintf("%s:%d", host.Address, host.Port)
			go func(address string) {
				txSubmitNtn := NewTxSubmitNtn()
				txSubmitNtn.Submit(txRawBytes, address)
			}(address)
		}
		/*
			} else if cfg.Submit.SocketPath != "" {
				return submitTxNtC(txRawBytes)
		*/
	} else if cfg.Submit.Url != "" {
		if err := submitTxApi(txRawBytes); err != nil {
			logger.Errorf("failed to submit TX via API: %s", err)
			return err
		}
		logger.Infof("successfully submit TX via API")
		// TODO: remove me
		//os.Exit(1)
	} else {
		// Populate address info from indexer network
		network := ouroboros.NetworkByName(cfg.Network)
		if network == ouroboros.NetworkInvalid {
			logger.Fatalf("unknown network: %s", cfg.Network)
		}
		address := fmt.Sprintf("%s:%d", network.PublicRootAddress, network.PublicRootPort)
		go func(address string) {
			txSubmitNtn := NewTxSubmitNtn()
			txSubmitNtn.Submit(txRawBytes, address)
		}(address)
	}
	return nil
}
