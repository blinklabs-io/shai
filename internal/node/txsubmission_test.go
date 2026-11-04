package node

import (
	"testing"
	"time"
)

func TestMempoolCallbacksRunOutsideLock(t *testing.T) {
	mempool := &txsubmissionMempool{
		Transactions: make(map[string]*TxsubmissionMempoolTransaction),
	}
	addResult := make(chan struct {
		hash     string
		lockFree bool
	}, 1)
	mempool.AddNewTransactionFunc(func(tx TxsubmissionMempoolTransaction) error {
		lockFree := mempool.TryLock()
		if lockFree {
			mempool.Unlock()
		}
		addResult <- struct {
			hash     string
			lockFree bool
		}{tx.Hash, lockFree}
		return nil
	})
	removeResult := make(chan struct {
		hash     string
		reason   MempoolRemovalReason
		lockFree bool
	}, 1)
	mempool.AddRemovedTransactionFunc(func(
		tx TxsubmissionMempoolTransaction,
		reason MempoolRemovalReason,
	) {
		lockFree := mempool.TryLock()
		if lockFree {
			mempool.Unlock()
		}
		removeResult <- struct {
			hash     string
			reason   MempoolRemovalReason
			lockFree bool
		}{tx.Hash, reason, lockFree}
	})

	tx := &TxsubmissionMempoolTransaction{
		Hash:     "tx-add",
		LastSeen: time.Now(),
		Source:   MempoolTransactionSourceOwned,
	}
	if err := mempool.addTransaction(tx); err != nil {
		t.Fatalf("add transaction: %v", err)
	}
	added := waitMempoolCallback(t, addResult, "add")
	mempool.removeTransaction(tx.Hash, MempoolRemovalReasonConfirmed)
	removed := waitMempoolCallback(t, removeResult, "remove")

	if added.hash != tx.Hash || !added.lockFree {
		t.Fatalf("unexpected add callback: %+v", added)
	}
	if removed.hash != tx.Hash ||
		removed.reason != MempoolRemovalReasonConfirmed ||
		!removed.lockFree {
		t.Fatalf("unexpected remove callback: %+v", removed)
	}
}

func TestMempoolExpirationEmitsRemoval(t *testing.T) {
	mempool := &txsubmissionMempool{
		Transactions: make(map[string]*TxsubmissionMempoolTransaction),
	}
	tx := &TxsubmissionMempoolTransaction{
		Hash:     "tx-expired",
		LastSeen: time.Now().Add(-2 * txsubmissionMempoolExpiration),
		Source:   MempoolTransactionSourceOwned,
	}
	mempool.Transactions[tx.Hash] = tx
	removed := make(chan struct {
		hash   string
		reason MempoolRemovalReason
	}, 1)
	mempool.AddRemovedTransactionFunc(func(
		tx TxsubmissionMempoolTransaction,
		reason MempoolRemovalReason,
	) {
		removed <- struct {
			hash   string
			reason MempoolRemovalReason
		}{tx.Hash, reason}
	})

	mempool.removeExpiredAt(time.Now())

	if _, ok := mempool.Transactions[tx.Hash]; ok {
		t.Fatal("expired transaction remained in mempool")
	}
	got := waitMempoolCallback(t, removed, "expiration removal")
	if got.hash != tx.Hash || got.reason != MempoolRemovalReasonExpired {
		t.Fatalf("unexpected removal event: %+v", got)
	}
}

func TestMempoolExpirationKeepsSubmittingGeneration(t *testing.T) {
	mempool := &txsubmissionMempool{
		Transactions: make(map[string]*TxsubmissionMempoolTransaction),
	}
	tx := &TxsubmissionMempoolTransaction{
		Hash:     "tx-submitting",
		LastSeen: time.Now().Add(-2 * txsubmissionMempoolExpiration),
		Source:   MempoolTransactionSourceOwned,
	}
	_, generation, err := mempool.addTransactionWithWait(tx, true)
	if err != nil {
		t.Fatalf("add transaction: %v", err)
	}
	if generation == nil {
		t.Fatal("owned transaction has no generation")
	}
	generation.submitting = true
	mempool.removeExpiredAt(time.Now())
	if mempool.Transactions[tx.Hash] == nil || mempool.addResults[tx.Hash] == nil {
		t.Fatal("submitting transaction expired")
	}
	generation.submitting = false
	mempool.removeExpiredAt(time.Now())
	if mempool.Transactions[tx.Hash] != nil || mempool.addResults[tx.Hash] != nil {
		t.Fatal("expired transaction generation was retained")
	}
}

