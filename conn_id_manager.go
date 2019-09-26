package quic

import (
	"fmt"

	"github.com/lucas-clemente/quic-go/internal/protocol"
	"github.com/lucas-clemente/quic-go/internal/utils"
	"github.com/lucas-clemente/quic-go/internal/wire"
)

type connIDManager struct {
	queue utils.NewConnectionIDList

	changedAtLeastOnce     bool
	packetsSinceLastChange uint64

	queueControlFrame func(wire.Frame)
}

func newConnIDManager(queueControlFrame func(wire.Frame)) *connIDManager {
	return &connIDManager{queueControlFrame: queueControlFrame}
}

func (h *connIDManager) Add(f *wire.NewConnectionIDFrame) error {
	if err := h.add(f); err != nil {
		return err
	}
	if h.queue.Len() > protocol.MaxActiveConnectionIDs {
		// delete the first connection ID in the queue
		val := h.queue.Remove(h.queue.Front())
		h.queueControlFrame(&wire.RetireConnectionIDFrame{
			SequenceNumber: val.SequenceNumber,
		})
	}
	return nil
}

func (h *connIDManager) add(f *wire.NewConnectionIDFrame) error {
	for el := h.queue.Front(); el != nil; el = el.Next() {
		if el.Value.SequenceNumber < f.RetirePriorTo {
			h.queueControlFrame(&wire.RetireConnectionIDFrame{
				SequenceNumber: el.Value.SequenceNumber,
			})
			h.queue.Remove(el)
		} else {
			break
		}
	}

	// insert a new element at the end
	if h.queue.Len() == 0 || h.queue.Back().Value.SequenceNumber < f.SequenceNumber {
		h.queue.PushBack(utils.NewConnectionID{
			SequenceNumber:      f.SequenceNumber,
			ConnectionID:        f.ConnectionID,
			StatelessResetToken: f.StatelessResetToken,
		})
		return nil
	}
	// insert a new element somewhere in the middle
	for el := h.queue.Front(); el != nil; el = el.Next() {
		if el.Value.SequenceNumber == f.SequenceNumber {
			if !el.Value.ConnectionID.Equal(f.ConnectionID) {
				return fmt.Errorf("received conflicting connection IDs for sequence number %d", f.SequenceNumber)
			}
			if el.Value.StatelessResetToken != f.StatelessResetToken {
				return fmt.Errorf("received conflicting stateless reset tokens for sequence number %d", f.SequenceNumber)
			}
			return nil
		}
		if el.Value.SequenceNumber > f.SequenceNumber {
			h.queue.InsertBefore(utils.NewConnectionID{
				SequenceNumber:      f.SequenceNumber,
				ConnectionID:        f.ConnectionID,
				StatelessResetToken: f.StatelessResetToken,
			}, el)
			return nil
		}
	}
	panic("should have processed NEW_CONNECTION_ID frame")
}

func (h *connIDManager) SentPacket() {
	h.packetsSinceLastChange++
}

func (h *connIDManager) shouldChangeConnID() bool {
	// iniate the first change as early as possible
	if !h.changedAtLeastOnce {
		return true
	}
	// For later changes, only change if
	// 1. The queue of connection IDs is filled more than 50%.
	// 2. We sent at least PacketsPerConnectionID packets
	return 2*h.queue.Len() > protocol.MaxActiveConnectionIDs &&
		h.packetsSinceLastChange > protocol.PacketsPerConnectionID
}

func (h *connIDManager) MaybeGetNewConnID() (protocol.ConnectionID, *[16]byte) {
	if !h.shouldChangeConnID() || h.queue.Len() == 0 {
		return nil, nil
	}

	h.changedAtLeastOnce = true
	h.packetsSinceLastChange = 0
	val := h.queue.Remove(h.queue.Front())
	return val.ConnectionID, &val.StatelessResetToken
}
