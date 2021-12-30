package quic

import (
	"fmt"
	"time"

	"github.com/yyleeshine/mpquic/repository/lucas-clemente/quic-go/ackhandler"
	"github.com/yyleeshine/mpquic/repository/lucas-clemente/quic-go/internal/protocol"
	"github.com/yyleeshine/mpquic/repository/lucas-clemente/quic-go/internal/utils"
	"github.com/yyleeshine/mpquic/repository/lucas-clemente/quic-go/internal/wire"
)

type scheduler struct {
	// XXX Currently round-robin based, inspired from MPTCP scheduler
	quotas map[protocol.PathID]uint
	sess      *session

}

func (sch *scheduler) setup() {
	sch.quotas = make(map[protocol.PathID]uint)
}

func (sch *scheduler) getRetransmission(s *session) (hasRetransmission bool, retransmitPacket *ackhandler.Packet, pth *path) {// 这个是获取需要重传的报文的
	// check for retransmissions first
	for {
		// TODO add ability to reinject on another path
		// XXX We need to check on ALL paths if any packet should be first retransmitted
		// TODO 轮询的时候是否需要从上一次的位置开始？
		s.pathsLock.RLock()
	retransmitLoop:
		for _, pthTmp := range s.paths {//遍历每一条路径，获取需要重传的报文
			retransmitPacket = pthTmp.sentPacketHandler.DequeuePacketForRetransmission()//从pthTmp的sentPacketHandler当中拿需要重传的报文
			if retransmitPacket != nil {//拿到报文之后，立刻退出循环
				pth = pthTmp
				break retransmitLoop
			}
		}
		s.pathsLock.RUnlock()
		if retransmitPacket == nil {
			break
		}
		hasRetransmission = true

		if retransmitPacket.EncryptionLevel != protocol.EncryptionForwardSecure {
			if s.handshakeComplete {
				// 当握手结束的时候，不能重传握手报文
				// Don't retransmit handshake packets when the handshake is complete
				continue
			}
			utils.Debugf("\tDequeueing handshake retransmission for packet 0x%x", retransmitPacket.PacketNumber)
			return
		}
		utils.Debugf("\tDequeueing retransmission of packet 0x%x from path %d", retransmitPacket.PacketNumber, pth.pathID)
		// resend the frames that were in the packet
		for _, frame := range retransmitPacket.GetFramesForRetransmission() { // 从该报文当中拿出所有的frame

			switch f := frame.(type) {//丢失的报文可能包含多种的Frame类型
			case *wire.StreamFrame://如果是streamFrame的话，那么就判断其类型是否是unreliable的，如果是不可靠的话，那么就不需要重传
				if ok,_ := sch.sess.streamsMap.GetStreasmType(f.StreamID);ok {
						//不可靠传输的逻辑,直接什么也不做，放弃重传是否可行？，应该是不可以的，接收方的逻辑应该改变一下，一旦ack之后，那些确实的Frame应该不再管了
					fmt.Println("存在丢包")
					//s.streamFramer.AddFrameForRetransmission(f)
				}else {//如果是可靠的话，那么就需要进行重传
					s.streamFramer.AddFrameForRetransmission(f)
				}
				// todo windowUpdateFrame中的ByteOffset是什么意思？？
			case *wire.WindowUpdateFrame://如果是windowUpdate帧的话，
				// only retransmit WindowUpdates if the stream is not yet closed and the we haven't sent another WindowUpdate with a higher ByteOffset for the stream
				// XXX Should it be adapted to multiple paths?
				currentOffset, err := s.flowControlManager.GetReceiveWindow(f.StreamID)
				if err == nil && f.ByteOffset >= currentOffset {
					s.packer.QueueControlFrame(f, pth)
				}
			case *wire.PathsFrame:
				// Schedule a new PATHS frame to send
				s.schedulePathsFrame()
			default:
				s.packer.QueueControlFrame(frame, pth)
			}
		}
	}
	return
}

