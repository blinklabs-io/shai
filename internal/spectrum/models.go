package spectrum

import (
	"encoding/hex"
	"fmt"
	"math/big"

	"github.com/blinklabs-io/gouroboros/cbor"
)

type WrappedPkh struct {
	cbor.StructAsArray
	Pkh []byte
}

func (w *WrappedPkh) UnmarshalCBOR(cborData []byte) error {
	var tmpConstr cbor.Constructor
	if _, err := cbor.Decode(cborData, &tmpConstr); err != nil {
		return nil
	}
	if tmpConstr.Constructor() != 0 {
		return nil
	}
	return cbor.DecodeGeneric(
		tmpConstr.FieldsCbor(),
		w,
	)
}

func (w *WrappedPkh) MarshalCBOR() ([]byte, error) {
	var tmpConstr cbor.Constructor
	if w.Pkh != nil {
		tmpConstr = cbor.NewConstructor(0, []any{w.Pkh})
	} else {
		tmpConstr = cbor.NewConstructor(1, []any{})
	}
	return cbor.Encode(&tmpConstr)
}

type SwapConfig struct {
	cbor.StructAsArray
	Base           AssetClass
	Quote          AssetClass
	PoolId         AssetClass
	FeeNum         uint64
	FeePerTokenNum *big.Int
	FeePerTokenDen *big.Int
	RewardPkh      []byte
	StakePkh       WrappedPkh
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
		"SwapConfig< base = %s, quote = %s, pool_id = %s, fee_num = %d, fee_per_token_num = %d, fee_per_token_den = %d, reward_pkh = %x, stake_pkh = %x, base_amount = %d, min_quote_amount = %d >",
		s.Base.String(),
		s.Quote.String(),
		s.PoolId.String(),
		s.FeeNum,
		s.FeePerTokenNum,
		s.FeePerTokenDen,
		s.RewardPkh,
		s.StakePkh.Pkh,
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

func (a AssetClass) IsLovelace() bool {
	return len(a.PolicyId) == 0 && len(a.Name) == 0
}

type DepositConfig struct {
	cbor.StructAsArray
	PoolId        AssetClass
	X             AssetClass
	Y             AssetClass
	Lq            AssetClass
	ExFee         uint64
	RewardPkh     []byte
	StakePkh      WrappedPkh
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
		"DepositConfig< pool_id = %s, x = %s, y = %s, lq = %s, ex_fee = %d, reward_pkh = %x, stake_pkh = %x, collateral_ada = %d >",
		d.PoolId.String(),
		d.X.String(),
		d.Y.String(),
		d.Lq.String(),
		d.ExFee,
		d.RewardPkh,
		d.StakePkh.Pkh,
		d.CollateralAda,
	)
}

type RedeemConfig struct {
	cbor.StructAsArray
	PoolId    AssetClass
	X         AssetClass
	Y         AssetClass
	Lq        AssetClass
	ExFee     uint64
	RewardPkh []byte
	StakePkh  WrappedPkh
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
		"RedeemConfig< pool_id = %s, x = %s, y = %s, lq = %s, ex_fee = %d, reward_pkh = %x, stake_pkh = %x >",
		r.PoolId.String(),
		r.X.String(),
		r.Y.String(),
		r.Lq.String(),
		r.ExFee,
		r.RewardPkh,
		r.StakePkh.Pkh,
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
