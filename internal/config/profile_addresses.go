package config

import (
	"encoding/hex"
	"fmt"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/ledger/common"
)

func mustScriptEnterpriseAddress(networkName, paymentHashHex string) string {
	return mustScriptAddress(
		networkName,
		common.AddressTypeScriptNone,
		paymentHashHex,
		"",
	)
}

func mustScriptScriptAddress(
	networkName string,
	paymentHashHex string,
	stakingHashHex string,
) string {
	return mustScriptAddress(
		networkName,
		common.AddressTypeScriptScript,
		paymentHashHex,
		stakingHashHex,
	)
}

func mustScriptAddress(
	networkName string,
	addressType uint8,
	paymentHashHex string,
	stakingHashHex string,
) string {
	network, ok := ouroboros.NetworkByName(networkName)
	if !ok {
		panic(fmt.Sprintf("unknown Cardano network %q", networkName))
	}
	paymentHash, err := hex.DecodeString(paymentHashHex)
	if err != nil {
		panic(fmt.Sprintf("invalid payment hash %q: %v", paymentHashHex, err))
	}
	var stakingHash []byte
	if stakingHashHex != "" {
		stakingHash, err = hex.DecodeString(stakingHashHex)
		if err != nil {
			panic(fmt.Sprintf("invalid staking hash %q: %v", stakingHashHex, err))
		}
	}
	address, err := common.NewAddressFromParts(
		addressType,
		network.Id,
		paymentHash,
		stakingHash,
	)
	if err != nil {
		panic(fmt.Sprintf("invalid script address parts: %v", err))
	}
	return address.String()
}
