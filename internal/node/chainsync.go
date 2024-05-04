package node

import (
	"encoding/hex"
	"fmt"
	"sync"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/protocol/blockfetch"
	"github.com/blinklabs-io/gouroboros/protocol/chainsync"
	ocommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/blinklabs-io/snek/event"
	input_chainsync "github.com/blinklabs-io/snek/input/chainsync"
)

const (
	maxRecentBlocks = 20
)

type chainsyncClientState struct {
	sync.Mutex
	tip          chainsyncTip
	recentBlocks []chainsyncBlock
	subs         map[ouroboros.ConnectionId]chan chainsyncBlock
}

func (c *chainsyncClientState) Sub(key ouroboros.ConnectionId) chan chainsyncBlock {
	c.Lock()
	defer c.Unlock()
	tmpChan := make(chan chainsyncBlock, maxRecentBlocks)
	if c.subs == nil {
		c.subs = make(map[ouroboros.ConnectionId]chan chainsyncBlock)
	}
	c.subs[key] = tmpChan
	// Send all current blocks
	for _, block := range c.recentBlocks {
		tmpChan <- block
	}
	return tmpChan
}

func (c *chainsyncClientState) Unsub(key ouroboros.ConnectionId) {
	c.Lock()
	defer c.Unlock()
	close(c.subs[key])
	delete(c.subs, key)
}

func (c *chainsyncClientState) AddBlock(block chainsyncBlock) {
	c.Lock()
	defer c.Unlock()
	c.recentBlocks = append(
		c.recentBlocks,
		block,
	)
	// Prune older blocks
	if len(c.recentBlocks) > maxRecentBlocks {
		c.recentBlocks = c.recentBlocks[len(c.recentBlocks)-maxRecentBlocks:]
	}
	// Publish new block to subscribers
	for _, pubChan := range c.subs {
		pubChan <- block
	}
}

func (c *chainsyncClientState) Rollback(slot uint64, hash string) {
	c.Lock()
	defer c.Unlock()
	// Remove recent blocks newer than the rollback block
	for idx, block := range c.recentBlocks {
		if block.Tip.SlotNumber > slot {
			c.recentBlocks = c.recentBlocks[:idx]
			break
		}
	}
	// Publish rollback to subscribers
	for _, pubChan := range c.subs {
		pubChan <- chainsyncBlock{
			Rollback: true,
			Tip: chainsyncTip{
				SlotNumber: slot,
				BlockHash:  hash,
			},
		}
	}
}

type chainsyncTip struct {
	SlotNumber  uint64
	BlockHash   string
	BlockNumber uint64
}

func (c chainsyncTip) String() string {
	return fmt.Sprintf(
		"< slot_number = %d, block_number = %d, block_hash = %s >",
		c.SlotNumber,
		c.BlockNumber,
		c.BlockHash,
	)
}

func (c chainsyncTip) ToTip() chainsync.Tip {
	hashBytes, _ := hex.DecodeString(c.BlockHash)
	return chainsync.Tip{
		BlockNumber: c.BlockNumber,
		Point: ocommon.Point{
			Slot: c.SlotNumber,
			Hash: hashBytes[:],
		},
	}
}

type chainsyncBlock struct {
	Tip      chainsyncTip
	Cbor     []byte
	Type     uint
	Rollback bool
}

func (c chainsyncBlock) String() string {
	return fmt.Sprintf(
		"%s (%d bytes)",
		c.Tip.String(),
		len(c.Cbor),
	)
}

func (n *Node) chainsyncClientHandleEvent(evt event.Event) error {
	switch e := evt.Payload.(type) {
	case input_chainsync.RollbackEvent:
		n.chainsyncClientState.Rollback(e.SlotNumber, e.BlockHash)
	case input_chainsync.BlockEvent:
		blockCtx := evt.Context.(input_chainsync.BlockContext)
		// Update current tip
		n.chainsyncClientState.tip = chainsyncTip{
			SlotNumber:  blockCtx.SlotNumber,
			BlockHash:   e.BlockHash,
			BlockNumber: blockCtx.BlockNumber,
		}
		// Determine block type, since snek doesn't provide this information
		blockType, err := ledger.DetermineBlockType(e.BlockCbor)
		if err != nil {
			return err
		}
		// Add to recent blocks
		n.chainsyncClientState.AddBlock(
			chainsyncBlock{
				Tip:  n.chainsyncClientState.tip,
				Cbor: e.BlockCbor,
				Type: blockType,
			},
		)
	case input_chainsync.TransactionEvent:
		eventCtx := evt.Context.(input_chainsync.TransactionContext)
		n.txsubmissionMempool.removeTransaction(eventCtx.TransactionHash)
	}
	return nil
}

