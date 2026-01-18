package spectrum

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"time"

	"github.com/Salvionied/apollo"
	serAddress "github.com/Salvionied/apollo/serialization/Address"
	"github.com/Salvionied/apollo/serialization/Key"
	"github.com/Salvionied/apollo/serialization/PlutusData"
	"github.com/Salvionied/apollo/serialization/Redeemer"
	"github.com/Salvionied/apollo/serialization/TransactionInput"
	"github.com/Salvionied/apollo/serialization/UTxO"

	// txBuildingUtils "github.com/Salvionied/apollo/txBuilding/Utils"
	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/shai/internal/config"
	"github.com/blinklabs-io/shai/internal/storage"
	"github.com/blinklabs-io/shai/internal/wallet"
)

const (
	swapTxTtlSlots = 30
	swapTxFee      = 295_000
)

/*
From mainnet TX 627a4e258e346ab5eaa3dcd4c66248c54698af2507d42944118de39b309d4e0a:

[
    {
        // Inputs
        0: [
            [
                h'7d0d434bd80d8a2fb9802fcc437ada8bd3f231e74058b4693e013ce1f8ae5604',
                0,
            ],
            [
                h'e0fa3fbeeedcfea69a4a8de71d696bd3c38bd5ae7852c96415aa667498b16f84',
                0,
            ],
        ],
        // Outputs
        1: [
	    // Pool address
            {
                // Address
                0: h'31e628bfd68c07a7a38fcd7d8df650812a9dfdbee54b1ed4c25c87ffbfb2f6abf60ccde92eae1a2f4fdf65f2eaf6208d872c6f0e597cc10b07',
                // Output amount/assets
                1: [
                    15062614004_3,
                    {
                        // ibtc_ADA_LQ
                        h'475362a850bf8d1f037794432cdea9fdbbf8d048a7c5115feeb7e91d': {h'696274635f4144415f4c51': 9223372036808097915_3},
			// ibtc_ADA_NFT
                        h'd8beceb1ac736c92df8e1210fb39803508533ae9573cffeb2b24a839': {h'696274635f4144415f4e4654': 1},
			// iBTC
                        h'f66d78b4a3cb3d37afa0ec36461e51ecbde00f26c8f0a68f94b69880': {h'69425443': 144656_2},
                    },
                ],
                // Datum option
                2: [
                    1,
                    24_0(<<121_0([_
                        121_0([_
                            h'd8beceb1ac736c92df8e1210fb39803508533ae9573cffeb2b24a839',
                            // ibtc_ADA_NFT
                            h'696274635f4144415f4e4654',
                        ]),
                        121_0([_ h'', h'']),
                        121_0([_
                            h'f66d78b4a3cb3d37afa0ec36461e51ecbde00f26c8f0a68f94b69880',
                            // iBTC
                            h'69425443',
                        ]),
                        121_0([_
                            h'475362a850bf8d1f037794432cdea9fdbbf8d048a7c5115feeb7e91d',
                            // ibtc_ADA_LQ
                            h'696274635f4144415f4c51',
                        ]),
                        997_1,
                        [_
                            h'f8d668a2d9dbf7d2b0cc74eb83b9c8ffa6235087676e97b8d5284522',
                        ],
                        20000000000_3,
                    ])>>),
                ],
            },
	    // Buyer
            [
                // Address
                h'01719bee424a97b58b3dca88fe5da6feac6494aa7226f975f3506c5b257846f6bb07f5b2825885e4502679e699b4e60a0c4609a46bc35454cd',
                // Amounts
                [
                    1418512_2,
                    {
                        // iBTC
                        h'f66d78b4a3cb3d37afa0ec36461e51ecbde00f26c8f0a68f94b69880': {h'69425443': 95_0},
                    },
                ],
            ],
	    // Matcher reward
            [
                // Address
                h'0166cbac5d856e5fc2d914f8ee2ebfc08732da4e3d1efeea27244b07c1cca89720456db673737ab27fa0ce106f3dc870266ebe6fcb42d903aa',
                // Amount
                1226226_2,
            ],
        ],
        // Fee
        2: 306032_2,
        // TTL
        3: 99111000_2,
        // Script data hash
        11: h'699320a2aa186dc067b80f12842246c6a9f85f617a07c3c78f47acd4b83132f8',
        // Collateral
        13: [
            [
                h'd1085e63731eb2b8786e7bba1735850e84dd4843cb63fd4a0cdb8242e4f083df',
                2,
            ],
        ],
        // Reference inputs
        18: [
            [
                h'fc9e99fd12a13a137725da61e57a410e36747d513b965993d92c32c67df9259a',
                2,
            ],
            [
                h'31a497ef6b0033e66862546aa2928a1987f8db3b8f93c59febbe0f47b14a83c6',
                0,
            ],
        ],
    },
    // Witness set
    {
        0: [
            [
                h'84297082268e97414e160f41c415c4e6678ffaa546fbf6ff6e725d9ba5d560e6',
                h'833853a6b01d7fbd81df280cae1780a170e0d41bf17df35c186ea89e0b2d660c107a3b496c808525da4657b73a1a6adeca9065e93ccd5332fbce369396093d02',
            ],
        ],
        // Redeemers
        5: [
            [
                0,
                0,
                121_0([_ 2, 0]),
                [520000_2, 155000000_2],
            ],
            [
                0,
                1,
                121_0([_ 0, 1, 1, 0]),
                [260000_2, 130000000_2],
            ],
        ],
    },
    true,
    null,
]

Input swap UTxO:

{
    0: h'712618e94cdb06792f05ae9b1ec78b0231f4b7f4215b1b4cf52e6342de',
    1: [12950770_2, {}],
    2: [
        1,
        24_0(<<121_0([_
            121_0([_ h'', h'']),
            121_0([_
                h'f66d78b4a3cb3d37afa0ec36461e51ecbde00f26c8f0a68f94b69880',
                // iBTC
                h'69425443',
            ]),
            121_0([_
                h'd8beceb1ac736c92df8e1210fb39803508533ae9573cffeb2b24a839',
                // ibtc_ADA_NFT
                h'696274635f4144415f4e4654',
            ]),
            997_1,
            16129032258064517_3,
            1000000000000_3,
            h'719bee424a97b58b3dca88fe5da6feac6494aa7226f975f3506c5b25',
            121_0([_
                h'7846f6bb07f5b2825885e4502679e699b4e60a0c4609a46bc35454cd',
            ]),
            10000000_2,
            93_0,
        ])>>),
    ],
}

Input swap datum:

121_0([_
    121_0([_ h'', h'']),
    121_0([_
        h'f66d78b4a3cb3d37afa0ec36461e51ecbde00f26c8f0a68f94b69880',
        // iBTC
        h'69425443',
    ]),
    121_0([_
        h'd8beceb1ac736c92df8e1210fb39803508533ae9573cffeb2b24a839',
        // ibtc_ADA_NFT
        h'696274635f4144415f4e4654',
    ]),
    997_1,
    16129032258064517_3,
    1000000000000_3,
    h'719bee424a97b58b3dca88fe5da6feac6494aa7226f975f3506c5b25',
    121_0([_
        h'7846f6bb07f5b2825885e4502679e699b4e60a0c4609a46bc35454cd',
    ]),
    10000000_2,
    93_0,
])

Input pool UTxO:

{
    0: h'31e628bfd68c07a7a38fcd7d8df650812a9dfdbee54b1ed4c25c87ffbfb2f6abf60ccde92eae1a2f4fdf65f2eaf6208d872c6f0e597cc10b07',
    1: [
        15052614004_3,
        {
            // ibtc_ADA_LQ
            h'475362a850bf8d1f037794432cdea9fdbbf8d048a7c5115feeb7e91d': {h'696274635f4144415f4c51': 9223372036808097915_3},
	    // ibtc_ADA_NFT
            h'd8beceb1ac736c92df8e1210fb39803508533ae9573cffeb2b24a839': {h'696274635f4144415f4e4654': 1},
	    // iBTC
            h'f66d78b4a3cb3d37afa0ec36461e51ecbde00f26c8f0a68f94b69880': {h'69425443': 144751_2},
        },
    ],
    2: [
        1,
        24_0(<<121_0([_
            121_0([_
                h'd8beceb1ac736c92df8e1210fb39803508533ae9573cffeb2b24a839',
                // ibtc_ADA_NFT
                h'696274635f4144415f4e4654',
            ]),
            121_0([_ h'', h'']),
            121_0([_
                h'f66d78b4a3cb3d37afa0ec36461e51ecbde00f26c8f0a68f94b69880',
                // iBTC
                h'69425443',
            ]),
            121_0([_
                h'475362a850bf8d1f037794432cdea9fdbbf8d048a7c5115feeb7e91d',
                // ibtc_ADA_LQ
                h'696274635f4144415f4c51',
            ]),
            997_1,
            [_
                h'f8d668a2d9dbf7d2b0cc74eb83b9c8ffa6235087676e97b8d5284522',
            ],
            20000000000_3,
        ])>>),
    ],
}

Input pool datum:

121_0([_
    121_0([_
        h'd8beceb1ac736c92df8e1210fb39803508533ae9573cffeb2b24a839',
        // ibtc_ADA_NFT
        h'696274635f4144415f4e4654',
    ]),
    121_0([_ h'', h'']),
    121_0([_
        h'f66d78b4a3cb3d37afa0ec36461e51ecbde00f26c8f0a68f94b69880',
        // iBTC
        h'69425443',
    ]),
    121_0([_
        h'475362a850bf8d1f037794432cdea9fdbbf8d048a7c5115feeb7e91d',
        // ibtc_ADA_LQ
        h'696274635f4144415f4c51',
    ]),
    997_1,
    [_
        h'f8d668a2d9dbf7d2b0cc74eb83b9c8ffa6235087676e97b8d5284522',
    ],
    20000000000_3,
])
*/

