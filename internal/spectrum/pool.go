// Copyright 2025 Blink Labs Software
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package spectrum

import (
	"errors"
	"math/big"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/shai/internal/storage"
)

const (
	feeDenom = 1000
)

type Pool struct {
	Id     AssetClass
	X      AssetAmount
	Y      AssetAmount
	Lq     AssetAmount
	FeeNum uint64
	Datum  PoolConfig
}

type AssetAmount struct {
	Class  AssetClass
	Amount uint64
}

func (a AssetAmount) IsAsset(asset AssetClass) bool {
	if string(asset.PolicyId) == string(a.Class.PolicyId) &&
		string(asset.Name) == string(a.Class.Name) {
		return true
	}
	return false
}

func (a AssetAmount) IsLovelace() bool {
	return a.Class.IsLovelace()
}

func NewPoolFromTransactionOutput(
	txOutput ledger.TransactionOutput,
) (*Pool, error) {
	if txOutput.Datum() == nil {
		return nil, errors.New("no datum found in transaction output")
	}
	var poolConfig PoolConfig
	if _, err := cbor.Decode(txOutput.Datum().Cbor(), &poolConfig); err != nil {
		return nil, err
	}
	p := &Pool{
		Id: poolConfig.Nft,
		X: AssetAmount{
			Class:  poolConfig.X,
			Amount: getAssetAmountFromTransactionOutput(poolConfig.X, txOutput),
		},
		Y: AssetAmount{
			Class:  poolConfig.Y,
			Amount: getAssetAmountFromTransactionOutput(poolConfig.Y, txOutput),
		},
		Lq: AssetAmount{
			Class: poolConfig.Lq,
			Amount: getAssetAmountFromTransactionOutput(
				poolConfig.Lq,
				txOutput,
			),
		},
		FeeNum: poolConfig.FeeNum,
		Datum:  poolConfig,
	}
	return p, nil
}

func getAssetAmountFromTransactionOutput(
	assetClass AssetClass,
	txOutput ledger.TransactionOutput,
) uint64 {
	if len(assetClass.PolicyId) == 0 {
		// ADA
		amount := txOutput.Amount()
		return amount.Uint64()
	}
	amount := txOutput.Assets().Asset(
		ledger.NewBlake2b224(assetClass.PolicyId),
		assetClass.Name,
	)
	return amount.Uint64()
}

func NewPoolFromUtxoBytes(utxoBytes []byte) (*Pool, error) {
	var utxo storage.Utxo
	if _, err := cbor.Decode(utxoBytes, &utxo); err != nil {
		return nil, err
	}
	return NewPoolFromTransactionOutput(utxo.Output)
}

func (p *Pool) OutputForInput(
	inputAssetClass AssetClass,
	inputAmount uint64,
) AssetAmount {
	var inputAsset, outputAsset AssetAmount
	if p.X.IsAsset(inputAssetClass) {
		inputAsset = p.X
		outputAsset = p.Y
	} else if p.Y.IsAsset(inputAssetClass) {
		inputAsset = p.Y
		outputAsset = p.X
	} else {
		return AssetAmount{}
	}
	// We have to use big.Int because we're dealing with multiplication of large numbers that overflows uint64, which makes this very, very ugly
	bigOutputAssetAmount := new(big.Int).SetUint64(outputAsset.Amount)
	bigInputAmount := new(big.Int).SetUint64(inputAmount)
	bigFeeNum := new(big.Int).SetUint64(p.FeeNum)
	bigInputAssetAmount := new(big.Int).SetUint64(inputAsset.Amount)
	bigFeeDenom := big.NewInt(feeDenom)
	// (outputAsset.Amount * inputAmount * p.FeeNum) / ((inputAsset.Amount+(inputAsset.Amount*0)/10000)*feeDenom + inputAmount*p.FeeNum)
	outputAmount := new(big.Int).Div(
		// (outputAsset.Amount * inputAmount * p.FeeNum)
		new(big.Int).Mul(
			bigFeeNum,
			new(big.Int).Mul(
				bigOutputAssetAmount,
				bigInputAmount,
			),
		),
		// ((inputAsset.Amount+(inputAsset.Amount*0)/10000)*feeDenom + inputAmount*p.FeeNum)
		new(big.Int).Add(
			new(big.Int).Mul(
				new(big.Int).Add(
					bigInputAssetAmount,
					// NOTE: we use a placeholder for the slippage calculation here, since using 0 makes it a no-op anyway
					// (inputAsset.Amount*0)/10000
					big.NewInt(0),
				),
				bigFeeDenom,
			),
			new(big.Int).Mul(
				bigInputAmount,
				bigFeeNum,
			),
		),
	).Uint64()
	return AssetAmount{
		Class:  outputAsset.Class,
		Amount: outputAmount,
	}
}

func (p *Pool) Asset(asset AssetClass) AssetAmount {
	if p.X.IsAsset(asset) {
		return p.X
	} else if p.Y.IsAsset(asset) {
		return p.Y
	}
	return AssetAmount{}
}

func (p *Pool) CalculateReturnToPool(
	inputAsset AssetAmount,
	rewardAsset AssetAmount,
) (uint64, []AssetAmount) {
	var retAda uint64
	retUnits := []AssetAmount{
		{
			Class:  p.Id,
			Amount: 1,
		},
		p.Lq,
	}
	inAsset := p.Asset(inputAsset.Class)
	inAsset.Amount += inputAsset.Amount
	outAsset := p.Asset(rewardAsset.Class)
	outAsset.Amount -= rewardAsset.Amount
	if inAsset.IsLovelace() {
		retAda = inAsset.Amount
		retUnits = append(retUnits, outAsset)
	} else if outAsset.IsLovelace() {
		retAda = outAsset.Amount
		retUnits = append(retUnits, inAsset)
	}
	return retAda, retUnits
}