type chainsyncServerState struct {
	cursor               chainsyncTip
	blockChan            chan chainsyncBlock
	needsInitialRollback bool
}

func (n *Node) chainsyncServerFindIntersect(ctx chainsync.CallbackContext, points []ocommon.Point) (ocommon.Point, chainsync.Tip, error) {
	var retPoint ocommon.Point
	var retTip chainsync.Tip
	// Find intersection
	var intersectTip chainsyncTip
	for _, block := range n.chainsyncClientState.recentBlocks {
		// Convert chainsyncTip to chainsync.Tip for easier comparison with ocommon.Point
		blockPoint := block.Tip.ToTip().Point
		for _, point := range points {
			if point.Slot != blockPoint.Slot {
				continue
			}
			// Compare as string since we can't directly compare byte slices
			if string(point.Hash) != string(blockPoint.Hash) {
				continue
			}
			intersectTip = block.Tip
			break
		}
	}

	// Populate return tip
	retTip = n.chainsyncClientState.tip.ToTip()

	if intersectTip.SlotNumber == 0 {
		return retPoint, retTip, chainsync.IntersectNotFoundError
	}

	// Create initial chainsync state for connection
	_ = n.chainsyncServerStateInit(ctx.ConnectionId, intersectTip)

	// Populate return point
	retPoint = intersectTip.ToTip().Point

	return retPoint, retTip, nil
}

func (n *Node) chainsyncServerRequestNext(ctx chainsync.CallbackContext) error {
	// Create initial chainsync state for connection
	serverState := n.chainsyncServerStateInit(ctx.ConnectionId, n.chainsyncClientState.tip)
	if serverState.needsInitialRollback {
		err := ctx.Server.RollBackward(
			serverState.cursor.ToTip().Point,
			n.chainsyncClientState.tip.ToTip(),
		)
		if err != nil {
			return err
		}
		serverState.needsInitialRollback = false
		return nil
	}
	for {
		sentAwaitReply := false
		select {
		case block := <-serverState.blockChan:
			// Ignore blocks older than what we've already sent
			if serverState.cursor.SlotNumber >= block.Tip.SlotNumber {
				continue
			}
			return n.chainsyncServerSendNext(ctx, block)
		default:
			err := ctx.Server.AwaitReply()
			if err != nil {
				return err
			}
			// Wait for next block and send
			go func() {
				block := <-serverState.blockChan
				_ = n.chainsyncServerSendNext(ctx, block)
			}()
			sentAwaitReply = true
		}
		if sentAwaitReply {
			break
		}
	}
	return nil
}

func (n *Node) chainsyncServerStateInit(connId connection.ConnectionId, cursor chainsyncTip) *chainsyncServerState {
	// Create initial chainsync state for connection
	if _, ok := n.chainsyncServerState[connId]; !ok {
		n.chainsyncServerState[connId] = &chainsyncServerState{
			cursor:               cursor,
			blockChan:            n.chainsyncClientState.Sub(connId),
			needsInitialRollback: true,
		}
	}
	return n.chainsyncServerState[connId]
}

func (n *Node) chainsyncServerSendNext(ctx chainsync.CallbackContext, block chainsyncBlock) error {
	var err error
	if block.Rollback {
		err = ctx.Server.RollBackward(
			block.Tip.ToTip().Point,
			n.chainsyncClientState.tip.ToTip(),
		)
	} else {
		blockBytes := block.Cbor[:]
		err = ctx.Server.RollForward(
			block.Type,
			blockBytes,
			n.chainsyncClientState.tip.ToTip(),
		)
	}
	return err
}

func (n *Node) blockfetchServerRequestRange(ctx blockfetch.CallbackContext, start ocommon.Point, end ocommon.Point) error {
	go func() {
		if err := ctx.Server.StartBatch(); err != nil {
			return
		}
		for _, block := range n.chainsyncClientState.recentBlocks {
			if block.Tip.SlotNumber < start.Slot {
				continue
			}
			if block.Tip.SlotNumber > end.Slot {
				break
			}
			blockBytes := block.Cbor[:]
			err := ctx.Server.Block(
				block.Type,
				blockBytes,
			)
			if err != nil {
				return
			}
		}
		if err := ctx.Server.BatchDone(); err != nil {
			return
		}
	}()
	return nil
}
