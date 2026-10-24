package config

type Network struct {
	ShelleyOffsetSlot uint64
	ShelleyOffsetTime int64
}

var Networks = map[string]Network{
	"preprod": {
		// Shelley began at slot 86400 after four 20-second Byron epochs.
		ShelleyOffsetSlot: 86400,
		ShelleyOffsetTime: 1655769600,
	},
	"preview": {
		ShelleyOffsetSlot: 0,
		ShelleyOffsetTime: 1666656000,
	},
	"mainnet": {
		ShelleyOffsetSlot: 4924800,
		ShelleyOffsetTime: 1596491091,
	},
}