type createSwapTxOpts struct {
	poolUtxoBytes     []byte
	pool              *Pool
	swapUtxoBytes     []byte
	swapConfig        SwapConfig
	outputPoolAddress string
	poolInputRef      config.ProfileConfigInputRef
}

func (s *Spectrum) createSwapTx(opts createSwapTxOpts) ([]byte, error) {
	//cfg := config.GetConfig()
	//logger := logging.GetLogger()
	bursa := wallet.GetWallet()

	// Decode pool UTxO
	var poolUtxo UTxO.UTxO
	if _, err := cbor.Decode(opts.poolUtxoBytes, &poolUtxo); err != nil {
		return nil, err
	}

	// Decode swap UTxO
	var swapUtxo UTxO.UTxO
	if _, err := cbor.Decode(opts.swapUtxoBytes, &swapUtxo); err != nil {
		return nil, err
	}

	// Gather UTxOs from our wallet
	utxosBytes, err := storage.GetStorage().GetUtxos(bursa.PaymentAddress)
	if err != nil {
		return nil, err
	}
	utxos := []UTxO.UTxO{}
	for _, utxoBytes := range utxosBytes {
		var utxo UTxO.UTxO
		if _, err := cbor.Decode(utxoBytes, &utxo); err != nil {
			return nil, err
		}
		utxos = append(utxos, utxo)
	}

	// Calculate reward lovelace and asset amounts
	rewardAsset := opts.pool.OutputForInput(
		opts.swapConfig.Base,
		opts.swapConfig.BaseAmount,
	)
	// Validate reward amount is non-zero
	if rewardAsset.Amount == 0 {
		return nil, fmt.Errorf("calculated reward is zero")
	}
	if rewardAsset.Amount < opts.swapConfig.MinQuoteAmount {
		return nil, fmt.Errorf(
			"calculated reward asset amount (%d) is less than MinQuoteAmount (%d) in swap order",
			rewardAsset.Amount,
			opts.swapConfig.MinQuoteAmount,
		)
	}
	var rewardLovelace uint64
	var rewardUnits []apollo.Unit
	if rewardAsset.IsLovelace() {
		rewardLovelace = rewardAsset.Amount
	} else {
		rewardUnits = append(
			rewardUnits,
			apollo.NewUnit(
				hex.EncodeToString(rewardAsset.Class.PolicyId),
				string(rewardAsset.Class.Name),
				int(rewardAsset.Amount),
			),
		)
	}

	// Calculate lovelace and assets to return to pool
	poolReturnLovelace, poolReturnAssets, err := opts.pool.CalculateReturnToPool(
		AssetAmount{
			Class:  opts.swapConfig.Base,
			Amount: opts.swapConfig.BaseAmount,
		},
		rewardAsset,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate pool return: %w", err)
	}
	poolReturnUnits := []apollo.Unit{}
	for _, asset := range poolReturnAssets {
		poolReturnUnits = append(
			poolReturnUnits,
			apollo.NewUnit(
				hex.EncodeToString(asset.Class.PolicyId),
				string(asset.Class.Name),
				int(asset.Amount),
			),
		)
	}

	// Calculate matcher fee
	// We have to use big.Float here because we're dealing with multiplication and division of large numbers that overflow uint64
	// ( FeePerTokenNum / FeePerTokenDen ) * MinQuoteAmount
	// TODO: figure out why we're losing 1 lovelace to (probably) rounding
	// NOTE: this is division, but of course they won't call it that for some reason
	// Validate FeePerTokenDen to avoid division by zero
	if opts.swapConfig.FeePerTokenDen.Sign() == 0 {
		return nil, fmt.Errorf(
			"FeePerTokenDen is zero, cannot calculate matcher fee",
		)
	}
	matcherFee, _ := new(big.Float).Quo(
		new(big.Float).Mul(
			new(big.Float).SetUint64(opts.swapConfig.MinQuoteAmount),
			new(big.Float).SetInt(opts.swapConfig.FeePerTokenNum),
		),
		new(big.Float).SetInt(opts.swapConfig.FeePerTokenDen),
	).Uint64()

	// Calculate leftover lovelace from swap order UTxO for return with reward
	// Validate to prevent uint64 underflow
	swapCoin := uint64(swapUtxo.Output.GetAmount().GetCoin())
	if swapCoin < matcherFee {
		return nil, fmt.Errorf(
			"swap UTxO lovelace (%d) < matcher fee (%d)",
			swapCoin,
			matcherFee,
		)
	}
	leftoverSwapLovelace := swapCoin - matcherFee
	if len(opts.swapConfig.Base.PolicyId) == 0 {
		if leftoverSwapLovelace < opts.swapConfig.BaseAmount {
			return nil, fmt.Errorf(
				"leftover lovelace (%d) < base amount (%d)",
				leftoverSwapLovelace,
				opts.swapConfig.BaseAmount,
			)
		}
		leftoverSwapLovelace -= opts.swapConfig.BaseAmount
	}
	rewardLovelace += leftoverSwapLovelace

	// Generate addresses
	tmpRewardAddr := addressFromKeys(
		opts.swapConfig.RewardPkh,
		opts.swapConfig.StakePkh.Pkh,
	)
	rewardAddress, _ := serAddress.DecodeAddress(tmpRewardAddr)

	poolAddress, _ := serAddress.DecodeAddress(opts.outputPoolAddress)

	changeAddress, _ := serAddress.DecodeAddress(bursa.PaymentAddress)

	currentSlot := unixTimeToSlot(time.Now().Unix())

	// Determine sorted input indexes
	// This is necessary because redeemer indexes reflect the alphanumerically
	// sorted order of the TX inputs, and the smart contract uses the same mapping
	// for redeemer datum input indexes
	datumPoolInputIdx := sortedInputIndex(
		[]UTxO.UTxO{
			poolUtxo,
			swapUtxo,
		},
		poolUtxo.Input,
	)
	// We can safely assume that the swap input index is whichever one that the pool
	// input index isn't
	datumSwapInputIdx := 0
	if datumPoolInputIdx == 0 {
		datumSwapInputIdx = 1
	}

	// Validate change amount to prevent negative output
	changeAmount := int(matcherFee) - swapTxFee
	if changeAmount <= 0 {
		return nil, fmt.Errorf(
			"matcher fee (%d) <= tx fee (%d), no profit",
			matcherFee,
			swapTxFee,
		)
	}

	cc := apollo.NewEmptyBackend()
	apollob := apollo.New(&cc)
	apollob = apollob.
		//SetWalletFromBech32(bursa.PaymentAddress).
		//SetWalletAsChangeAddress().
		AddInputAddress(changeAddress).
		AddLoadedUTxOs(utxos...).
		SetTtl(int64(currentSlot+swapTxTtlSlots)).
		PayToContract(
			poolAddress,
			&PlutusData.PlutusData{
				Value: opts.pool.Datum,
			},
			int(poolReturnLovelace),
			true,
			poolReturnUnits...,
		).
		PayToAddress(
			rewardAddress, int(rewardLovelace), rewardUnits...,
		).
		PayToAddress(
			changeAddress, changeAmount,
		).
		AddReferenceInput(
			opts.poolInputRef.TxId,
			int(opts.poolInputRef.OutputIdx),
		).
		CollectFrom(
			poolUtxo,
			Redeemer.Redeemer{
				Tag: Redeemer.SPEND,
				// NOTE: these values are estimated
				ExUnits: Redeemer.ExecutionUnits{
					Mem:   530_000,
					Steps: 165_000_000,
				},
				Data: PlutusData.PlutusData{
					Value: cbor.NewConstructor(
						0,
						cbor.IndefLengthList{
							2,                 // action (swap)
							datumPoolInputIdx, // pool input index
						},
					),
				},
			},
		).
		AddReferenceInput(
			s.config.SwapInputRef.TxId,
			int(s.config.SwapInputRef.OutputIdx),
		).
		CollectFrom(
			swapUtxo,
			Redeemer.Redeemer{
				Tag: Redeemer.SPEND,
				// NOTE: these values are estimated
				ExUnits: Redeemer.ExecutionUnits{
					Mem:   270_000,
					Steps: 140_000_000,
				},
				Data: PlutusData.PlutusData{
					Value: cbor.NewConstructor(
						0,
						cbor.IndefLengthList{
							datumPoolInputIdx, // pool input index
							datumSwapInputIdx, // swap order input index
							1,                 // reward output index
							0,                 // action (apply)
						},
					),
				},
			},
		)

	tx, err := apollob.
		DisableExecutionUnitsEstimation().
		//Complete()
		CompleteExact(swapTxFee)
	if err != nil {
		return nil, err
	}
	vKeyBytes, err := hex.DecodeString(bursa.PaymentVKey.CborHex)
	if err != nil {
		return nil, err
	}
	sKeyBytes, err := hex.DecodeString(bursa.PaymentExtendedSKey.CborHex)
	if err != nil {
		return nil, err
	}
	// Strip off leading 2 bytes as shortcut for CBOR decoding to unwrap bytes
	vKeyBytes = vKeyBytes[2:]
	sKeyBytes = sKeyBytes[2:]
	// Strip out public key portion of extended private key
	sKeyBytes = append(sKeyBytes[:64], sKeyBytes[96:]...)
	vkey := Key.VerificationKey{Payload: vKeyBytes}
	skey := Key.SigningKey{Payload: sKeyBytes}
	tx, err = tx.SignWithSkey(vkey, skey)
	if err != nil {
		return nil, err
	}
	txBytes, err := tx.GetTx().Bytes()
	if err != nil {
		return nil, err
	}
	return txBytes, nil
}

func unixTimeToSlot(unixTime int64) uint64 {
	cfg := config.GetConfig()
	networkCfg := config.Networks[cfg.Network]
	return networkCfg.ShelleyOffsetSlot + uint64(
		unixTime-networkCfg.ShelleyOffsetTime,
	)
}

func sortedInputIndex(
	utxos []UTxO.UTxO,
	txInput TransactionInput.TransactionInput,
) int {
	sortedUtxos := apollo.SortInputs(utxos)
	for idx, utxo := range sortedUtxos {
		if string(utxo.Input.TransactionId) == string(txInput.TransactionId) {
			if utxo.Input.Index == txInput.Index {
				return idx
			}
		}
	}
	return -1
}
