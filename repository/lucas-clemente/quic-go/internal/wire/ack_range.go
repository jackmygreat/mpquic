package wire

import "github.com/yyleeshine/mpquic/repository/lucas-clemente/quic-go/internal/protocol"

// AckRange is an ACK range
type AckRange struct {
	First protocol.PacketNumber//最小的
	Last  protocol.PacketNumber
}
