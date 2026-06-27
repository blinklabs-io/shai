package config

import (
	"slices"

	"github.com/blinklabs-io/shai/dex"
)

// poolAddrs builds the monitored-address list for a protocol from the canonical
// locator registry in the public dex package, so the service config and the
// library share a single source of truth for pool script addresses.
func poolAddrs(protocol string) []ProfileConfigAddress {
	addrs := dex.PoolAddresses(protocol)
	out := make([]ProfileConfigAddress, 0, len(addrs))
	for _, a := range addrs {
		out = append(out, ProfileConfigAddress{Address: a})
	}
	return out
}

type ProfileType int

const (
	ProfileTypeNone ProfileType = iota
	ProfileTypeSpectrum
	ProfileTypeOracle
	ProfileTypeSynthetics
)

type Profile struct {
	Name          string
	Type          ProfileType
	InterceptSlot uint64
	InterceptHash string
	Config        any
}

type ProfileConfigInputRef struct {
	TxId      string
	OutputIdx uint
}

type SpectrumProfileConfig struct {
	SwapHash        string
	DepositHash     string
	RedeemHash      string
	PoolV1Hash      string
	PoolV2Hash      string
	SwapInputRef    ProfileConfigInputRef
	DepositInputRef ProfileConfigInputRef
	RedeemInputRef  ProfileConfigInputRef
	PoolV1InputRef  ProfileConfigInputRef
	PoolV2InputRef  ProfileConfigInputRef
}

// ProfileConfigAddress represents a script address to monitor
type ProfileConfigAddress struct {
	Address string
}

// OracleProfileConfig contains configuration for oracle price tracking
type OracleProfileConfig struct {
	Protocol      string                  // Protocol name (e.g., "minswap", "sundaeswap")
	PoolAddresses []ProfileConfigAddress  // Pool addresses to monitor
	InputRefs     []ProfileConfigInputRef // Reference inputs if needed
}

// SyntheticsProfileConfig contains configuration for synthetics protocols.
type SyntheticsProfileConfig struct {
	Protocol        string                 // Protocol name (e.g., "butane")
	CDPAddresses    []ProfileConfigAddress // CDP contract addresses to monitor
	OracleAddresses []ProfileConfigAddress // Oracle feed addresses
	PriceFeedPolicy string                 // Policy ID for price feed tokens
}

func GetProfiles() []Profile {
	var ret []Profile
	if networkProfiles, ok := Profiles[globalConfig.Network]; ok {
		for k, profile := range networkProfiles {
			if slices.Contains(globalConfig.Profiles, k) {
				ret = append(ret, profile)
			}
		}
	}
	return ret
}

func GetAvailableProfiles() []string {
	var ret []string
	if networkProfiles, ok := Profiles[globalConfig.Network]; ok {
		for k := range networkProfiles {
			ret = append(ret, k)
		}
	}
	return ret
}

