package node

import (
	"testing"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/blinklabs-io/gouroboros/protocol/localtxsubmission"
)

func TestInboundLocalTxSubmissionIsRejected(t *testing.T) {
	mempool := &txsubmissionMempool{
		Transactions: make(map[string]*TxsubmissionMempoolTransaction),
	}
	n := &Node{txsubmissionMempool: mempool}
	callbacks := 0
	mempool.AddNewTransactionFunc(func(TxsubmissionMempoolTransaction) error {
		callbacks++
		return nil
	})
	tests := []struct {
		name    string
		content any
	}{
		{name: "wrong payload type", content: "not bytes"},
		{name: "invalid CBOR", content: []byte{0xff}},
		{name: "structurally valid", content: localTxSubmissionBytes(t)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := n.localTxsubmissionServerSubmitTx(
				localtxsubmission.CallbackContext{},
				localtxsubmission.MsgSubmitTxTransaction{
					EraId: babbage.EraIdBabbage,
					Raw: cbor.Tag{
						Number:  24,
						Content: tt.content,
					},
				},
			)
			if err == nil {
				t.Fatal("inbound transaction was accepted")
			}
		})
	}
	if callbacks != 0 {
		t.Fatalf("inbound local-tx submission triggered %d callbacks", callbacks)
	}
	if len(mempool.Transactions) != 0 {
		t.Fatalf("inbound transaction was retained: %d", len(mempool.Transactions))
	}
}

func TestRejectedOwnedTransactionReleasesMempoolState(t *testing.T) {
	mempool := &txsubmissionMempool{
		Transactions: make(map[string]*TxsubmissionMempoolTransaction),
	}
	n := &Node{txsubmissionMempool: mempool}
	added := 0
	removed := make(chan MempoolRemovalReason, 1)
	mempool.AddNewTransactionFunc(func(TxsubmissionMempoolTransaction) error {
		added++
		return nil
	})
	mempool.AddRemovedTransactionFunc(func(
		_ TxsubmissionMempoolTransaction,
		reason MempoolRemovalReason,
	) {
		removed <- reason
	})
	txBytes := localTxSubmissionBytes(t)
	handle, _, err := n.AddOwnedTransaction(txBytes)
	if err != nil {
		t.Fatalf("add owned transaction: %v", err)
	}
	if added != 1 {
		t.Fatalf("owned transaction callbacks = %d, want 1", added)
	}
	n.RejectOwnedTransaction(handle)
	if reason := waitMempoolCallback(t, removed, "rejected removal"); reason != MempoolRemovalReasonRejected {
		t.Fatalf("removal reason = %q, want rejected", reason)
	}
	if len(mempool.Transactions) != 0 {
		t.Fatal("rejected transaction remained in mempool")
	}
}

func TestAddOutboundTransactionRequiresPeer(t *testing.T) {
	n := &Node{
		txsubmissionMempool: &txsubmissionMempool{
			Transactions: make(map[string]*TxsubmissionMempoolTransaction),
		},
		connTransactionChans: make(
			map[ouroboros.ConnectionId]chan ntnTransaction,
		),
	}
	err := n.AddOutboundTransaction(localTxSubmissionBytes(t))
	if err == nil {
		t.Fatal("outbound transaction succeeded without a peer")
	}
	if len(n.txsubmissionMempool.Transactions) != 0 {
		t.Fatal("unsent outbound transaction remained in mempool")
	}
}

func TestAddOutboundTransactionDoesNotBlockOnFullPeer(t *testing.T) {
	queue := make(chan ntnTransaction, 1)
	queue <- ntnTransaction{Hash: "occupied"}
	n := &Node{
		txsubmissionMempool: &txsubmissionMempool{
			Transactions: make(map[string]*TxsubmissionMempoolTransaction),
		},
		connTransactionChans: map[ouroboros.ConnectionId]chan ntnTransaction{
			{}: queue,
		},
	}
	if err := n.AddOutboundTransaction(localTxSubmissionBytes(t)); err == nil {
		t.Fatal("outbound transaction succeeded with only a full peer queue")
	}
}

func TestStaleOwnedGenerationCannotBeginSubmission(t *testing.T) {
	n := &Node{txsubmissionMempool: &txsubmissionMempool{
		Transactions: make(map[string]*TxsubmissionMempoolTransaction),
	}}
	txBytes := localTxSubmissionBytes(t)
	stale, _, err := n.AddOwnedTransaction(txBytes)
	if err != nil {
		t.Fatalf("add first generation: %v", err)
	}
	n.RejectOwnedTransaction(stale)
	current, _, err := n.AddOwnedTransaction(txBytes)
	if err != nil {
		t.Fatalf("add current generation: %v", err)
	}
	if n.BeginOwnedTransactionSubmission(stale) {
		t.Fatal("stale generation began submission")
	}
	if !n.BeginOwnedTransactionSubmission(current) {
		t.Fatal("current generation could not begin submission")
	}
	n.EndOwnedTransactionSubmission(current)
}

func localTxSubmissionBytes(t *testing.T) []byte {
	t.Helper()
	txBytes, err := (&babbage.BabbageTransaction{
		Body: babbage.BabbageTransactionBody{
			TxInputs: shelley.NewShelleyTransactionInputSet(
				[]shelley.ShelleyTransactionInput{{}},
			),
		},
		TxIsValid: true,
	}).MarshalCBOR()
	if err != nil {
		t.Fatalf("encode transaction: %v", err)
	}
	return txBytes
}
