package node

import (
	"github.com/blinklabs-io/gouroboros/protocol/peersharing"
)

func (n *Node) peerSharingShareRequest(ctx peersharing.CallbackContext, amount int) ([]peersharing.PeerAddress, error) {
	// This purposely returns an empty set of peers. We only care about getting
	// our own address shared for now, but it's required that we respond properly
	// to this
	return []peersharing.PeerAddress{}, nil
}