func TestPeerTransactionsCannotTriggerMempoolActions(t *testing.T) {
	mempool := &txsubmissionMempool{
		Transactions: make(map[string]*TxsubmissionMempoolTransaction),
	}
	added := make(chan struct{}, 1)
	removed := make(chan struct{}, 1)
	mempool.AddNewTransactionFunc(func(TxsubmissionMempoolTransaction) error {
		added <- struct{}{}
		return nil
	})
	mempool.AddRemovedTransactionFunc(func(
		TxsubmissionMempoolTransaction,
		MempoolRemovalReason,
	) {
		removed <- struct{}{}
	})
	tx := &TxsubmissionMempoolTransaction{
		Hash:     "peer-claimed-order",
		Cbor:     []byte("unvalidated peer body"),
		LastSeen: time.Now(),
		Source:   MempoolTransactionSourceObserved,
	}
	if err := mempool.addTransaction(tx); err != nil {
		t.Fatalf("add peer transaction: %v", err)
	}
	if err := mempool.addTransaction(tx); err != nil {
		t.Fatalf("repeat peer transaction: %v", err)
	}
	if len(mempool.Transactions) != 0 {
		t.Fatalf("observed transactions were retained: %d", len(mempool.Transactions))
	}
	mempool.removeTransaction(tx.Hash, MempoolRemovalReasonConfirmed)

	select {
	case <-added:
		t.Fatal("peer transaction triggered add callback")
	case <-removed:
		t.Fatal("peer transaction triggered remove callback")
	default:
	}
}

func TestOwnedTransactionAfterObservation(t *testing.T) {
	mempool := &txsubmissionMempool{
		Transactions: make(map[string]*TxsubmissionMempoolTransaction),
	}
	added := make(chan MempoolTransactionSource, 1)
	removed := make(chan MempoolTransactionSource, 1)
	mempool.AddNewTransactionFunc(func(tx TxsubmissionMempoolTransaction) error {
		added <- tx.Source
		return nil
	})
	mempool.AddRemovedTransactionFunc(func(
		tx TxsubmissionMempoolTransaction,
		_ MempoolRemovalReason,
	) {
		removed <- tx.Source
	})
	peer := &TxsubmissionMempoolTransaction{
		Hash:     "promoted",
		Cbor:     []byte("peer"),
		LastSeen: time.Now(),
		Source:   MempoolTransactionSourceObserved,
	}
	if err := mempool.addTransaction(peer); err != nil {
		t.Fatalf("add peer transaction: %v", err)
	}
	localSeen := time.Now().Add(-time.Minute)
	local := &TxsubmissionMempoolTransaction{
		Hash:     peer.Hash,
		Cbor:     []byte("local"),
		LastSeen: localSeen,
		Source:   MempoolTransactionSourceOwned,
	}
	if err := mempool.addTransaction(local); err != nil {
		t.Fatalf("add owned transaction: %v", err)
	}
	if source := waitMempoolCallback(t, added, "owned add"); source != MempoolTransactionSourceOwned {
		t.Fatalf("add callback source = %d, want owned", source)
	}
	if err := mempool.addTransaction(peer); err != nil {
		t.Fatalf("repeat peer transaction: %v", err)
	}
	stored, ok := mempool.Transactions[peer.Hash]
	if !ok || stored == nil {
		t.Fatal("promoted transaction was not retained")
	}
	if stored.Source != MempoolTransactionSourceOwned ||
		string(stored.Cbor) != "local" ||
		!stored.LastSeen.Equal(localSeen) {
		t.Fatalf("observed replay changed owned transaction: %+v", stored)
	}
	mempool.removeTransaction(peer.Hash, MempoolRemovalReasonExpired)
	if source := waitMempoolCallback(t, removed, "owned removal"); source != MempoolTransactionSourceOwned {
		t.Fatalf("remove callback source = %d, want owned", source)
	}
}

func TestObservedReplayDoesNotRefreshOwnedTransaction(t *testing.T) {
	mempool := &txsubmissionMempool{
		Transactions: make(map[string]*TxsubmissionMempoolTransaction),
	}
	ownedSeen := time.Now().Add(-time.Minute)
	owned := &TxsubmissionMempoolTransaction{
		Hash:     "owned-replay",
		Cbor:     []byte("owned"),
		LastSeen: ownedSeen,
		Source:   MempoolTransactionSourceOwned,
	}
	if err := mempool.addTransaction(owned); err != nil {
		t.Fatalf("add owned transaction: %v", err)
	}
	if err := mempool.addTransaction(&TxsubmissionMempoolTransaction{
		Hash:     owned.Hash,
		Cbor:     []byte("observed"),
		LastSeen: time.Now(),
		Source:   MempoolTransactionSourceObserved,
	}); err != nil {
		t.Fatalf("replay observed transaction: %v", err)
	}
	stored := mempool.Transactions[owned.Hash]
	if stored == nil || !stored.LastSeen.Equal(ownedSeen) ||
		string(stored.Cbor) != "owned" ||
		stored.Source != MempoolTransactionSourceOwned {
		t.Fatalf("observed replay changed owned transaction: %+v", stored)
	}
}

