package utils

import "github.com/yyleeshine/mpquic/repository/lucas-clemente/quic-go/internal/protocol"

// ByteInterval is an interval from one ByteCount to the other
// +gen linkedlist
type ByteInterval struct {
	Start protocol.ByteCount
	End   protocol.ByteCount
}
