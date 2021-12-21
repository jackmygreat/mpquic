package utils

import "github.com/yyleeshine/mpquic/repository/lucas-clemente/quic-go/internal/protocol"

// PacketInterval is an interval from one PacketNumber to the other
// +gen linkedlist
type PacketInterval struct {
	Start protocol.PacketNumber
	End   protocol.PacketNumber
}