func TestMempoolLifecycleCallbacksPreserveCausalOrder(t *testing.T) {
	mempool := &txsubmissionMempool{
		Transactions: make(map[string]*TxsubmissionMempoolTransaction),
	}
	addEntered := make(chan struct{})
	releaseAdd := make(chan struct{})
	removed := make(chan struct{})
	mempool.AddNewTransactionFunc(func(TxsubmissionMempoolTransaction) error {
		close(addEntered)
		<-releaseAdd
		return nil
	})
	mempool.AddRemovedTransactionFunc(func(
		TxsubmissionMempoolTransaction,
		MempoolRemovalReason,
	) {
		close(removed)
	})
	if err := mempool.addTransaction(&TxsubmissionMempoolTransaction{
		Hash:     "causal-order",
		LastSeen: time.Now(),
		Source:   MempoolTransactionSourceOwned,
	}); err != nil {
		t.Fatalf("add transaction: %v", err)
	}
	<-addEntered
	removeDone := make(chan struct{})
	go func() {
		mempool.removeTransaction(
			"causal-order",
			MempoolRemovalReasonConfirmed,
		)
		close(removeDone)
	}()
	select {
	case <-removed:
		t.Fatal("removal callback overtook the blocked add callback")
	case <-removeDone:
		t.Fatal("removal returned before ordered callbacks completed")
	case <-time.After(100 * time.Millisecond):
	}
	close(releaseAdd)
	waitMempoolCallback(t, removed, "ordered removal")
	waitMempoolCallback(t, removeDone, "removal completion")
}

func TestExpiredGenerationFinishesBeforeSameHashReadd(t *testing.T) {
	mempool := &txsubmissionMempool{
		Transactions: map[string]*TxsubmissionMempoolTransaction{
			"same-hash": {
				Hash:     "same-hash",
				LastSeen: time.Now().Add(-2 * txsubmissionMempoolExpiration),
				Source:   MempoolTransactionSourceOwned,
			},
		},
	}
	removalEntered := make(chan struct{})
	releaseRemoval := make(chan struct{})
	added := make(chan struct{})
	mempool.AddRemovedTransactionFunc(func(
		TxsubmissionMempoolTransaction,
		MempoolRemovalReason,
	) {
		close(removalEntered)
		<-releaseRemoval
	})
	mempool.AddNewTransactionFunc(func(TxsubmissionMempoolTransaction) error {
		close(added)
		return nil
	})
	mempool.removeExpiredAt(time.Now())
	<-removalEntered
	if err := mempool.addTransaction(&TxsubmissionMempoolTransaction{
		Hash:     "same-hash",
		LastSeen: time.Now(),
		Source:   MempoolTransactionSourceOwned,
	}); err != nil {
		t.Fatalf("re-add transaction: %v", err)
	}
	select {
	case <-added:
		t.Fatal("new generation callback overtook old generation removal")
	case <-time.After(100 * time.Millisecond):
	}
	close(releaseRemoval)
	waitMempoolCallback(t, added, "new generation add")
}

func TestMempoolCallbackCanEnqueueTransaction(t *testing.T) {
	mempool := &txsubmissionMempool{
		Transactions: make(map[string]*TxsubmissionMempoolTransaction),
	}
	callbacks := make(chan string, 2)
	mempool.AddNewTransactionFunc(func(tx TxsubmissionMempoolTransaction) error {
		if tx.Hash == "first" {
			if err := mempool.addTransaction(&TxsubmissionMempoolTransaction{
				Hash:     "second",
				LastSeen: time.Now(),
				Source:   MempoolTransactionSourceOwned,
			}); err != nil {
				return err
			}
		}
		callbacks <- tx.Hash
		return nil
	})
	if err := mempool.addTransaction(&TxsubmissionMempoolTransaction{
		Hash:     "first",
		LastSeen: time.Now(),
		Source:   MempoolTransactionSourceOwned,
	}); err != nil {
		t.Fatalf("add first transaction: %v", err)
	}
	if got := waitMempoolCallback(t, callbacks, "first add"); got != "first" {
		t.Fatalf("first callback = %q", got)
	}
	if got := waitMempoolCallback(t, callbacks, "nested add"); got != "second" {
		t.Fatalf("second callback = %q", got)
	}
}