func (sch *scheduler) selectPathRoundRobin(s *session, hasRetransmission bool, hasStreamRetransmission bool, fromPth *path) *path {
	if sch.quotas == nil {
		sch.setup()
	}

	// XXX Avoid using PathID 0 if there is more than 1 path
	if len(s.paths) <= 1 {
		if !hasRetransmission && !s.paths[protocol.InitialPathID].SendingAllowed() {
			return nil
		}
		return s.paths[protocol.InitialPathID]
	}

	// TODO cope with decreasing number of paths (needed?)
	var selectedPath *path
	var lowerQuota, currentQuota uint
	var ok bool

	// Max possible value for lowerQuota at the beginning
	lowerQuota = ^uint(0)

pathLoop:
	for pathID, pth := range s.paths {
		// Don't block path usage if we retransmit, even on another path
		if !hasRetransmission && !pth.SendingAllowed() {
			continue pathLoop
		}

		// If this path is potentially failed, do no consider it for sending
		if pth.potentiallyFailed.Get() {
			continue pathLoop
		}

		// XXX Prevent using initial pathID if multiple paths
		if pathID == protocol.InitialPathID {
			continue pathLoop
		}

		currentQuota, ok = sch.quotas[pathID]
		if !ok {
			sch.quotas[pathID] = 0
			currentQuota = 0
		}

		if currentQuota < lowerQuota {
			selectedPath = pth
			lowerQuota = currentQuota
		}
	}

	return selectedPath

}

func (sch *scheduler) selectPathLowLatency(s *session, hasRetransmission bool, hasStreamRetransmission bool, fromPth *path) *path {
	// XXX Avoid using PathID 0 if there is more than 1 path
	if len(s.paths) <= 1 {
		if !hasRetransmission && !s.paths[protocol.InitialPathID].SendingAllowed() {
			return nil
		}
		return s.paths[protocol.InitialPathID]
	}

	// FIXME Only works at the beginning... Cope with new paths during the connection
	if hasRetransmission && hasStreamRetransmission && fromPth.rttStats.SmoothedRTT() == 0 {
		// Is there any other path with a lower number of packet sent?
		currentQuota := sch.quotas[fromPth.pathID]
		for pathID, pth := range s.paths {
			if pathID == protocol.InitialPathID || pathID == fromPth.pathID {
				continue
			}
			// The congestion window was checked when duplicating the packet
			if sch.quotas[pathID] < currentQuota {
				return pth
			}
		}
	}

	var selectedPath *path
	var lowerRTT time.Duration
	var currentRTT time.Duration
	selectedPathID := protocol.PathID(255)

pathLoop:
	for pathID, pth := range s.paths {
		// Don't block path usage if we retransmit, even on another path
		if !hasRetransmission && !pth.SendingAllowed() {
			continue pathLoop
		}

		// If this path is potentially failed, do not consider it for sending
		if pth.potentiallyFailed.Get() {
			continue pathLoop
		}

		// XXX Prevent using initial pathID if multiple paths
		if pathID == protocol.InitialPathID {
			continue pathLoop
		}

		currentRTT = pth.rttStats.SmoothedRTT()

		// Prefer staying single-path if not blocked by current path
		// Don't consider this sample if the smoothed RTT is 0
		if lowerRTT != 0 && currentRTT == 0 {
			continue pathLoop
		}

		// Case if we have multiple paths unprobed
		if currentRTT == 0 {
			currentQuota, ok := sch.quotas[pathID]
			if !ok {
				sch.quotas[pathID] = 0
				currentQuota = 0
			}
			lowerQuota, _ := sch.quotas[selectedPathID]
			if selectedPath != nil && currentQuota > lowerQuota {
				continue pathLoop
			}
		}

		if currentRTT != 0 && lowerRTT != 0 && selectedPath != nil && currentRTT >= lowerRTT {
			continue pathLoop
		}

		// Update
		lowerRTT = currentRTT
		selectedPath = pth
		selectedPathID = pathID
	}

	return selectedPath
}

// Lock of s.paths must be held
func (sch *scheduler) selectPath(s *session, hasRetransmission bool, hasStreamRetransmission bool, fromPth *path) *path {
	// XXX Currently round-robin
	// TODO select the right scheduler dynamically
	return sch.selectPathLowLatency(s, hasRetransmission, hasStreamRetransmission, fromPth)
	// return sch.selectPathRoundRobin(s, hasRetransmission, hasStreamRetransmission, fromPth)
}

