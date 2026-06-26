package spectrum

import (
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/blinklabs-io/apollo/v2"
	"github.com/blinklabs-io/apollo/v2/backend/fixed"
	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/ledger/common"
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
	poolUtxo, err := decodeUtxo(opts.poolUtxoBytes)
	if err != nil {
		return nil, err
	}

	// Decode swap UTxO
	swapUtxo, err := decodeUtxo(opts.swapUtxoBytes)
	if err != nil {
		return nil, err
	}

	// Gather UTxOs from our wallet
	utxosBytes, err := storage.GetStorage().GetUtxos(bursa.PaymentAddress)
	if err != nil {
		return nil, err
	}
	utxos := []common.Utxo{}
	for _, utxoBytes := range utxosBytes {
		utxo, err := decodeUtxo(utxoBytes)
		if err != nil {
			return nil, err
		}
		utxos = append(utxos, utxo)
	}
	collateralUtxo, err := selectCollateralUtxo(utxos)
	if err != nil {
		return nil, err
	}

	// Calculate reward lovelace and asset amounts
	rewardAsset := opts.pool.OutputForInput(
		opts.swapConfig.Base,
		opts.swapConfig.BaseAmount,
	)
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
				int64(rewardAsset.Amount),
			),
		)
	}

	// Reward amount must be non-zero, otherwise the swap order is malformed
	// and there is nothing to pay out to the trader.
	if rewardAsset.Amount == 0 {
		return nil, errors.New(
			"calculated reward amount is zero for swap order",
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
		return nil, fmt.Errorf("pool return calculation failed: %w", err)
	}
	poolReturnUnits := []apollo.Unit{}
	for _, asset := range poolReturnAssets {
		poolReturnUnits = append(
			poolReturnUnits,
			apollo.NewUnit(
				hex.EncodeToString(asset.Class.PolicyId),
				string(asset.Class.Name),
				int64(asset.Amount),
			),
		)
	}

	// Guard against a nil numerator, which would panic in big.Float.SetInt
	// during the matcher fee calculation below.
	if opts.swapConfig.FeePerTokenNum == nil {
		return nil, errors.New(
			"FeePerTokenNum is nil for swap order",
		)
	}
	// Guard against division-by-zero in the matcher fee calculation below.
	if opts.swapConfig.FeePerTokenDen == nil ||
		opts.swapConfig.FeePerTokenDen.Sign() == 0 {
		return nil, errors.New(
			"FeePerTokenDen is zero for swap order",
		)
	}

	// Calculate matcher fee
	// We have to use big.Float here because we're dealing with multiplication and division of large numbers that overflow uint64
	// ( FeePerTokenNum / FeePerTokenDen ) * MinQuoteAmount
	// TODO: figure out why we're losing 1 lovelace to (probably) rounding
	// NOTE: this is division, but of course they won't call it that for some reason
	matcherFee, _ := new(big.Float).Quo(
		new(big.Float).Mul(
			new(big.Float).SetUint64(opts.swapConfig.MinQuoteAmount),
			new(big.Float).SetInt(opts.swapConfig.FeePerTokenNum),
		),
		new(big.Float).SetInt(opts.swapConfig.FeePerTokenDen),
	).Uint64()

	// Calculate leftover lovelace from swap order UTxO for return with reward
	swapUtxoCoin := swapUtxo.Output.Amount().Uint64()
	// Guard against uint64 underflow: the swap UTxO must hold at least the
	// matcher fee.
	if swapUtxoCoin < matcherFee {
		return nil, fmt.Errorf(
			"swap UTxO coin (%d) is less than matcher fee (%d)",
			swapUtxoCoin,
			matcherFee,
		)
	}
	leftoverSwapLovelace := swapUtxoCoin - matcherFee
	if len(opts.swapConfig.Base.PolicyId) == 0 {
		// Guard against uint64 underflow: for ADA-base swaps the leftover
		// lovelace must cover the base amount being sent to the pool.
		if leftoverSwapLovelace < opts.swapConfig.BaseAmount {
			return nil, fmt.Errorf(
				"leftover swap lovelace (%d) is less than base amount (%d)",
				leftoverSwapLovelace,
				opts.swapConfig.BaseAmount,
			)
		}
		leftoverSwapLovelace -= opts.swapConfig.BaseAmount
	}
	rewardLovelace += leftoverSwapLovelace

	// Guard against a non-positive change output: the matcher fee must exceed
	// the transaction fee, otherwise the change output below would underflow.
	if matcherFee <= swapTxFee {
		return nil, fmt.Errorf(
			"matcher fee (%d) does not exceed tx fee (%d)",
			matcherFee,
			swapTxFee,
		)
	}

	// Generate addresses
	tmpRewardAddr := addressFromKeys(
		opts.swapConfig.RewardPkh,
		opts.swapConfig.StakePkh.Pkh,
	)
	rewardAddress, err := common.NewAddress(tmpRewardAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse reward address: %w", err)
	}

	poolAddress, err := common.NewAddress(opts.outputPoolAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to parse pool address: %w", err)
	}

	changeAddress, err := common.NewAddress(bursa.PaymentAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to parse change address: %w", err)
	}

	currentSlot := unixTimeToSlot(time.Now().Unix())

	// Determine sorted input indexes
	// This is necessary because redeemer indexes reflect the alphanumerically
	// sorted order of the TX inputs, and the smart contract uses the same mapping
	// for redeemer datum input indexes
	datumPoolInputIdx := sortedInputIndex(
		[]common.Utxo{
			poolUtxo,
			swapUtxo,
		},
		poolUtxo.Id,
	)
	// We can safely assume that the swap input index is whichever one that the pool
	// input index isn't
	datumSwapInputIdx := 0
	if datumPoolInputIdx == 0 {
		datumSwapInputIdx = 1
	}

	// Build the pool datum from the pool's stored Plutus-data CBOR so the
	// returned pool output carries the exact datum bytes seen on-chain.
	var poolDatum common.Datum
	if _, err := cbor.Decode(opts.pool.Datum.Cbor(), &poolDatum); err != nil {
		return nil, fmt.Errorf("failed to decode pool datum: %w", err)
	}

	// Build the spend redeemers as Plutus constructor data. We encode the same
	// constructor/indefinite-list structure used on-chain and round-trip it
	// through CBOR so the bytes are identical to what the validator expects.
	poolRedeemer, err := plutusDatum(cbor.NewConstructorEncoder(
		0,
		cbor.IndefLengthList{
			2,                 // action (swap)
			datumPoolInputIdx, // pool input index
		},
	))
	if err != nil {
		return nil, fmt.Errorf("failed to build pool redeemer: %w", err)
	}
	swapRedeemer, err := plutusDatum(cbor.NewConstructorEncoder(
		0,
		cbor.IndefLengthList{
			datumPoolInputIdx, // pool input index
			datumSwapInputIdx, // swap order input index
			1,                 // reward output index
			0,                 // action (apply)
		},
	))
	if err != nil {
		return nil, fmt.Errorf("failed to build swap redeemer: %w", err)
	}

	apollob := apollo.New(fixed.NewEmptyFixedChainContext())
	// Set the wallet from our mnemonic so signing produces correct CIP-1852
	// extended Ed25519 signatures via bursa.
	apollob, err = apollob.SetWalletFromMnemonic(bursa.Mnemonic)
	if err != nil {
		return nil, fmt.Errorf("failed to set wallet: %w", err)
	}
	apollob = apollob.
		SetChangeAddress(changeAddress).
		AddCollateral(collateralUtxo).
		SetTtl(int64(currentSlot+swapTxTtlSlots)).
		PayToContract(
			poolAddress,
			&poolDatum,
			int64(poolReturnLovelace),
			poolReturnUnits...,
		).
		PayToAddress(
			rewardAddress, int64(rewardLovelace), rewardUnits...,
		).
		PayToAddress(
			changeAddress, int64(matcherFee)-swapTxFee,
		)
	// AddReferenceInput returns an error in v2, so it can no longer be chained.
	apollob, err = apollob.AddReferenceInput(
		opts.poolInputRef.TxId,
		int(opts.poolInputRef.OutputIdx),
	)
	if err != nil {
		return nil, err
	}
	apollob = apollob.CollectFrom(
		poolUtxo,
		poolRedeemer,
		// NOTE: these values are estimated
		common.ExUnits{Memory: 530_000, Steps: 165_000_000},
	)
	apollob, err = apollob.AddReferenceInput(
		s.config.SwapInputRef.TxId,
		int(s.config.SwapInputRef.OutputIdx),
	)
	if err != nil {
		return nil, err
	}
	apollob = apollob.CollectFrom(
		swapUtxo,
		swapRedeemer,
		// NOTE: these values are estimated
		common.ExUnits{Memory: 270_000, Steps: 140_000_000},
	)

	// CompleteExact(fee) is replaced in v2 by SetFee(fee) + Complete().
	tx, err := apollob.
		DisableExecutionUnitsEstimation().
		SetFee(swapTxFee).
		Complete()
	if err != nil {
		return nil, err
	}
	if err := validateSwapTxInputs(
		tx.GetTx().Inputs(),
		poolUtxo.Id,
		swapUtxo.Id,
		datumPoolInputIdx,
		datumSwapInputIdx,
	); err != nil {
		return nil, err
	}
	tx, err = tx.Sign()
	if err != nil {
		return nil, err
	}
	txBytes, err := tx.GetTxCbor()
	if err != nil {
		return nil, err
	}
	decodedTx, err := ledger.NewTransactionFromCbor(
		ledger.TxTypeConway,
		txBytes,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to decode built transaction CBOR: %w", err)
	}
	if err := validateSwapTxInputs(
		decodedTx.Inputs(),
		poolUtxo.Id,
		swapUtxo.Id,
		datumPoolInputIdx,
		datumSwapInputIdx,
	); err != nil {
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
	utxos []common.Utxo,
	txInput common.TransactionInput,
) int {
	sortedUtxos := apollo.SortInputs(utxos)
	for idx, utxo := range sortedUtxos {
		if utxo.Id.Id() == txInput.Id() &&
			utxo.Id.Index() == txInput.Index() {
			return idx
		}
	}
	return -1
}

func selectCollateralUtxo(utxos []common.Utxo) (common.Utxo, error) {
	for _, utxo := range utxos {
		if utxo.Output == nil || utxo.Output.Assets() != nil {
			continue
		}
		addr := utxo.Output.Address()
		if addr.Type() != common.AddressTypeKeyKey &&
			addr.Type() != common.AddressTypeKeyNone {
			continue
		}
		amount := utxo.Output.Amount()
		if amount == nil || amount.Sign() <= 0 {
			continue
		}
		return utxo, nil
	}
	return common.Utxo{}, errors.New(
		"script transaction requires an ADA-only wallet UTxO for collateral",
	)
}

func validateSwapTxInputs(
	inputs []common.TransactionInput,
	poolInput common.TransactionInput,
	swapInput common.TransactionInput,
	datumPoolInputIdx int,
	datumSwapInputIdx int,
) error {
	if len(inputs) != 2 {
		return fmt.Errorf(
			"unexpected swap transaction input count: got %d, want 2",
			len(inputs),
		)
	}
	poolInputIdx := transactionInputIndex(inputs, poolInput)
	if poolInputIdx < 0 {
		return errors.New("built transaction is missing pool input")
	}
	swapInputIdx := transactionInputIndex(inputs, swapInput)
	if swapInputIdx < 0 {
		return errors.New("built transaction is missing swap input")
	}
	if poolInputIdx != datumPoolInputIdx ||
		swapInputIdx != datumSwapInputIdx {
		return fmt.Errorf(
			"built transaction input order changed: pool input index got %d want %d, swap input index got %d want %d",
			poolInputIdx,
			datumPoolInputIdx,
			swapInputIdx,
			datumSwapInputIdx,
		)
	}
	return nil
}

func transactionInputIndex(
	inputs []common.TransactionInput,
	txInput common.TransactionInput,
) int {
	for idx, input := range inputs {
		if input.Id() == txInput.Id() && input.Index() == txInput.Index() {
			return idx
		}
	}
	return -1
}

// decodeUtxo reconstructs a gouroboros common.Utxo from the stored CBOR
// representation (a 2-element array of [input, output]). common.Utxo has no
// custom CBOR unmarshaler and its Output field is an interface, so the input
// and output are decoded individually, mirroring internal/storage's handling.
func decodeUtxo(utxoBytes []byte) (common.Utxo, error) {
	tmpUnwrap := []cbor.RawMessage{}
	if _, err := cbor.Decode(utxoBytes, &tmpUnwrap); err != nil {
		return common.Utxo{}, err
	}
	if len(tmpUnwrap) != 2 {
		return common.Utxo{}, fmt.Errorf(
			"unexpected UTxO CBOR structure: got %d elements, want 2",
			len(tmpUnwrap),
		)
	}
	var input ledger.ShelleyTransactionInput
	if _, err := cbor.Decode(tmpUnwrap[0], &input); err != nil {
		return common.Utxo{}, err
	}
	output, err := ledger.NewTransactionOutputFromCbor(tmpUnwrap[1])
	if err != nil {
		return common.Utxo{}, err
	}
	return common.Utxo{Id: input, Output: output}, nil
}

// plutusDatum encodes a value to CBOR and decodes it back into a common.Datum.
// This builds redeemer data from gouroboros Plutus constructor encoders while
// producing the exact on-chain CBOR the validator expects.
func plutusDatum(value any) (common.Datum, error) {
	cborBytes, err := cbor.Encode(value)
	if err != nil {
		return common.Datum{}, err
	}
	var datum common.Datum
	if _, err := cbor.Decode(cborBytes, &datum); err != nil {
		return common.Datum{}, err
	}
	return datum, nil
}
