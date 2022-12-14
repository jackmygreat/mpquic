package ackhandler

import (
	"github.com/yyleeshine/mpquic/repository/lucas-clemente/quic-go/internal/wire"
)

// Returns a new slice with all non-retransmittable frames deleted.
func stripNonRetransmittableFrames(fs []wire.Frame) []wire.Frame {
	res := make([]wire.Frame, 0, len(fs))
	for _, f := range fs {
		if IsFrameRetransmittable(f) {
			res = append(res, f)
		}
	}
	return res
}

// IsFrameRetransmittable returns true if the frame should be retransmitted.
func IsFrameRetransmittable(f wire.Frame) bool {
	switch f.(type) {
	case *wire.StopWaitingFrame:
		return false
	case *wire.AckFrame:
		return false
	case *wire.StreamFrame:
		if f.(*wire.StreamFrame).UnreliableMarker {
			return false
		} else {
			return true
		}
	default:
		return true
	}
}

// HasRetransmittableFrames returns true if at least one frame is retransmittable.
func HasRetransmittableFrames(fs []wire.Frame) bool {
	for _, f := range fs {
		if IsFrameRetransmittable(f) {
			return true
		}
	}
	return false
}
