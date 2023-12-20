package config

type ProfileType int

const (
	ProfileTypeNone ProfileType = iota
	ProfileTypeSpectrum
)

type Profile struct {
	Name           string
	Type           ProfileType
	InterceptSlot  uint64
	InterceptHash  string
	SwapAddress    string
	DepositAddress string
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
			Name:           "Teddy Swap",
			Type:           ProfileTypeSpectrum,
			InterceptSlot:  9113273,
			InterceptHash:  "427d8bf518d376d53627dd83302a000213454642e97d2eeddc19cdcc89abfe8b",
			SwapAddress:    "addr_test1wp9tz7hungv6furtdl3zn72sree86wtghlcr4jc637r2eagcksy0g",
			DepositAddress: "addr_test1wqx8pkqywyu3qd2x7rnk4tlvlhcxvl9m897gjah5pt50evc2v3m47",
		},
	},
	"mainnet": map[string]Profile{
		"spectrum": Profile{
			Name:           "Spectrum",
			Type:           ProfileTypeSpectrum,
			InterceptSlot:  98823654,
			InterceptHash:  "4666f26d15f4802c0d4c81b841583ea6d90d623d168c77f1e45200eda1f82638",
			SwapAddress:    "addr1wynp362vmvr8jtc946d3a3utqgclfdl5y9d3kn849e359hsskr20n",
			DepositAddress: "addr1wyr4uz0tp75fu8wrg6gm83t20aphuc9vt6n8kvu09ctkugqpsrmeh",
		},
		"teddyswap": Profile{
			Name:           "Teddy Swap",
			Type:           ProfileTypeSpectrum,
			InterceptSlot:  109076993,
			InterceptHash:  "328bac757d1b100c68e0fd8f346a1bd53ee415b94271b8b7353866a22063f7bf",
			SwapAddress:    "addr1z99tz7hungv6furtdl3zn72sree86wtghlcr4jc637r2eadkp2avt5gp297dnxhxcmy6kkptepsr5pa409qa7gf8stzs0706a3",
			DepositAddress: "addr1zyx8pkqywyu3qd2x7rnk4tlvlhcxvl9m897gjah5pt50evakp2avt5gp297dnxhxcmy6kkptepsr5pa409qa7gf8stzs6z6f9z",
		},
	},
}