var Profiles = map[string]map[string]Profile{
	"preprod": {
		"minswap-v2": {
			Name:          "minswap-v2",
			Type:          ProfileTypeOracle,
			InterceptSlot: 62910357,
			InterceptHash: "42d965bbc11668723b3bc3741a969c2750d2df0d16714a0af63b4ef2ee221eb1",
			Config: OracleProfileConfig{
				Protocol: "minswap-v2",
				PoolAddresses: []ProfileConfigAddress{
					// Minswap V2 pool script address (preprod)
					{
						Address: mustScriptEnterpriseAddress(
							"preprod",
							"d6ba9b7509eac866288ff5072d2a18205ac56f744bc82dcd808cb8fe",
						),
					},
				},
				InputRefs: []ProfileConfigInputRef{
					// Pool reference script
					{
						TxId:      "9f30b1c3948a009ceebda32d0b1d25699674b2eaf8b91ef029a43bfc1073ce28",
						OutputIdx: 0,
					},
				},
			},
		},
	},
	"preview": {
		"sundaeswap-v3": {
			Name:          "sundaeswap-v3",
			Type:          ProfileTypeOracle,
			InterceptSlot: 48535234,
			InterceptHash: "311319fd4889453ddbba6fde8b085cd1ec8d7d341e6128c003ad8cbbfbf09043",
			Config: OracleProfileConfig{
				Protocol: "sundaeswap-v3",
				PoolAddresses: []ProfileConfigAddress{
					// SundaeSwap V3 pool script address (preview)
					{
						Address: mustScriptScriptAddress(
							"preview",
							"44a1eb2d9f58add4eb1932bd0048e6a1947e85e3fe4f32956a110414",
							"cc27980a8557fe9db2c9ac0a2677f4d1306dbf10689983758f0b8dbe",
						),
					},
				},
				InputRefs: []ProfileConfigInputRef{
					// Order reference script
					{
						TxId:      "92ec2274938de291d3837b7facf9eddfaed57cd6ff97e26af57cb7a9978e3887",
						OutputIdx: 0,
					},
					// Pool reference script
					{
						TxId:      "8036a88a61427262aba964a42d0b9924739ffc3214de9a07c54b5a09af7f0d7d",
						OutputIdx: 0,
					},
				},
			},
		},
		"teddyswap": {
			Name:          "teddyswap",
			Type:          ProfileTypeSpectrum,
			InterceptSlot: 32045163,
			InterceptHash: "825568a8f7272fa8662c5a1fee156fe5dfb932ae8a47c8526b737399c9b3e836",
			Config: SpectrumProfileConfig{
				SwapHash:    "4ab17afc9a19a4f06b6fe229f9501e727d3968bff03acb1a8f86acf5",
				DepositHash: "0c70d8047139103546f0e76aafecfdf0667cbb397c8976f40ae8fcb3",
				RedeemHash:  "ab658d65b5717bf07bd3b1a9ad28d31c183811bba4076aeace9feb8e",
				PoolV2Hash:  "28bbd1f7aebb3bc59e13597f333aeefb8f5ab78eda962de1d605b388",
				SwapInputRef: ProfileConfigInputRef{
					"81bdfd89f3c8ff1a23dbe70af2db399ad0ed028b36a41974662a2cf8cda3c7c3",
					0,
				},
				DepositInputRef: ProfileConfigInputRef{
					"77186dc10826227acd5e4a48e636bd3b11d5f39cc051d794540a7125903e157c",
					0,
				},
				RedeemInputRef: ProfileConfigInputRef{
					"2266866d4d85cd582a34d27638a6eeb885cc4fb96fee230c86720e1f3f9eb0a0",
					0,
				},
				PoolV2InputRef: ProfileConfigInputRef{
					"64747d26baba95016a42c078360a431bb74d603f3f2582eb1b77d5dcfd53f128",
					0,
				},
			},
		},
	},
	"mainnet": {
		"minswap-v1": {
			Name:          "minswap-v1",
			Type:          ProfileTypeOracle,
			InterceptSlot: 51340496, // Minswap V1 deployment (March 2022)
			InterceptHash: "ba74da9715acfd2a01c88b52e41b574621c65c91e8eda35c9c3cd8e8c5f64d4c",
			Config: OracleProfileConfig{
				Protocol:      "minswap-v1",
				PoolAddresses: poolAddrs("minswap-v1"),
				InputRefs:     []ProfileConfigInputRef{},
			},
		},
		"minswap-v2": {
			Name:          "minswap-v2",
			Type:          ProfileTypeOracle,
			InterceptSlot: 72316896, // Minswap V2 deployment
			InterceptHash: "3e86a51cdabb354e5fe4b2511f91c4e8e323af5e50ef5eb2d5f3d5a7dab1f3b1",
			Config: OracleProfileConfig{
				Protocol:      "minswap-v2",
				PoolAddresses: poolAddrs("minswap-v2"),
				InputRefs: []ProfileConfigInputRef{
					// Pool reference script
					{
						TxId:      "2536194d2a976370a932174c10975493ab58fd7c16395d50e62b7c0e1949baea",
						OutputIdx: 0,
					},
				},
			},
		},
		"sundaeswap-v1": {
			Name:          "sundaeswap-v1",
			Type:          ProfileTypeOracle,
			InterceptSlot: 51337110, // First block at/after the V1 launch slot 51337095
			InterceptHash: "d1423d7eb6fc87d7ad1a54e44dd8cb70483370877346f3178dd507d7609046c8",
			Config: OracleProfileConfig{
				Protocol:      "sundaeswap-v1",
				PoolAddresses: poolAddrs("sundaeswap-v1"),
				InputRefs:     []ProfileConfigInputRef{},
			},
		},
		"sundaeswap-v3": {
			Name:          "sundaeswap-v3",
			Type:          ProfileTypeOracle,
			InterceptSlot: 123703740,
			InterceptHash: "c43d1bb5308d1ad7baa11120291ed2ba620784ebd96ae02a63c5511b3346581a",
			Config: OracleProfileConfig{
				Protocol:      "sundaeswap-v3",
				PoolAddresses: poolAddrs("sundaeswap-v3"),
				InputRefs:     []ProfileConfigInputRef{
					// TODO: Add reference inputs if needed
				},
			},
		},
		"splash-v1": {
			Name:          "splash-v1",
			Type:          ProfileTypeOracle,
			InterceptSlot: 98823654,                                                           // Splash deployment (rebrand of Spectrum)
			InterceptHash: "4666f26d15f4802c0d4c81b841583ea6d90d623d168c77f1e45200eda1f82638", // Splash deployment hash
			Config: OracleProfileConfig{
				Protocol:      "splash-v1",
				PoolAddresses: poolAddrs("splash-v1"),
				InputRefs: []ProfileConfigInputRef{
					// Pool reference script (same as Spectrum PoolV2)
					{
						TxId:      "fc9e99fd12a13a137725da61e57a410e36747d513b965993d92c32c67df9259a",
						OutputIdx: 0,
					},
				},
			},
		},
		"wingriders-v2": {
			Name:          "wingriders-v2",
			Type:          ProfileTypeOracle,
			InterceptSlot: 61318994, // WingRiders V2 launch
			InterceptHash: "c1c2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6c1c2",
			Config: OracleProfileConfig{
				Protocol:      "wingriders-v2",
				PoolAddresses: poolAddrs("wingriders-v2"),
				InputRefs:     []ProfileConfigInputRef{},
			},
		},
		"vyfi": {
			Name:          "vyfi",
			Type:          ProfileTypeOracle,
			InterceptSlot: 92346471, // VyFi launch (May 15, 2023)
			InterceptHash: "b0ce8bc8fbdcb803287e2b67d8eca865ef93be7e3d2473bfa81d89a8d58b7ee3",
			Config: OracleProfileConfig{
				Protocol:      "vyfi",
				PoolAddresses: poolAddrs("vyfi"),
				InputRefs:     []ProfileConfigInputRef{},
			},
		},
		"cswap": {
			Name: "cswap",
			Type: ProfileTypeOracle,
			// CSWAP DEX pool script launch (mainnet)
			InterceptSlot: 149650740,
			InterceptHash: "6027a8e3af4cd1cd2b3de0a1b583882573953c200c0ccf120119a04d1def5b49",
			Config: OracleProfileConfig{
				Protocol:      "cswap",
				PoolAddresses: poolAddrs("cswap"),
				InputRefs:     []ProfileConfigInputRef{},
			},
		},
		"indigo": {
			Name:          "indigo",
			Type:          ProfileTypeSynthetics,
			InterceptSlot: 75599947, // ~Oct 30 2022 (epoch 372)
			InterceptHash: "5af43c107055e2339c0d8d931c10679adb0b39ae0322cc521d7c327dcc6d816f",
			Config: SyntheticsProfileConfig{
				Protocol: "indigo",
				CDPAddresses: []ProfileConfigAddress{
					{
						Address: "addr1z8jd97ct35n4s5ss8lt4sq0zclw0dmf7yak8fj46m0jm3dhzfjvtm0pg7ms9fvw5luec6euwzku8wqjpt5gv0q86052qv9nxuw",
					},
					{
						Address: "addr1w80ptp0qgmcklhmeweesqgeurtlma8fsxsr9dt8au30fzss0czhl9",
					},
				},
			},
		},
		"butane": {
			Name:          "butane",
			Type:          ProfileTypeSynthetics,
			InterceptSlot: 145000000,
			InterceptHash: "0000000000000000000000000000000000000000000000000000000000000000",
			Config: SyntheticsProfileConfig{
				Protocol:        "butane",
				CDPAddresses:    []ProfileConfigAddress{},
				OracleAddresses: []ProfileConfigAddress{},
				PriceFeedPolicy: "b41d06ebccb6278d3ee7b4cd2faa321537156c9fd9c8dd40e95f91ea",
			},
		},
		"spectrum": {
			Name:          "spectrum",
			Type:          ProfileTypeSpectrum,
			InterceptSlot: 98823654,
			InterceptHash: "4666f26d15f4802c0d4c81b841583ea6d90d623d168c77f1e45200eda1f82638",
			Config: SpectrumProfileConfig{
				SwapHash:    "2618e94cdb06792f05ae9b1ec78b0231f4b7f4215b1b4cf52e6342de",
				DepositHash: "075e09eb0fa89e1dc34691b3c56a7f437e60ac5ea67b338f2e176e20",
				RedeemHash:  "83da79f531c19f9ce4d85359f56968a742cf05cc25ed3ca48c302dee",
				PoolV1Hash:  "e628bfd68c07a7a38fcd7d8df650812a9dfdbee54b1ed4c25c87ffbf",
				PoolV2Hash:  "6b9c456aa650cb808a9ab54326e039d5235ed69f069c9664a8fe5b69",
				SwapInputRef: ProfileConfigInputRef{
					"fc9e99fd12a13a137725da61e57a410e36747d513b965993d92c32c67df9259a",
					2,
				},
				DepositInputRef: ProfileConfigInputRef{
					"fc9e99fd12a13a137725da61e57a410e36747d513b965993d92c32c67df9259a",
					0,
				},
				RedeemInputRef: ProfileConfigInputRef{
					"fc9e99fd12a13a137725da61e57a410e36747d513b965993d92c32c67df9259a",
					1,
				},
				PoolV1InputRef: ProfileConfigInputRef{
					"31a497ef6b0033e66862546aa2928a1987f8db3b8f93c59febbe0f47b14a83c6",
					0,
				},
				PoolV2InputRef: ProfileConfigInputRef{
					"c8c93656e8bce07fabe2f42d703060b7c71bfa2e48a2956820d1bd81cc936faa",
					0,
				},
			},
		},
		"teddyswap": {
			Name:          "teddyswap",
			Type:          ProfileTypeSpectrum,
			InterceptSlot: 109076993,
			InterceptHash: "328bac757d1b100c68e0fd8f346a1bd53ee415b94271b8b7353866a22063f7bf",
			Config: SpectrumProfileConfig{
				SwapHash:    "4ab17afc9a19a4f06b6fe229f9501e727d3968bff03acb1a8f86acf5",
				DepositHash: "0c70d8047139103546f0e76aafecfdf0667cbb397c8976f40ae8fcb3",
				RedeemHash:  "ab658d65b5717bf07bd3b1a9ad28d31c183811bba4076aeace9feb8e",
				PoolV2Hash:  "28bbd1f7aebb3bc59e13597f333aeefb8f5ab78eda962de1d605b388",
				SwapInputRef: ProfileConfigInputRef{
					"fb6906c2bc39777086036f9c46c297e9d8a41ede154b398d85245a2549b4bf04",
					0,
				},
				DepositInputRef: ProfileConfigInputRef{
					"570f810fe5f8cef730587fb832bb70d8783bad711064d70fc1a378cbefdd7c94",
					0,
				},
				RedeemInputRef: ProfileConfigInputRef{
					"e33584ade2b47fb0ab697b63585fb4be935852131643981ba95acde09fe31f41",
					0,
				},
				PoolV2InputRef: ProfileConfigInputRef{
					"cdafc4e33524e767c4d0ffde094d56fa42105dcfc9b62857974f86fd0e443c32",
					0,
				},
			},
		},
	},
}