// Lock of s.paths must be free (in case of log print)
func (sch *scheduler) performPacketSending(s *session, windowUpdateFrames []*wire.WindowUpdateFrame, pth *path) (*ackhandler.Packet, bool, error) {
	// add a retransmittable frame
	if pth.sentPacketHandler.ShouldSendRetransmittablePacket() {
		s.packer.QueueControlFrame(&wire.PingFrame{}, pth)
	}
	packet, err := s.packer.PackPacket(pth)
	if err != nil || packet == nil {
		return nil, false, err
	}
	if err = s.sendPackedPacket(packet, pth); err != nil {
		return nil, false, err
	}

	// send every window update twice
	for _, f := range windowUpdateFrames {
		s.packer.QueueControlFrame(f, pth)
	}

	// Packet sent, so update its quota
	sch.quotas[pth.pathID]++

	// Provide some logging if it is the last packet
	for _, frame := range packet.frames {
		switch frame := frame.(type) {
		case *wire.StreamFrame:
			if frame.FinBit {
				// Last packet to send on the stream, print stats
				s.pathsLock.RLock()
				utils.Infof("Info for stream %x of %x", frame.StreamID, s.connectionID)
				for pathID, pth := range s.paths {
					sntPkts, sntRetrans, sntLost := pth.sentPacketHandler.GetStatistics()
					rcvPkts := pth.receivedPacketHandler.GetStatistics()
					utils.Infof("Path %x: sent %d retrans %d lost %d; rcv %d rtt %v", pathID, sntPkts, sntRetrans, sntLost, rcvPkts, pth.rttStats.SmoothedRTT())
				}
				s.pathsLock.RUnlock()
			}
		default:
		}
	}

	pkt := &ackhandler.Packet{
		PacketNumber:    packet.number,
		Frames:          packet.frames,
		Length:          protocol.ByteCount(len(packet.raw)),
		EncryptionLevel: packet.encryptionLevel,
	}

	return pkt, true, nil
}

// Lock of s.paths must be free
func (sch *scheduler) ackRemainingPaths(s *session, totalWindowUpdateFrames []*wire.WindowUpdateFrame) error {
	// Either we run out of data, or CWIN of usable paths are full
	// Send ACKs on paths not yet used, if needed. Either we have no data to send and
	// it will be a pure ACK, or we will have data in it, but the CWIN should then
	// not be an issue.
	s.pathsLock.RLock()
	defer s.pathsLock.RUnlock()
	// get WindowUpdate frames
	// this call triggers the flow controller to increase the flow control windows, if necessary
	windowUpdateFrames := totalWindowUpdateFrames
	if len(windowUpdateFrames) == 0 {
		windowUpdateFrames = s.getWindowUpdateFrames(s.peerBlocked)
	}
	for _, pthTmp := range s.paths {
		ackTmp := pthTmp.GetAckFrame()
		for _, wuf := range windowUpdateFrames {
			s.packer.QueueControlFrame(wuf, pthTmp)
		}
		if ackTmp != nil || len(windowUpdateFrames) > 0 {
			if pthTmp.pathID == protocol.InitialPathID && ackTmp == nil {
				continue
			}
			swf := pthTmp.GetStopWaitingFrame(false)
			if swf != nil {
				s.packer.QueueControlFrame(swf, pthTmp)
			}
			s.packer.QueueControlFrame(ackTmp, pthTmp)
			// XXX (QDC) should we instead call PackPacket to provides WUFs?
			var packet *packedPacket
			var err error
			if ackTmp != nil {
				// Avoid internal error bug
				packet, err = s.packer.PackAckPacket(pthTmp)
			} else {
				packet, err = s.packer.PackPacket(pthTmp)
			}
			if err != nil {
				return err
			}
			err = s.sendPackedPacket(packet, pthTmp)
			if err != nil {
				return err
			}
		}
	}
	s.peerBlocked = false
	return nil
}

