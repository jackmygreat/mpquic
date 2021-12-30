package ackhandler

import (
	"errors"
	"time"

	"github.com/yyleeshine/mpquic/repository/lucas-clemente/quic-go/internal/protocol"
	"github.com/yyleeshine/mpquic/repository/lucas-clemente/quic-go/internal/wire"
)

var errInvalidPacketNumber = errors.New("ReceivedPacketHandler: Invalid packet number")

type receivedPacketHandler struct {
	largestObserved             protocol.PacketNumber //接收到的最大的
	lowerLimit                  protocol.PacketNumber //用来限制最小报文序号
	largestObservedReceivedTime time.Time

	packetHistory *receivedPacketHistory

	ackSendDelay time.Duration //从接收报文到ack发送之间所经历的延迟

	packetsReceivedSinceLastAck                int
	retransmittablePacketsReceivedSinceLastAck int
	ackQueued                                  bool //当这个字段为true的时候，证明已经满足发送ackFrame的条件了
	ackAlarm                                   time.Time //这个字段的意思是在队列当中的报文序号不足以ack,但是超过了该时间，仍然需要发送ack
	lastAck                                    *wire.AckFrame // 上一次确认过的数据包的范围的历史数据

	version protocol.VersionNumber

	packets uint64
}

// NewReceivedPacketHandler creates a new receivedPacketHandler
func NewReceivedPacketHandler(version protocol.VersionNumber) ReceivedPacketHandler {
	return &receivedPacketHandler{
		packetHistory: newReceivedPacketHistory(),
		ackSendDelay:  protocol.AckSendDelay,
		version:       version,
	}
}

func (h *receivedPacketHandler) GetStatistics() uint64 {// 已经发送的报文的总数
	return h.packets
}

func (h *receivedPacketHandler) ReceivedPacket(packetNumber protocol.PacketNumber, shouldInstigateAck bool) error {
	if packetNumber == 0 {
		return errInvalidPacketNumber
	}

	// A new packet was received on that path and passes checks, so count it for stats
	h.packets++

	if packetNumber > h.largestObserved {// 更新一下收到的最大序号和接收时间
		h.largestObserved = packetNumber
		h.largestObservedReceivedTime = time.Now()
	}

	if packetNumber <= h.lowerLimit {
		return nil
	}

	if err := h.packetHistory.ReceivedPacket(packetNumber); err != nil {//接收报文序列号的历史轨迹数据,将该报文序号添加到其中
		return err
	}
	h.maybeQueueAck(packetNumber, shouldInstigateAck)
	return nil
}

// SetLowerLimit sets a lower limit for acking packets.
// Packets with packet numbers smaller or equal than p will not be acked.
func (h *receivedPacketHandler) SetLowerLimit(p protocol.PacketNumber) {
	h.lowerLimit = p
	h.packetHistory.DeleteUpTo(p)
}

func (h *receivedPacketHandler) maybeQueueAck(packetNumber protocol.PacketNumber, shouldInstigateAck bool) {
	// 自上一次ack之后再次收到的报文数量
	h.packetsReceivedSinceLastAck++

	if shouldInstigateAck {//如果是可重传报文的话，自上一次ack自后，需要发送ack的报文接收数量
		h.retransmittablePacketsReceivedSinceLastAck++
	}

	// always ack the first packet
	if h.lastAck == nil {
		h.ackQueued = true
	}

	if h.version < protocol.Version39 {
		// Always send an ack every 20 packets in order to allow the peer to discard
		// information from the SentPacketManager and provide an RTT measurement.
		// From QUIC 39, this is not needed anymore, since the peer will regularly send a retransmittable packet.
		if h.packetsReceivedSinceLastAck >= protocol.MaxPacketsReceivedBeforeAckSend {
			h.ackQueued = true
		}
	}

	// if the packet number is smaller than the largest acked packet, it must have been reported missing with the last ACK
	// note that it cannot be a duplicate because they're already filtered out by ReceivedPacket()
	if h.lastAck != nil && packetNumber < h.lastAck.LargestAcked {
		h.ackQueued = true
	}

	// check if a new missing range above the previously was created
	if h.lastAck != nil && h.packetHistory.GetHighestAckRange().First > h.lastAck.LargestAcked {
		h.ackQueued = true
	}

	if !h.ackQueued && shouldInstigateAck {
		if h.retransmittablePacketsReceivedSinceLastAck >= protocol.RetransmittablePacketsBeforeAck {
			h.ackQueued = true
		} else {
			if h.ackAlarm.IsZero() {
				h.ackAlarm = time.Now().Add(h.ackSendDelay)
			}
		}
	}

	if h.ackQueued {
		// cancel the ack alarm
		h.ackAlarm = time.Time{}
	}
}

func (h *receivedPacketHandler) GetAckFrame() *wire.AckFrame {//这个要做工作嘛？？？
	if !h.ackQueued && (h.ackAlarm.IsZero() || h.ackAlarm.After(time.Now())) {
		return nil
	}

	ackRanges := h.packetHistory.GetAckRanges()
	ack := &wire.AckFrame{//
		LargestAcked:       h.largestObserved,
		LowestAcked:        ackRanges[len(ackRanges)-1].First,
		PacketReceivedTime: h.largestObservedReceivedTime,
	}

	if len(ackRanges) > 1 {
		ack.AckRanges = ackRanges
	}

	h.lastAck = ack
	h.ackAlarm = time.Time{}
	h.ackQueued = false
	h.packetsReceivedSinceLastAck = 0
	h.retransmittablePacketsReceivedSinceLastAck = 0

	return ack
}

func (h *receivedPacketHandler) GetClosePathFrame() *wire.ClosePathFrame {
	ackRanges := h.packetHistory.GetAckRanges()
	frame := &wire.ClosePathFrame{
		LargestAcked: h.largestObserved,
		LowestAcked:  ackRanges[len(ackRanges)-1].First,
	}

	if len(ackRanges) > 1 {
		frame.AckRanges = ackRanges
	}

	return frame
}

func (h *receivedPacketHandler) GetAlarmTimeout() time.Time { return h.ackAlarm }