func TestSynchronousOwnedAddWaitsForReservationCallback(t *testing.T) {
	mempool := &txsubmissionMempool{
		Transactions: make(map[string]*TxsubmissionMempoolTransaction),
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	mempool.AddNewTransactionFunc(func(TxsubmissionMempoolTransaction) error {
		close(entered)
		<-release
		return nil
	})
	done := make(chan error, 1)
	go func() {
		_, _, err := mempool.addTransactionWithWait(
			&TxsubmissionMempoolTransaction{
				Hash:     "synchronous-owned",
				LastSeen: time.Now(),
				Source:   MempoolTransactionSourceOwned,
			},
			true,
		)
		done <- err
	}()
	<-entered
	select {
	case err := <-done:
		t.Fatalf("owned add returned before reservation callback: %v", err)
	default:
	}
	close(release)
	if err := waitMempoolCallback(t, done, "synchronous owned add"); err != nil {
		t.Fatalf("synchronous owned add: %v", err)
	}
}

func TestDuplicateOwnedAddWaitsAndDoesNotReadmit(t *testing.T) {
	mempool := &txsubmissionMempool{
		Transactions: make(map[string]*TxsubmissionMempoolTransaction),
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	mempool.AddNewTransactionFunc(func(TxsubmissionMempoolTransaction) error {
		close(entered)
		<-release
		return nil
	})
	type result struct {
		added bool
		err   error
	}
	first := make(chan result, 1)
	second := make(chan result, 1)
	tx := &TxsubmissionMempoolTransaction{
		Hash:     "duplicate-owned",
		LastSeen: time.Now(),
		Source:   MempoolTransactionSourceOwned,
	}
	go func() {
		added, _, err := mempool.addTransactionWithWait(tx, true)
		first <- result{added: added, err: err}
	}()
	<-entered
	go func() {
		added, _, err := mempool.addTransactionWithWait(tx, true)
		second <- result{added: added, err: err}
	}()
	select {
	case got := <-second:
		t.Fatalf("duplicate add returned before first callback: %+v", got)
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	firstResult := waitMempoolCallback(t, first, "first owned add")
	secondResult := waitMempoolCallback(t, second, "duplicate owned add")
	if firstResult.err != nil || !firstResult.added {
		t.Fatalf("first add result: %+v", firstResult)
	}
	if secondResult.err != nil || secondResult.added {
		t.Fatalf("duplicate add result: %+v", secondResult)
	}
}

func TestBlockedCallbackDoesNotDelayUnrelatedConfirmation(t *testing.T) {
	mempool := &txsubmissionMempool{
		Transactions: make(map[string]*TxsubmissionMempoolTransaction),
	}
	blockedEntered := make(chan struct{})
	releaseBlocked := make(chan struct{})
	removed := make(chan string, 1)
	mempool.AddNewTransactionFunc(func(tx TxsubmissionMempoolTransaction) error {
		if tx.Hash == "blocked" {
			close(blockedEntered)
			<-releaseBlocked
		}
		return nil
	})
	mempool.AddRemovedTransactionFunc(func(
		tx TxsubmissionMempoolTransaction,
		_ MempoolRemovalReason,
	) {
		removed <- tx.Hash
	})
	for _, hash := range []string{"blocked", "independent"} {
		if err := mempool.addTransaction(&TxsubmissionMempoolTransaction{
			Hash:     hash,
			LastSeen: time.Now(),
			Source:   MempoolTransactionSourceOwned,
		}); err != nil {
			t.Fatalf("add %s transaction: %v", hash, err)
		}
	}
	<-blockedEntered
	removeDone := make(chan struct{})
	go func() {
		mempool.removeTransaction(
			"independent",
			MempoolRemovalReasonConfirmed,
		)
		close(removeDone)
	}()
	if got := waitMempoolCallback(t, removed, "independent removal"); got != "independent" {
		t.Fatalf("removed transaction = %q, want independent", got)
	}
	waitMempoolCallback(t, removeDone, "independent removal completion")
	close(releaseBlocked)
}

func waitMempoolCallback[T any](
	t *testing.T,
	callback <-chan T,
	name string,
) T {
	t.Helper()
	select {
	case value := <-callback:
		return value
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s callback", name)
		var zero T
		return zero
	}
}
