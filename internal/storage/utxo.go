package storage

import (
	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
)

type Utxo struct {
	cbor.DecodeStoreCbor
	Ref    ledger.ShelleyTransactionInput
	Output ledger.TransactionOutput
}

func (u *Utxo) UnmarshalCBOR(data []byte) error {
	tmpUnwrap := []cbor.RawMessage{}
	if _, err := cbor.Decode(data, &tmpUnwrap); err != nil {
		return err
	}
	if _, err := cbor.Decode(tmpUnwrap[0], &(u.Ref)); err != nil {
		return err
	}
	txOutput, err := ledger.NewTransactionOutputFromCbor(tmpUnwrap[1])
	if err != nil {
		return err
	}
	u.Output = txOutput
	return nil
}
