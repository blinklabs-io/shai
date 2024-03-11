package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"os"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/ledger"
	"golang.org/x/crypto/blake2b"
)

var cmdlineFlags struct {
	network       string
	scriptData    string
	scriptPath    string
	plutusVersion int
}

func main() {
	flag.StringVar(&cmdlineFlags.scriptData, "script-data", "", "hex-encoded script data")
	flag.StringVar(&cmdlineFlags.scriptPath, "script-path", "", "path to script file to load")
	flag.StringVar(&cmdlineFlags.network, "network", "mainnet", "named network to generate script address for")
	flag.IntVar(&cmdlineFlags.plutusVersion, "plutus-version", 2, "plutus version of script")
	flag.Parse()

	if (cmdlineFlags.scriptPath == "" && cmdlineFlags.scriptData == "") || cmdlineFlags.network == "" {
		fmt.Printf("ERROR: you must specify the network and script\n")
		os.Exit(1)
	}

	network := ouroboros.NetworkByName(cmdlineFlags.network)
	if network == ouroboros.NetworkInvalid {
		fmt.Printf("ERROR: unknown named network: %s\n", network)
		os.Exit(1)
	}

	var scriptData []byte
	var err error
	if cmdlineFlags.scriptData != "" {
		scriptData, err = hex.DecodeString(cmdlineFlags.scriptData)
	} else {
		scriptData, err = os.ReadFile(cmdlineFlags.scriptPath)
	}
	if err != nil {
		fmt.Printf("ERROR: failed to read script file: %s\n", err)
		os.Exit(1)
	}
	//fmt.Printf("scriptData(%d) = %x\n", len(scriptData), scriptData)

	/*
		var innerScriptData []byte
		if _, err := cbor.Decode(scriptData, &innerScriptData); err != nil {
			fmt.Printf("ERROR: failed to unwrap outer CBOR for script: %s\n", err)
			os.Exit(1)
		}
	*/
	//fmt.Printf("innerScriptData(%d) = %x\n", len(innerScriptData), innerScriptData)
	/*
		innerScriptData = append(
			[]byte{byte(cmdlineFlags.plutusVersion)},
			innerScriptData...,
		)
		fmt.Printf("innerScriptData(%d) = %x\n", len(innerScriptData), innerScriptData)
	*/

	hash, _ := blake2b.New(28, nil)
	hash.Write([]byte{byte(cmdlineFlags.plutusVersion)})
	//hash.Write(innerScriptData[:])
	hash.Write(scriptData[:])
	scriptHash := hash.Sum(nil)
	//fmt.Printf("scriptHash(%d) = %x\n", len(scriptHash), scriptHash)

	address, _ := ledger.NewAddressFromParts(
		ledger.AddressTypeScriptNone,
		network.Id,
		scriptHash,
		nil,
	)

	fmt.Printf("Script hash:    %x\n", scriptHash)
	fmt.Printf("Script address: %s\n", address.String())
}
