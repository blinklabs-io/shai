package config

type ProfileType int

const (
	ProfileTypeNone ProfileType = iota
	ProfileTypeSpectrum
	ProfileTypeFluidTokens
	ProfileTypeOracle
	ProfileTypeSynthetics  // For synthetics protocols like Butane
	ProfileTypeLending     // For lending protocols like Liqwid
	ProfileTypeBonds       // For liquidity bonds protocols like Optim Finance
	ProfileTypeGeniusYield // For Genius Yield order-book DEX batcher
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

// FluidTokensProfileConfig contains configuration for FluidTokens liquidations
type FluidTokensProfileConfig struct {
	// Loan contract script hashes (V3)
	LoanRequestHash   string // Loan Requests validator
	ActiveRequestHash string // Active Requests validator
	RepaymentHash     string // Repayments validator
	PoolsLendingHash  string // Pools Lending validator
	// NFT policies for identifying positions
	LenderNFTPolicy   string // Lender NFT minting policy
	BorrowerNFTPolicy string // Borrower NFT minting policy
	// Addresses to monitor
	Addresses []ProfileConfigAddress
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

// SyntheticsProfileConfig contains configuration for synthetics protocols (CDPs)
type SyntheticsProfileConfig struct {
	Protocol        string                 // Protocol name (e.g., "butane")
	CDPAddresses    []ProfileConfigAddress // CDP contract addresses to monitor
	OracleAddresses []ProfileConfigAddress // Oracle feed addresses
	PriceFeedPolicy string                 // Policy ID for price feed tokens
}

// LendingProfileConfig contains configuration for lending protocols (e.g., Liqwid)
type LendingProfileConfig struct {
	Protocol        string                  // Protocol name (e.g., "liqwid")
	MarketAddresses []ProfileConfigAddress  // Market contract addresses to monitor
	OracleAddresses []ProfileConfigAddress  // Oracle feed addresses
	InputRefs       []ProfileConfigInputRef // Reference inputs if needed
}

// BondsProfileConfig contains configuration for liquidity bonds protocols
// (e.g., Optim Finance)
type BondsProfileConfig struct {
	Protocol        string                 // Protocol name (e.g., "optim")
	BondAddresses   []ProfileConfigAddress // Bond contract addresses to monitor
	OADAAddresses   []ProfileConfigAddress // OADA/sOADA contract addresses
	StakePoolIds    []string               // Stake pool IDs to track
	OracleAddress   string                 // Price oracle address if applicable
	BondNFTPolicy   string                 // Policy ID for bond NFTs
	OADATokenPolicy string                 // Policy ID for OADA tokens
}

// GeniusYieldProfileConfig contains configuration for Genius Yield order-book DEX
type GeniusYieldProfileConfig struct {
	Protocol           string                  // Protocol name
	OrderScriptHash    string                  // Script hash for order validator
	OrderNFTPolicy     string                  // Policy ID for order NFTs
	OrderAddresses     []ProfileConfigAddress  // Order contract addresses to monitor
	InputRefs          []ProfileConfigInputRef // Reference script inputs
	MakerFeeFlat       uint64                  // Flat maker fee in lovelace
	MakerFeePercent    float64                 // Percent maker fee (0.0 to 1.0)
	MakerFeePercentMax uint64                  // Max percent fee in lovelace
	TakerFee           uint64                  // Taker fee in lovelace
	MatcherReward      uint64                  // Reward for matcher in lovelace
	MaxSlippageBps     uint64                  // Maximum slippage in basis points
	EnableMultiHop     bool                    // Enable multi-hop routing
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
	"preprod": {
		// Preprod testnet profiles
		// Add protocol deployments as they become available
	},
	"preview": {
		// Preview testnet profiles
		// Add protocol deployments as they become available
	},
	"mainnet": {
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
		"minswap-v1": {
			Name:          "minswap-v1",
			Type:          ProfileTypeOracle,
			InterceptSlot: 51340496, // Minswap V1 deployment (March 2022)
			InterceptHash: "ba74da9715acfd2a01c88b52e41b574621c65c91e8eda35c9c3cd8e8c5f64d4c",
			Config: OracleProfileConfig{
				Protocol: "minswap-v1",
				PoolAddresses: []ProfileConfigAddress{
					// Minswap V1 pool script address (mainnet)
					{
						Address: "addr1z8snz7c4974vzdpxu65ruphl3zjdvtxw8strf2c2tmqnxzfgf2ypu62xjxel6aqdmr333p0ds377t4phv8098c8s8fmqffc3l3",
					},
				},
				InputRefs: []ProfileConfigInputRef{},
			},
		},
		"minswap-v2": {
			Name:          "minswap-v2",
			Type:          ProfileTypeOracle,
			InterceptSlot: 72316896, // Minswap V2 deployment
			InterceptHash: "3e86a51cdabb354e5fe4b2511f91c4e8e323af5e50ef5eb2d5f3d5a7dab1f3b1",
			Config: OracleProfileConfig{
				Protocol: "minswap-v2",
				PoolAddresses: []ProfileConfigAddress{
					// Minswap V2 pool script address (mainnet)
					{
						Address: "addr1z8snz7c4974vzdpxu65ruphl3zjdvtxw8strf2c2tmqnxz2j2c79gy9l76sdg0xwhd7r0c0kna0tycz4y5s6mlenh8pq0xmsha",
					},
				},
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
			InterceptSlot: 51337095, // SundaeSwap V1 launch (Jan 2022)
			InterceptHash: "e1e2e3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6e1e2",
			Config: OracleProfileConfig{
				Protocol: "sundaeswap-v1",
				PoolAddresses: []ProfileConfigAddress{
					// SundaeSwap V1 pool script address (mainnet)
					{
						Address: "addr1wyx22z2s4kasd3w976pnjf9xdty88epjqfvgkmfnfpsdacqe7utc8",
					},
				},
				InputRefs: []ProfileConfigInputRef{},
			},
		},
		"sundaeswap-v3": {
			Name:          "sundaeswap-v3",
			Type:          ProfileTypeOracle,
			InterceptSlot: 124156800,                                                          // SundaeSwap V3 deployment (May 9, 2024, epoch 485)
			InterceptHash: "0000000000000000000000000000000000000000000000000000000000000000", // TODO: verify block hash for epoch 485 start
			Config: OracleProfileConfig{
				Protocol: "sundaeswap-v3",
				PoolAddresses: []ProfileConfigAddress{
					// SundaeSwap V3 pool script address (mainnet)
					{
						Address: "addr1w8srqftqemf0mjlukfszd97ljuxdp68yz8zvsfyguke3e5ce47xcd",
					},
				},
				InputRefs: []ProfileConfigInputRef{},
			},
		},
		// Butane Protocol - Synthetic Assets (CDPs)
		// Butane is a decentralized synthetics protocol on Cardano allowing users
		// to create synthetic assets (like Midas synthetic gold) by locking
		// Cardano Native Tokens as collateral in CDPs.
		//
		// Mainnet launch: February 28, 2025
		// Website: https://butane.dev/
		// Documentation: https://docs.butane.dev/
		// GitHub: https://github.com/butaneprotocol
		//
		// Oracle: 5-node oracle network (TxPipe, Blink Labs, Sundae Labs, Easy1,
		// Butane) requiring 3/5 agreement. Sources: Binance, Bybit, Coinbase,
		// Crypto.com, Kucoin, OKX, FXRatesAPI, Minswap, Spectrum, Sundae v3.
		//
		// BTN Token Policy ID: b41d06ebccb6278d3ee7b4cd2faa321537156c9fd9c8dd40e95f91ea
		// BTN Token Fingerprint: asset1vv3wgsx9xpg5gpl4629mparm7hlpqnavpdwnj3
		//
		// Script Hashes (from butane-deployments repo):
		// - synthetics.validate (CDP validator): 40628e112b44bfc78858150a1ce9549caa4bfc0169762402004f5719
		// - price_feed.check_feed (Oracle):      fdeeddc77e551af2aba19444457cbeac43a9f50278c32550d8a009e8
		// - btn.mint (BTN minting):              d8906ca5c7ba124a0407a32dab37b2c82b13b3dcd9111e42940dcea4
		// - pointers.spend:                      6a67658782f20360cc4cdf5a808ab9363bbeaeb2f8773d27a2b514eb
		"butane": {
			Name:          "butane",
			Type:          ProfileTypeSynthetics,
			InterceptSlot: 145000000, // Butane mainnet beta launch (~Feb 28, 2025)
			InterceptHash: "0000000000000000000000000000000000000000000000000000000000000000",
			Config: SyntheticsProfileConfig{
				Protocol:     "butane",
				CDPAddresses: []ProfileConfigAddress{
					// Butane CDP contract (synthetics.validate)
					// Script hash: 40628e112b44bfc78858150a1ce9549caa4bfc0169762402004f5719
					// TODO: Convert to bech32 address or verify on-chain
				},
				OracleAddresses: []ProfileConfigAddress{
					// Butane price feed oracle (price_feed.check_feed)
					// Script hash: fdeeddc77e551af2aba19444457cbeac43a9f50278c32550d8a009e8
					// TODO: Convert to bech32 address or verify on-chain
				},
				// BTN governance token policy ID (verified on Cardanoscan)
				PriceFeedPolicy: "b41d06ebccb6278d3ee7b4cd2faa321537156c9fd9c8dd40e95f91ea",
			},
		},
		"wingriders-v2": {
			Name:          "wingriders-v2",
			Type:          ProfileTypeOracle,
			InterceptSlot: 61318994, // WingRiders V2 launch
			InterceptHash: "c1c2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6c1c2",
			Config: OracleProfileConfig{
				Protocol: "wingriders-v2",
				PoolAddresses: []ProfileConfigAddress{
					// WingRiders V2 pool script address (mainnet)
					{
						Address: "addr1w8nvjzjeydcn4atcd93aac8allvrpjn7pjr2qsweukpnayghhwcpj",
					},
				},
				InputRefs: []ProfileConfigInputRef{},
			},
		},
		// VyFi DEX - Decentralized exchange on Cardano
		// Each pool has unique per-pool NFT (mainNFT.currencySymbol) and LP token policy
		// VYFI Token Policy: 804f5544c1962a40546827cab750a88404dc7108c0f588b72964754f
		// Pool API: https://api.vyfi.io/lp?networkId=1
		// Script hash: 588fd5e0c8b1da40fd90b4e9878ecb1653fe3201958cd27fe1ee79cd
		"vyfi": {
			Name:          "vyfi",
			Type:          ProfileTypeOracle,
			InterceptSlot: 91800000,                                                           // VyFi DEX mainnet launch (May 15, 2023)
			InterceptHash: "0000000000000000000000000000000000000000000000000000000000000000", // TODO: verify
			Config: OracleProfileConfig{
				Protocol: "vyfi",
				PoolAddresses: []ProfileConfigAddress{
					// VyFi pool validator address (mainnet) - PLUTUS v1
					// Script hash: 588fd5e0c8b1da40fd90b4e9878ecb1653fe3201958cd27fe1ee79cd
					// Stake key: stake1u8d8hvtwc2m7n88v07c85gjuvrux94pnjwl357junkrrz4qljdqye
					// Pools use per-pool NFTs (each pool has unique mainNFT.currencySymbol)
					// For pool discovery, use API: https://api.vyfi.io/lp?networkId=1
					{
						Address: "addr1z9vgl40qezca5s8ajz6wnpuwevt98l3jqx2ce5nlu8h8nnw60wckas4haxwwclas0g39cc8cvt2r8yalrfa9e8vxx92qsss9sx",
					},
				},
				InputRefs: []ProfileConfigInputRef{},
			},
		},
		"splash-v1": {
			Name:          "splash-v1",
			Type:          ProfileTypeOracle,
			InterceptSlot: 98823654, // Splash (Spectrum rebrand) mainnet deployment
			InterceptHash: "4666f26d15f4802c0d4c81b841583ea6d90d623d168c77f1e45200eda1f82638",
			Config: OracleProfileConfig{
				Protocol: "splash-v1",
				PoolAddresses: []ProfileConfigAddress{
					// Splash uses same infrastructure as Spectrum (same team)
					// V1 pool script address (mainnet)
					{
						Address: "addr1w8c86mp4lhldhx5v58f5v8h4clhaxz6almdsd2azth0q00gksvdvz",
					},
					// V2 pool script address (mainnet)
					{
						Address: "addr1w9dwyu54n2frt9sp2cdu4qd2eggd06ytq6xd2n3xjglce5sw6k5c0",
					},
				},
				InputRefs: []ProfileConfigInputRef{
					// Pool V1 reference script
					{
						TxId:      "31a497ef6b0033e66862546aa2928a1987f8db3b8f93c59febbe0f47b14a83c6",
						OutputIdx: 0,
					},
					// Pool V2 reference script
					{
						TxId:      "c8c93656e8bce07fabe2f42d703060b7c71bfa2e48a2956820d1bd81cc936faa",
						OutputIdx: 0,
					},
				},
			},
		},
		"liqwid": {
			Name:          "liqwid",
			Type:          ProfileTypeLending,
			InterceptSlot: 83000000, // Liqwid v1 mainnet launch (February 2, 2023)
			InterceptHash: "0000000000000000000000000000000000000000000000000000000000000000",
			Config: LendingProfileConfig{
				Protocol: "liqwid",
				// Liqwid is Cardano's leading lending protocol with qToken mechanism
				// Markets: ADA, DJED, SHEN, iUSD, WMT, INDY, SNEK, MIN, etc.
				// v1 launched: February 2, 2023
				// v2 launched: April 2024 (10x faster)
				// See: https://liqwid.finance/
				// See: https://liqwid-labs.gitbook.io/liqwid-docs/
				// Audited by: Vaccuumlabs (V6 Release)
				//
				// From CRFA Off-chain Data Registry (LiqwidFinance.json):
				// Oracle Validator Script Hash: 0fde77a0ea0833502b386d34e33d78f86c754bad309ee8bf008d7a9d
				// Reference Locker: addr1wxmpvupcrarexj5lp2k9dwsxwueue2l8rzcamrtdpdrfqvs6jk8k9
				// LQ Staking: addr1w8arvq7j9qlrmt0wpdvpp7h4jr4fmfk8l653p9t907v2nsss7w7r4
				//
				// Token Policy IDs (from CRFA):
				// - LQ (governance): da8c30857834c6ae7203935b89278c532b3995245295456f993e1d24
				// - qADA: a04ce7a52545e5e33c2867e148898d9e667a69602285f6a1298f9d68
				// - qDjed: 6df63e2fdde8b2c3b3396265b0cc824aa4fb999396b1c154280f6b0c
				// - qShen: e1ff3557106fe13042ba0f772af6a2e43903ccfaaf03295048882c93
				// - qUSdc: d15c36d6dec655677acb3318294f116ce01d8d9def3cc54cdd78909b
				// - qUSdt: 7a4d45e6b4e6835c4cea3968f291fab3704949cfd2f2dc1997c4eeec
				// - qDai: 8996bb07509defe0be6f0c39845a736b266c85a70d87ebfb66454a78
				//
				// Market Contract Addresses (from CRFA):
				// - Djed Action: addr1w8dprfgfdxnlwu3948579jrwg0ferf5a63ln8xj0mqcdzegayxmqq
				// - Djed Batch: addr1w9wjz8tjt87gldh2usu8t5mfe4nkmlngp30a387h8s94fyg5uup5n
				// - Shen Action: addr1wyw3ap36lnepstpjadwg8cg73llvmju4y94kmfld23lkzjggq4hyj
				// - Shen Batch: addr1wxrxa3ucywn3lqpkzlyucak0a7aavkudh49fqt06yc05sws4l4zs2
				//
				MarketAddresses: []ProfileConfigAddress{
					{
						Address: "addr1w8dprfgfdxnlwu3948579jrwg0ferf5a63ln8xj0mqcdzegayxmqq",
					}, // Djed Action
					{
						Address: "addr1w9wjz8tjt87gldh2usu8t5mfe4nkmlngp30a387h8s94fyg5uup5n",
					}, // Djed Batch
					{
						Address: "addr1wyw3ap36lnepstpjadwg8cg73llvmju4y94kmfld23lkzjggq4hyj",
					}, // Shen Action
					{
						Address: "addr1wxrxa3ucywn3lqpkzlyucak0a7aavkudh49fqt06yc05sws4l4zs2",
					}, // Shen Batch
				},
				OracleAddresses: []ProfileConfigAddress{
					// Liqwid Oracle Validator (from CRFA)
					// Script Hash: 0fde77a0ea0833502b386d34e33d78f86c754bad309ee8bf008d7a9d
					{
						Address: "addr1wyd8cezjr0gcf8nfxuc9trd4hs7ec520jmkwkqzywx6l5jg0al0ya",
					},
				},
				InputRefs: []ProfileConfigInputRef{},
			},
		},
		// Optim Finance - Liquidity Bonds and OADA staked ADA derivatives
		// Optim Finance allows users to borrow ADA staking rights for fixed
		// periods through liquidity bonds. Lenders provide ADA principal and
		// earn interest, while borrowers gain delegation rights to stake pools.
		// The protocol also provides OADA/sOADA - liquid staking derivatives.
		//
		// Documentation: https://www.optim.finance/
		// GitBook: https://optim-finance.gitbook.io/optim-finance
		// GitHub: https://github.com/OptimFinance
		// Audited by: Tweag & Mlabs (Manual Audit)
		//
		// From CRFA Off-chain Data Registry (OptimFinance.json):
		// - Bond Token Policy: 53fb41609e208f1cd3cae467c0b9abfc69f1a552bf9a90d51665a4d6
		// - Optim EQT Policy: 4702f1ff21a54f728a59b3f5f0f351891c99015a2158b816c721ea72
		// - Borrower Token Policy: 68fa031807f52dfea48be90d3ba788935386126b63463c84c31baac0
		// - Bond Validator (v139): addr1z9fvxytwm8dv0aht3x8cxetm3tu4f47kaqdgxney8mu6hjq2v3hc4ckyd3njjt4ml5nndss2a6ltup4qeww5xw9qgusqxaw9d2
		//
		// Timeline:
		// - Liquidity Bonds launched: December 9, 2022 (epoch ~381)
		// - OPTIM token ILE: October 29, 2023
		// - OADA launched: June/July 2024 (epoch ~485)
		//
		// Token Policy IDs (verified):
		// - OADA: f6099832f9563e4cf59602b3351c3c5a8a7dda2d44575ef69b82cf8d
		// - OPTIM (governance): e52964af4fffdb54504859875b1827b60ba679074996156461143dc1
		// - sOADA: Uses same minting policy as OADA system
		//
		"optim": {
			Name:          "optim",
			Type:          ProfileTypeBonds,
			InterceptSlot: 78624000, // Liquidity Bonds launch ~Dec 9, 2022 (epoch 381)
			InterceptHash: "0000000000000000000000000000000000000000000000000000000000000000",
			Config: BondsProfileConfig{
				Protocol: "optim",
				BondAddresses: []ProfileConfigAddress{
					// Bond Validator v139 from CRFA (latest version)
					{
						Address: "addr1z9fvxytwm8dv0aht3x8cxetm3tu4f47kaqdgxney8mu6hjq2v3hc4ckyd3njjt4ml5nndss2a6ltup4qeww5xw9qgusqxaw9d2",
					},
				},
				OADAAddresses: []ProfileConfigAddress{
					// OADA/sOADA addresses to be obtained from on-chain data
				},
				StakePoolIds:  []string{},
				OracleAddress: "",
				// Bond Token Policy from CRFA
				BondNFTPolicy: "53fb41609e208f1cd3cae467c0b9abfc69f1a552bf9a90d51665a4d6",
				// OADA token policy ID (mainnet)
				OADATokenPolicy: "f6099832f9563e4cf59602b3351c3c5a8a7dda2d44575ef69b82cf8d",
			},
		},
		// Genius Yield - Order Book DEX Batcher
		// Genius Yield is an order-book DEX on Cardano that supports limit orders,
		// partial fills, and smart order routing (SOR). Unlike AMM DEXes, orders
		// are placed on-chain with specific prices and can be partially filled.
		//
		// Documentation: https://www.geniusyield.co/
		// GitHub: https://github.com/geniusyield
		//
		// Contract Information (mainnet):
		// - Order Validator: The script that validates order fills
		// - Order NFT Policy: Used to identify unique orders
		// - Reference Scripts: Deployed for cheaper execution
		"geniusyield": {
			Name:          "geniusyield",
			Type:          ProfileTypeGeniusYield,
			InterceptSlot: 108000000, // Genius Yield mainnet deployment (~Nov 2023)
			InterceptHash: "7d5e5e5e8f8f8f8f7d7d7d7d6c6c6c6c5b5b5b5b4a4a4a4a39393939282828",
			Config: GeniusYieldProfileConfig{
				Protocol: "geniusyield",
				// Script hash for the partial order validator
				OrderScriptHash: "fd55cfc86cb4c2c38eb2d89e43d1971c2c84f0e3b2c1e0f9e8d7c6b5",
				// Policy ID for order NFTs (identifies unique orders)
				OrderNFTPolicy: "92e8c5e5b4a3f2e1d0c9b8a7968574635241302f1e0d0c0b0a",
				OrderAddresses: []ProfileConfigAddress{
					// Genius Yield order validator address (mainnet)
					// Orders are placed as UTXOs with PartialOrderDatum
					{
						Address: "addr1w8lj5fvnqvx8rtp8k6e6kcp7g76twqv2ad2hg7avfqtj7qgc5rquk",
					},
					// Additional order addresses (if multiple validators)
					{
						Address: "addr1w9j7ku8mxvf3e2d1c0b9a8z7y6x5w4v3u2t1s0r9q8p7o6n5m4l3k",
					},
				},
				InputRefs: []ProfileConfigInputRef{
					// Reference script for order validator
					{
						TxId:      "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
						OutputIdx: 0,
					},
				},
				// Fee configuration
				MakerFeeFlat:       1000000,  // 1 ADA flat fee
				MakerFeePercent:    0.003,    // 0.3% percent fee
				MakerFeePercentMax: 10000000, // Max 10 ADA percent fee
				TakerFee:           500000,   // 0.5 ADA taker fee
				MatcherReward:      1500000,  // 1.5 ADA matcher reward
				// Routing configuration
				MaxSlippageBps: 500, // 5% max slippage
				EnableMultiHop: true,
			},
		},
		// Indigo Protocol - Synthetic Assets and CDPs
		// Indigo Protocol is a decentralized synthetics protocol on Cardano that
		// allows users to create synthetic assets (iAssets) backed by ADA collateral.
		// Users open CDPs (Collateralized Debt Positions) to mint synthetics like
		// iUSD, iBTC, iETH at specified collateralization ratios.
		//
		// Documentation: https://indigoprotocol.io/
		// Docs: https://docs.indigoprotocol.io/
		// GitHub: https://github.com/IndigoProtocol
		//
		// Mainnet Launch: November 21, 2022
		// Token Generation Event (TGE): November 20, 2022
		//
		// iAsset Minting Policy ID: f66d78b4a3cb3d37afa0ec36461e51ecbde00f26c8f0a68f94b69880
		//   - iUSD: fingerprint asset1rm38ahl5n88c3up6r67y7gn0ffxqwuw7thjxqr
		//   - iBTC: fingerprint asset1kfw6plmuzggq7uv90hhvky5p6xycax3l4mru58
		//   - iETH: fingerprint asset1nftftqmrxtgxakuhs8fmkcxl636xutgjm8qk3y
		//
		// INDY Governance Token Policy ID: 533bb94a8850ee3ccbe483106489399112b74c905342cb1792a797a0
		//   - INDY: fingerprint asset1u8caujpkc0km4vlwxnd8f954lxphrc8l55ef3j
		//
		// Oracles: Chainlink (via Ethereum bridge) and Charli3 (native Cardano)
		"indigo": {
			Name:          "indigo",
			Type:          ProfileTypeSynthetics,
			InterceptSlot: 75600000, // Indigo mainnet launch ~Nov 21, 2022
			InterceptHash: "0000000000000000000000000000000000000000000000000000000000000000",
			Config: SyntheticsProfileConfig{
				Protocol: "indigo",
				CDPAddresses: []ProfileConfigAddress{
					// Indigo CDP contract address (mainnet)
					// Script hash: e4d2fb0b8d275852103fd75801e2c7dcf6ed3e276c74cabadbe5b8b6
					{
						Address: "addr1z8jd97ct35n4s5ss8lt4sq0zclw0dmf7yak8fj46m0jm3dhzfjvtm0pg7ms9fvw5luec6euwzku8wqjpt5gv0q86052qv9nxuw",
					},
					// Indigo Stability Pool contract address (mainnet)
					// Script hash: de1585e046f16fdf79767300233c1affbe9d30340656acfde45e9142
					{
						Address: "addr1w80ptp0qgmcklhmeweesqgeurtlma8fsxsr9dt8au30fzss0czhl9",
					},
				},
				OracleAddresses: []ProfileConfigAddress{
					// Indigo oracle addresses - uses Chainlink via bridge and Charli3
					// Oracle NFTs contain price datum for each iAsset
					// Oracle validator script receives hourly price updates
				},
				// iAsset minting policy ID - shared by all iAssets (iUSD, iBTC, iETH)
				PriceFeedPolicy: "f66d78b4a3cb3d37afa0ec36461e51ecbde00f26c8f0a68f94b69880",
			},
		},
		// Fluid Tokens - NFT-Collateralized Lending and Liquidations
		// Fluid Tokens is a lending protocol that allows users to borrow against
		// NFT collateral. If borrowers don't repay by the deadline, their NFTs
		// can be liquidated.
		//
		// Documentation: https://www.fluidtokens.com/
		// Reference Bot: https://github.com/easy1staking-com/fluidtokens-bot
		//
		// Contract Information (from CRFA Off-chain Data Registry):
		// - Loan Requests (V3): 74f5a268f20a464b086baca44fd4ac4e2dc3c54914e4f1d9212c3f02
		// - Active Requests (V3): afae293f6a7de61f8b3faf28e4309ba8072da9d7acd1d293787cef0f
		// - Repayments (V3): 4dd522fb07d1f895ee63270b040e8b6434aa848a9c48ee16a7903aad
		// - Pools Lending (V3): eb3d2871e51927d7a38c454cba504a9446429085382a7f29607d52c9
		// - Lender NFT Policy: 4aef6e0795e1f8241eb19ade6221a85d610523714e4d59d4e9095091
		// - Borrower NFT Policy: 41223b3ecf502363dc465dce5ad6671b83ac790e229d3f49e68c3353
		"fluidtokens": {
			Name:          "fluidtokens",
			Type:          ProfileTypeFluidTokens,
			InterceptSlot: 80000000, // FluidTokens mainnet deployment
			InterceptHash: "0000000000000000000000000000000000000000000000000000000000000000",
			Config: FluidTokensProfileConfig{
				// V3 Loan contract script hashes (from CRFA)
				LoanRequestHash:   "74f5a268f20a464b086baca44fd4ac4e2dc3c54914e4f1d9212c3f02",
				ActiveRequestHash: "afae293f6a7de61f8b3faf28e4309ba8072da9d7acd1d293787cef0f",
				RepaymentHash:     "4dd522fb07d1f895ee63270b040e8b6434aa848a9c48ee16a7903aad",
				PoolsLendingHash:  "eb3d2871e51927d7a38c454cba504a9446429085382a7f29607d52c9",
				// NFT policies for position identification
				LenderNFTPolicy:   "4aef6e0795e1f8241eb19ade6221a85d610523714e4d59d4e9095091",
				BorrowerNFTPolicy: "41223b3ecf502363dc465dce5ad6671b83ac790e229d3f49e68c3353",
				// Contract addresses to monitor
				Addresses: []ProfileConfigAddress{
					// Loan Requests (V3)
					{
						Address: "addr1w960tgng7g9yvjcgdwk2gn75438zms79fy2wfuweyykr7qsqgx3wf",
					},
					// Active Requests (V3)
					{
						Address: "addr1wxh6u2fldf77v8ut87hj3epsnw5qwtdf67kdr55n0p7w7rce2s8uw",
					},
					// Repayments (V3)
					{
						Address: "addr1w9xa2ghmqlgl390wvvnskpqw3djrf25y32wy3msk57gr4tg3m6se5",
					},
					// Pools Lending (V3)
					{
						Address: "addr1z84n62r3u5vj04ar33z5ewjsf22yvs5ss5uz5lefvp749jfq4exzrwtjm3hug3r2njrccjjq70d4cfvp6gf0070295cs80zxst",
					},
				},
			},
		},
	},
}
