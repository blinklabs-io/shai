package config

type ProfileType int

const (
	ProfileTypeNone ProfileType = iota
	ProfileTypeSpectrum
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

func GetProfiles() []Profile {
	var ret []Profile
	if networkProfiles, ok := Profiles[globalConfig.Network]; ok {
		for k, profile := range networkProfiles {
			for _, tmpProfile := range globalConfig.Profiles {
				if k == tmpProfile {
					ret = append(ret, profile)
					break
				}
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
	"preview": map[string]Profile{
		"teddyswap": Profile{
			Name:          "teddyswap",
			Type:          ProfileTypeSpectrum,
			InterceptSlot: 32045163,
			InterceptHash: "825568a8f7272fa8662c5a1fee156fe5dfb932ae8a47c8526b737399c9b3e836",
			Config: SpectrumProfileConfig{
				SwapHash:        "4ab17afc9a19a4f06b6fe229f9501e727d3968bff03acb1a8f86acf5",
				DepositHash:     "0c70d8047139103546f0e76aafecfdf0667cbb397c8976f40ae8fcb3",
				RedeemHash:      "ab658d65b5717bf07bd3b1a9ad28d31c183811bba4076aeace9feb8e",
				PoolV2Hash:      "28bbd1f7aebb3bc59e13597f333aeefb8f5ab78eda962de1d605b388",
				SwapInputRef:    ProfileConfigInputRef{"81bdfd89f3c8ff1a23dbe70af2db399ad0ed028b36a41974662a2cf8cda3c7c3", 0},
				DepositInputRef: ProfileConfigInputRef{"77186dc10826227acd5e4a48e636bd3b11d5f39cc051d794540a7125903e157c", 0},
				RedeemInputRef:  ProfileConfigInputRef{"2266866d4d85cd582a34d27638a6eeb885cc4fb96fee230c86720e1f3f9eb0a0", 0},
				PoolV2InputRef:  ProfileConfigInputRef{"64747d26baba95016a42c078360a431bb74d603f3f2582eb1b77d5dcfd53f128", 0},
			},
		},
	},
	"mainnet": map[string]Profile{
		"spectrum": Profile{
			Name:          "spectrum",
			Type:          ProfileTypeSpectrum,
			InterceptSlot: 98823654,
			InterceptHash: "4666f26d15f4802c0d4c81b841583ea6d90d623d168c77f1e45200eda1f82638",
			Config: SpectrumProfileConfig{
				SwapHash:        "2618e94cdb06792f05ae9b1ec78b0231f4b7f4215b1b4cf52e6342de",
				DepositHash:     "075e09eb0fa89e1dc34691b3c56a7f437e60ac5ea67b338f2e176e20",
				RedeemHash:      "83da79f531c19f9ce4d85359f56968a742cf05cc25ed3ca48c302dee",
				PoolV1Hash:      "e628bfd68c07a7a38fcd7d8df650812a9dfdbee54b1ed4c25c87ffbf",
				PoolV2Hash:      "6b9c456aa650cb808a9ab54326e039d5235ed69f069c9664a8fe5b69",
				SwapInputRef:    ProfileConfigInputRef{"fc9e99fd12a13a137725da61e57a410e36747d513b965993d92c32c67df9259a", 2},
				DepositInputRef: ProfileConfigInputRef{"fc9e99fd12a13a137725da61e57a410e36747d513b965993d92c32c67df9259a", 0},
				RedeemInputRef:  ProfileConfigInputRef{"fc9e99fd12a13a137725da61e57a410e36747d513b965993d92c32c67df9259a", 1},
				PoolV1InputRef:  ProfileConfigInputRef{"31a497ef6b0033e66862546aa2928a1987f8db3b8f93c59febbe0f47b14a83c6", 0},
				PoolV2InputRef:  ProfileConfigInputRef{"c8c93656e8bce07fabe2f42d703060b7c71bfa2e48a2956820d1bd81cc936faa", 0},
			},
		},
		"teddyswap": Profile{
			Name:          "teddyswap",
			Type:          ProfileTypeSpectrum,
			InterceptSlot: 109076993,
			InterceptHash: "328bac757d1b100c68e0fd8f346a1bd53ee415b94271b8b7353866a22063f7bf",
			Config: SpectrumProfileConfig{
				SwapHash:        "4ab17afc9a19a4f06b6fe229f9501e727d3968bff03acb1a8f86acf5",
				DepositHash:     "0c70d8047139103546f0e76aafecfdf0667cbb397c8976f40ae8fcb3",
				RedeemHash:      "ab658d65b5717bf07bd3b1a9ad28d31c183811bba4076aeace9feb8e",
				PoolV2Hash:      "28bbd1f7aebb3bc59e13597f333aeefb8f5ab78eda962de1d605b388",
				SwapInputRef:    ProfileConfigInputRef{"fb6906c2bc39777086036f9c46c297e9d8a41ede154b398d85245a2549b4bf04", 0},
				DepositInputRef: ProfileConfigInputRef{"570f810fe5f8cef730587fb832bb70d8783bad711064d70fc1a378cbefdd7c94", 0},
				RedeemInputRef:  ProfileConfigInputRef{"e33584ade2b47fb0ab697b63585fb4be935852131643981ba95acde09fe31f41", 0},
				PoolV2InputRef:  ProfileConfigInputRef{"cdafc4e33524e767c4d0ffde094d56fa42105dcfc9b62857974f86fd0e443c32", 0},
			},
		},
	},
}