func (sch *scheduler) sendPacket(s *session) error {
	var pth *path

	// Update leastUnacked value of paths
	s.pathsLock.RLock()
	for _, pthTmp := range s.paths {
		pthTmp.SetLeastUnacked(pthTmp.sentPacketHandler.GetLeastUnacked())
	}
	s.pathsLock.RUnlock()

	// get WindowUpdate frames
	// this call triggers the flow controller to increase the flow control windows, if necessary
	windowUpdateFrames := s.getWindowUpdateFrames(false)
	for _, wuf := range windowUpdateFrames {
		s.packer.QueueControlFrame(wuf, pth)
	}

	// Repeatedly try sending until we don't have any more data, or run out of the congestion window
	for {
		// We first check for retransmissions
		// 先询问会话是否存在需要重传的报文，第一个是bool值，代表了是否有重传的报文，第二个和第三个返回值只有重传的报文是握手的时候有意义分别代表了握手时需要重传的报文和发送的路径
		hasRetransmission, retransmitHandshakePacket, fromPth := sch.getRetransmission(s)
		// XXX There might still be some stream frames to be retransmitted
		hasStreamRetransmission := s.streamFramer.HasFramesForRetransmission()//查询是否有需要重传的Frame

		// Select the path here
		s.pathsLock.RLock()
		pth = sch.selectPath(s, hasRetransmission, hasStreamRetransmission, fromPth)
		s.pathsLock.RUnlock()

		// XXX No more path available, should we have a new QUIC error message?
		// 如果没有可用的path了，需要发送窗口更新帧？
		if pth == nil {
			windowUpdateFrames := s.getWindowUpdateFrames(false)
			return sch.ackRemainingPaths(s, windowUpdateFrames)
		}

		// If we have an handshake packet retransmission, do it directly
		if hasRetransmission && retransmitHandshakePacket != nil {
			s.packer.QueueControlFrame(pth.sentPacketHandler.GetStopWaitingFrame(true), pth)
			packet, err := s.packer.PackHandshakeRetransmission(retransmitHandshakePacket, pth)
			if err != nil {
				return err
			}
			if err = s.sendPackedPacket(packet, pth); err != nil {
				return err
			}
			continue
		}

		// XXX Some automatic ACK generation should be done someway
		var ack *wire.AckFrame

		ack = pth.GetAckFrame()
		if ack != nil {
			s.packer.QueueControlFrame(ack, pth)
		}
		if ack != nil || hasStreamRetransmission {
			swf := pth.sentPacketHandler.GetStopWaitingFrame(hasStreamRetransmission)
			if swf != nil {
				s.packer.QueueControlFrame(swf, pth)
			}
		}

		// Also add CLOSE_PATH frames, if any
		for cpf := s.streamFramer.PopClosePathFrame(); cpf != nil; cpf = s.streamFramer.PopClosePathFrame() {
			s.packer.QueueControlFrame(cpf, pth)
		}

		// Also add ADD ADDRESS frames, if any
		for aaf := s.streamFramer.PopAddAddressFrame(); aaf != nil; aaf = s.streamFramer.PopAddAddressFrame() {
			s.packer.QueueControlFrame(aaf, pth)
		}

		// Also add PATHS frames, if any
		for pf := s.streamFramer.PopPathsFrame(); pf != nil; pf = s.streamFramer.PopPathsFrame() {
			s.packer.QueueControlFrame(pf, pth)
		}

		pkt, sent, err := sch.performPacketSending(s, windowUpdateFrames, pth)
		if err != nil {
			return err
		}
		windowUpdateFrames = nil
		if !sent {
			// Prevent sending empty packets
			return sch.ackRemainingPaths(s, windowUpdateFrames)
		}

		// Duplicate traffic when it was sent on an unknown performing path
		// FIXME adapt for new paths coming during the connection
		if pth.rttStats.SmoothedRTT() == 0 {
			currentQuota := sch.quotas[pth.pathID]
			// Was the packet duplicated on all potential paths?
		duplicateLoop:
			for pathID, tmpPth := range s.paths {
				if pathID == protocol.InitialPathID || pathID == pth.pathID {
					continue
				}
				if sch.quotas[pathID] < currentQuota && tmpPth.sentPacketHandler.SendingAllowed() {
					// Duplicate it
					pth.sentPacketHandler.DuplicatePacket(pkt)
					break duplicateLoop
				}
			}
		}

		// And try pinging on potentially failed paths
		if fromPth != nil && fromPth.potentiallyFailed.Get() {
			err = s.sendPing(fromPth)
			if err != nil {
				return err
			}
		}
	}
}
