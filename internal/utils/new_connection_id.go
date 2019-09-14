package utils

import (
	"github.com/lucas-clemente/quic-go/internal/protocol"
)

type NewConnectionID struct {
	SequenceNumber      uint64
	ConnectionID        protocol.ConnectionID
	StatelessResetToken [16]byte
}
