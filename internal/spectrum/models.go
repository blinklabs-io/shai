package spectrum

import (
	"encoding/hex"
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
)

type SwapConfig struct {
	cbor.StructAsArray
	Base           AssetClass
	Quote          AssetClass
	PoolId         AssetClass
	FeeNum         uint64
	FeePerTokenNum uint64
	FeePerTokenDen uint64
	RewardPkh      []byte
	StakePkhPd     any
	BaseAmount     uint64
	MinQuoteAmount uint64
}

func (s *SwapConfig) UnmarshalCBOR(cborData []byte) error {
	var tmpConstr cbor.Constructor
	if _, err := cbor.Decode(cborData, &tmpConstr); err != nil {
		return err
	}
	return cbor.DecodeGeneric(
		tmpConstr.FieldsCbor(),
		s,
	)
}

func (s SwapConfig) String() string {
	return fmt.Sprintf(
		"SwapConfig< base = %s, quote = %s, pool_id = %s, fee_num = %d, fee_per_token_num = %d, fee_per_token_den = %d, reward_pkh = %s, stake_pkh_pd = %#v, base_amount = %d, min_quote_amount = %d >",
		s.Base.String(),
		s.Quote.String(),
		s.PoolId.String(),
		s.FeeNum,
		s.FeePerTokenNum,
		s.FeePerTokenDen,
		hex.EncodeToString(s.RewardPkh),
		s.StakePkhPd,
		s.BaseAmount,
		s.MinQuoteAmount,
	)
}

type AssetClass struct {
	cbor.StructAsArray
	PolicyId []byte
	Name     []byte
}

func (a *AssetClass) UnmarshalCBOR(cborData []byte) error {
	var tmpConstr cbor.Constructor
	if _, err := cbor.Decode(cborData, &tmpConstr); err != nil {
		return err
	}
	return cbor.DecodeGeneric(
		tmpConstr.FieldsCbor(),
		a,
	)
}

func (a AssetClass) String() string {
	return fmt.Sprintf(
		"AssetClass< name = %s, policy_id = %s >",
		a.Name,
		hex.EncodeToString(a.PolicyId),
	)
}

type DepositConfig struct {
	cbor.StructAsArray
	PoolId        AssetClass
	X             AssetClass
	Y             AssetClass
	Lq            AssetClass
	ExFee         uint64
	RewardPkh     []byte
	StakePkhPd    any
	CollateralAda uint64
}

func (d *DepositConfig) UnmarshalCBOR(cborData []byte) error {
	var tmpConstr cbor.Constructor
	if _, err := cbor.Decode(cborData, &tmpConstr); err != nil {
		return err
	}
	return cbor.DecodeGeneric(
		tmpConstr.FieldsCbor(),
		d,
	)
}

func (d DepositConfig) String() string {
	return fmt.Sprintf(
		"DepositConfig< pool_id = %s, x = %s, y = %s, lq = %s, ex_fee = %d, reward_pkh = %s, stake_pkh_pd = %#v, collateral_ada = %d >",
		d.PoolId.String(),
		d.X.String(),
		d.Y.String(),
		d.Lq.String(),
		d.ExFee,
		hex.EncodeToString(d.RewardPkh),
		d.StakePkhPd,
		d.CollateralAda,
	)
}

type RedeemConfig struct {
	cbor.StructAsArray
	PoolId     AssetClass
	X          AssetClass
	Y          AssetClass
	Lq         AssetClass
	ExFee      uint64
	RewardPkh  []byte
	StakePkhPd any
}

func (r *RedeemConfig) UnmarshalCBOR(cborData []byte) error {
	var tmpConstr cbor.Constructor
	if _, err := cbor.Decode(cborData, &tmpConstr); err != nil {
		return err
	}
	return cbor.DecodeGeneric(
		tmpConstr.FieldsCbor(),
		r,
	)
}

func (r RedeemConfig) String() string {
	return fmt.Sprintf(
		"RedeemConfig< pool_id = %s, x = %s, y = %s, lq = %s, ex_fee = %d, reward_pkh = %s, stake_pkh_pd = %#v >",
		r.PoolId.String(),
		r.X.String(),
		r.Y.String(),
		r.Lq.String(),
		r.ExFee,
		hex.EncodeToString(r.RewardPkh),
		r.StakePkhPd,
	)
}

type PoolConfig struct {
	cbor.StructAsArray
	Nft         AssetClass
	X           AssetClass
	Y           AssetClass
	Lq          AssetClass
	FeeNum      uint64
	AdminPolicy []any
	LqBound     uint64
}

func (p *PoolConfig) UnmarshalCBOR(cborData []byte) error {
	var tmpConstr cbor.Constructor
	if _, err := cbor.Decode(cborData, &tmpConstr); err != nil {
		return err
	}
	return cbor.DecodeGeneric(
		tmpConstr.FieldsCbor(),
		p,
	)
}

func (p PoolConfig) String() string {
	return fmt.Sprintf(
		"PoolConfig< nft = %s, x = %s, y = %s, lq = %s, fee_num = %d, admin_policy = %v, lq_bound = %d >",
		p.Nft.String(),
		p.X.String(),
		p.Y.String(),
		p.Lq.String(),
		p.FeeNum,
		p.AdminPolicy,
		p.LqBound,
	)
}
