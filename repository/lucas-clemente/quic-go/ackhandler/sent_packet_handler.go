package ackhandler

import (
	"errors"
	"fmt"
	"time"

	"github.com/yyleeshine/mpquic/repository/lucas-clemente/quic-go/congestion"
	"github.com/yyleeshine/mpquic/repository/lucas-clemente/quic-go/internal/protocol"
	"github.com/yyleeshine/mpquic/repository/lucas-clemente/quic-go/internal/utils"
	"github.com/yyleeshine/mpquic/repository/lucas-clemente/quic-go/internal/wire"
	"github.com/yyleeshine/mpquic/repository/lucas-clemente/quic-go/qerr"
)

const (
	// Maximum reordering in time space before time based loss detection considers a packet lost.
	// In fraction of an RTT.
	timeReorderingFraction = 1.0 / 8
	// defaultRTOTimeout is the RTO time on new connections
	defaultRTOTimeout = 500 * time.Millisecond
	// Minimum time in the future an RTO alarm may be set for.
	minRTOTimeout = 200 * time.Millisecond
	// maxRTOTimeout is the maximum RTO time
	maxRTOTimeout = 60 * time.Second
	// Sends up to two tail loss probes before firing a RTO, as per
	// draft RFC draft-dukkipati-tcpm-tcp-loss-probe
	maxTailLossProbes = 2
	// TCP RFC calls for 1 second RTO however Linux differs from this default and
	// define the minimum RTO to 200ms, we will use the same until we have data to
	// support a higher or lower value
	minRetransmissionTime = 200 * time.Millisecond
	// Minimum tail loss probe time in ms
	minTailLossProbeTimeout = 10 * time.Millisecond
)

var (
	// ErrDuplicateOrOutOfOrderAck occurs when a duplicate or an out-of-order ACK is received
	ErrDuplicateOrOutOfOrderAck = errors.New("SentPacketHandler: Duplicate or out-of-order ACK")
	// ErrTooManyTrackedSentPackets occurs when the sentPacketHandler has to keep track of too many packets
	ErrTooManyTrackedSentPackets = errors.New("Too many outstanding non-acked and non-retransmitted packets")
	// ErrAckForSkippedPacket occurs when the client sent an ACK for a packet number that we intentionally skipped
	ErrAckForSkippedPacket = qerr.Error(qerr.InvalidAckData, "Received an ACK for a skipped packet number")
	errAckForUnsentPacket  = qerr.Error(qerr.InvalidAckData, "Received ACK for an unsent package")
)

var errPacketNumberNotIncreasing = errors.New("Already sent a packet with a higher packet number")

type sentPacketHandler struct {
	lastSentPacketNumber protocol.PacketNumber //上一次发送的报文序号
	skippedPackets       []protocol.PacketNumber //跳过的报文序号

	numNonRetransmittablePackets int // number of non-retransmittable packets since the last retransmittable packet 自从上次发送过可重传的报文之后发送的不可重传的报文的数量

	LargestAcked protocol.PacketNumber //最大被确认的报文序号

	largestReceivedPacketWithAck protocol.PacketNumber // 收到的ack的最大报文序号

	packetHistory      *PacketList //发送的报文历史数据
	stopWaitingManager stopWaitingManager

	retransmissionQueue []*Packet // 重传的报文的队列

	bytesInFlight protocol.ByteCount //在途中的报文数量

	congestion congestion.SendAlgorithm //拥塞控制算法
	rttStats   *congestion.RTTStats // 时延信息

	onRTOCallback func(time.Time) bool //超时后的回调函数

	// The number of times an RTO has been sent without receiving an ack.
	rtoCount uint32 //超时的次数

	// The number of times a TLP has been sent without receiving an ACK
	tlpCount uint32

	// The time at which the next packet will be considered lost based on early transmit or exceeding the reordering window in time.
	lossTime time.Time

	// The time the last packet was sent, used to set the retransmission timeout
	lastSentTime time.Time

	// The alarm timeout
	alarm time.Time // 通知超时的字段

	packets         uint64 // 发送的报文数
	retransmissions uint64 // 重传的次数
	losses          uint64 // 丢失的报文数目
}

// NewSentPacketHandler creates a new sentPacketHandler
func NewSentPacketHandler(rttStats *congestion.RTTStats, cong congestion.SendAlgorithm, onRTOCallback func(time.Time) bool) SentPacketHandler {
	var congestionControl congestion.SendAlgorithm // 拥塞控制算法

	if cong != nil {
		congestionControl = cong
	} else {//如果cong参数为空的话，那么证明该quic是单路径的，因此创建的拥塞控制算法是单路径的
		congestionControl = congestion.NewCubicSender(
			congestion.DefaultClock{},
			rttStats,
			false, /* don't use reno since chromium doesn't (why?) */
			protocol.InitialCongestionWindow,
			protocol.DefaultMaxCongestionWindow,
		)
	}

	return &sentPacketHandler{
		packetHistory:      NewPacketList(),
		stopWaitingManager: stopWaitingManager{},
		rttStats:           rttStats,
		congestion:         congestionControl,
		onRTOCallback:      onRTOCallback, //超时的回调函数
	}
}

func (h *sentPacketHandler) GetStatistics() (uint64, uint64, uint64) {// 取出该路径的数据，发送报文数、
	return h.packets, h.retransmissions, h.losses
}

func (h *sentPacketHandler) largestInOrderAcked() protocol.PacketNumber {// 这个返回的是那些被确认的报文的最大序号
	if f := h.packetHistory.Front(); f != nil {
		return f.Value.PacketNumber - 1
	}
	return h.LargestAcked
}

// MaxNonRetransmittablePackets is the maximum number of non-retransmittable packets that we send in a row
func (h *sentPacketHandler) ShouldSendRetransmittablePacket() bool {
	return h.numNonRetransmittablePackets >= protocol.MaxNonRetransmittablePackets
}

func (h *sentPacketHandler) SentPacket(packet *Packet) error {
	if packet.PacketNumber <= h.lastSentPacketNumber { // 报文的序号在发送的时候一定是递增的
		return errPacketNumberNotIncreasing
	}
	// MaxTrackedSentPackets is maximum number of sent packets saved for either later retransmission or entropy calculation
	// 也就是说一个path在发送的时候不能积压太多的发送了但是还未被确认的报文
	if protocol.PacketNumber(len(h.retransmissionQueue)+h.packetHistory.Len()+1) > protocol.MaxTrackedSentPackets {
		return ErrTooManyTrackedSentPackets
	}
	//
	for p := h.lastSentPacketNumber + 1; p < packet.PacketNumber; p++ {
		h.skippedPackets = append(h.skippedPackets, p) // 将那些跳过的报文序号记录下来，以便确认的时候不会将这些报文序号标记为丢失
		// MaxTrackedSkippedPackets is the maximum number of skipped packet numbers the SentPacketHandler keep track of for Optimistic ACK attack mitigation
		if len(h.skippedPackets) > protocol.MaxTrackedSkippedPackets { //
			h.skippedPackets = h.skippedPackets[1:]
		}
	}

	h.lastSentPacketNumber = packet.PacketNumber
	now := time.Now()

	// Update some statistics
	h.packets++

	// XXX RTO and TLP are recomputed based on the possible last sent retransmission. Is it ok like this?
	h.lastSentTime = now

	// 将报文中的那些非可重传的报文给过滤掉，只记录那些可重传的frame，以便丢失的时候重传
	packet.Frames = stripNonRetransmittableFrames(packet.Frames)//这个时候是否需要修改一下packet的length？
	isRetransmittable := len(packet.Frames) != 0

	if isRetransmittable {//可重传的报文需要记录下来
		packet.SendTime = now
		h.bytesInFlight += packet.Length
		h.packetHistory.PushBack(*packet)
		h.numNonRetransmittablePackets = 0
	} else {
		h.numNonRetransmittablePackets++
	}

	h.congestion.OnPacketSent(
		now,
		h.bytesInFlight,
		packet.PacketNumber,
		packet.Length,
		isRetransmittable,
	)

	h.updateLossDetectionAlarm() // 报文丢失的alarm
	return nil
}

func (h *sentPacketHandler) ReceivedAck(ackFrame *wire.AckFrame, withPacketNumber protocol.PacketNumber, rcvTime time.Time) error {
	if ackFrame.LargestAcked > h.lastSentPacketNumber {//接收到的ackFrame的确认的报文序号大于在该路径上发送的报文序号，那肯定报错啊，属于薛定谔的报文序号了
		return errAckForUnsentPacket
	}

	// duplicate or out-of-order ACK
	if withPacketNumber <= h.largestReceivedPacketWithAck {//这一个是为了啥？？？？？
		return ErrDuplicateOrOutOfOrderAck
	}
	h.largestReceivedPacketWithAck = withPacketNumber

	// ignore repeated ACK (ACKs that don't have a higher LargestAcked than the last ACK)
	if ackFrame.LargestAcked <= h.largestInOrderAcked() {//重复的ack
		return nil
	}
	h.LargestAcked = ackFrame.LargestAcked//更新确认的最大的报文序号

	if h.skippedPacketsAcked(ackFrame) {// 确认了包含从未发送过的报文序号，那么就会报错
		return ErrAckForSkippedPacket
	}

	rttUpdated := h.maybeUpdateRTT(ackFrame.LargestAcked, ackFrame.DelayTime, rcvTime)

	if rttUpdated {
		h.congestion.MaybeExitSlowStart()
	}

	ackedPackets, err := h.determineNewlyAckedPackets(ackFrame)//拿到所有的报文序号
	if err != nil {
		return err
	}

	if len(ackedPackets) > 0 {
		for _, p := range ackedPackets {
			h.onPacketAcked(p)// 所做的工作就是将该报文从 报文的存档中删除掉， bytesInflight也要减小
			h.congestion.OnPacketAcked(p.Value.PacketNumber, p.Value.Length, h.bytesInFlight)//调整拥塞
		}
	}
	// todo begin----------------------------------
	h.detectLostPackets() //检测丢失的报文
	h.updateLossDetectionAlarm() // 更新Alarm

	h.garbageCollectSkippedPackets()
	h.stopWaitingManager.ReceivedAck(ackFrame) // 这个是在干啥？？
	// todo ------------------------------------end
	return nil
}
// 接收到 closePathFrame 的话
func (h *sentPacketHandler) ReceivedClosePath(f *wire.ClosePathFrame, withPacketNumber protocol.PacketNumber, rcvTime time.Time) error {
	if f.LargestAcked > h.lastSentPacketNumber {
		return errAckForUnsentPacket
	}

	// this should never happen, since a closePath frame should be the last packet on a path
	if withPacketNumber <= h.largestReceivedPacketWithAck {
		return ErrDuplicateOrOutOfOrderAck
	}
	h.largestReceivedPacketWithAck = withPacketNumber

	// Compared to ACK frames, we should not ignore duplicate LargestAcked

	if h.skippedPacketsAckedClosePath(f) {
		return ErrAckForSkippedPacket
	}

	// No need for RTT estimation

	ackedPackets, err := h.determineNewlyAckedPacketsClosePath(f)
	if err != nil {
		return err
	}

	if len(ackedPackets) > 0 {
		for _, p := range ackedPackets {
			h.onPacketAcked(p)
			h.congestion.OnPacketAcked(p.Value.PacketNumber, p.Value.Length, h.bytesInFlight)
		}
	}

	h.SetInflightAsLost() // 因为该路径被关闭了，因此被关闭之后该path上还未被确认的所有报文都将会被视作丢失

	h.garbageCollectSkippedPackets()
	// We do not send any STOP WAITING Frames, so no need to update the manager

	return nil
}
// 从ackFrame当中确定所有被确认的报文
func (h *sentPacketHandler) determineNewlyAckedPackets(ackFrame *wire.AckFrame) ([]*PacketElement, error) {
	var ackedPackets []*PacketElement
	ackRangeIndex := 0
	for el := h.packetHistory.Front(); el != nil; el = el.Next() {
		packet := el.Value
		packetNumber := packet.PacketNumber

		// Ignore packets below the LowestAcked
		if packetNumber < ackFrame.LowestAcked {
			continue
		}
		// Break after LargestAcked is reached
		if packetNumber > ackFrame.LargestAcked {
			break
		}

		if ackFrame.HasMissingRanges() {
			ackRange := ackFrame.AckRanges[len(ackFrame.AckRanges)-1-ackRangeIndex]//从最后一个开始遍历

			for packetNumber > ackRange.Last && ackRangeIndex < len(ackFrame.AckRanges)-1 {
				ackRangeIndex++
				ackRange = ackFrame.AckRanges[len(ackFrame.AckRanges)-1-ackRangeIndex]
			}

			if packetNumber >= ackRange.First { // packet i contained in ACK range
				if packetNumber > ackRange.Last {
					return nil, fmt.Errorf("BUG: ackhandler would have acked wrong packet 0x%x, while evaluating range 0x%x -> 0x%x", packetNumber, ackRange.First, ackRange.Last)
				}
				ackedPackets = append(ackedPackets, el)
			}
		} else {
			ackedPackets = append(ackedPackets, el)
		}
	}

	return ackedPackets, nil
}
// 从ClosePathFrame当中确定所有被确认的报文
func (h *sentPacketHandler) determineNewlyAckedPacketsClosePath(f *wire.ClosePathFrame) ([]*PacketElement, error) {
	var ackedPackets []*PacketElement
	ackRangeIndex := 0
	for el := h.packetHistory.Front(); el != nil; el = el.Next() {
		packet := el.Value
		packetNumber := packet.PacketNumber

		// Ignore packets below the LowestAcked
		if packetNumber < f.LowestAcked {
			continue
		}
		// Break after LargestAcked is reached
		if packetNumber > f.LargestAcked {
			break
		}

		if f.HasMissingRanges() {
			ackRange := f.AckRanges[len(f.AckRanges)-1-ackRangeIndex]

			for packetNumber > ackRange.Last && ackRangeIndex < len(f.AckRanges)-1 {
				ackRangeIndex++
				ackRange = f.AckRanges[len(f.AckRanges)-1-ackRangeIndex]
			}

			if packetNumber >= ackRange.First { // packet i contained in ACK range
				if packetNumber > ackRange.Last {
					return nil, fmt.Errorf("BUG: ackhandler would have acked wrong packet 0x%x, while evaluating range 0x%x -> 0x%x with ClosePath frame", packetNumber, ackRange.First, ackRange.Last)
				}
				ackedPackets = append(ackedPackets, el)
			}
		} else {
			ackedPackets = append(ackedPackets, el)
		}
	}

	return ackedPackets, nil
}

func (h *sentPacketHandler) maybeUpdateRTT(largestAcked protocol.PacketNumber, ackDelay time.Duration, rcvTime time.Time) bool {
	for el := h.packetHistory.Front(); el != nil; el = el.Next() {
		packet := el.Value
		if packet.PacketNumber == largestAcked {// 找到那个发送的报文，从中提取出发送的时间戳
			h.rttStats.UpdateRTT(rcvTime.Sub(packet.SendTime), ackDelay, time.Now())
			return true
		}
		// Packets are sorted by number, so we can stop searching
		if packet.PacketNumber > largestAcked {
			break
		}
	}
	return false
}

func (h *sentPacketHandler) hasOutstandingRetransmittablePacket() bool {//查询是否存在仍然还在途中未被确认的含有streamFrame的报文
	for el := h.packetHistory.Front(); el != nil; el = el.Next() {
		if el.Value.IsRetransmittable() {
			return true
		}
	}
	return false
}
// 这个函数是为了更新超时的alarm，每收到一个 ackFrame 都会更新一次
func (h *sentPacketHandler) updateLossDetectionAlarm() {
	// Cancel the alarm if no packets are outstanding
	if h.packetHistory.Len() == 0 {//如果没有还未被确认的报文
		h.alarm = time.Time{}//重置计时器
		return
	}

	// TODO(#496): Handle handshake packets separately
	if !h.lossTime.IsZero() {//
		// Early retransmit timer or time loss detection.
		h.alarm = h.lossTime
	} else if h.rttStats.SmoothedRTT() != 0 && h.tlpCount < maxTailLossProbes {
		// TLP
		h.alarm = h.lastSentTime.Add(h.computeTLPTimeout())
	} else {
		// RTO
		h.alarm = h.lastSentTime.Add(utils.MaxDuration(h.computeRTOTimeout(), minRetransmissionTime))
	}
}

func (h *sentPacketHandler) detectLostPackets() {
	h.lossTime = time.Time{}
	now := time.Now()

	maxRTT := float64(utils.MaxDuration(h.rttStats.LatestRTT(), h.rttStats.SmoothedRTT()))
	delayUntilLost := time.Duration((1.0 + timeReorderingFraction) * maxRTT)

	var lostPackets []*PacketElement
	for el := h.packetHistory.Front(); el != nil; el = el.Next() {
		packet := el.Value

		if packet.PacketNumber > h.LargestAcked {
			break
		}

		timeSinceSent := now.Sub(packet.SendTime)
		if timeSinceSent > delayUntilLost {
			// Update statistics
			h.losses++
			lostPackets = append(lostPackets, el)
		} else if h.lossTime.IsZero() {
			// Note: This conditional is only entered once per call
			h.lossTime = now.Add(delayUntilLost - timeSinceSent)
		}
	}

	if len(lostPackets) > 0 {
		for _, p := range lostPackets {
			h.queuePacketForRetransmission(p) // 将所有需要重传的报文加入到队列当中去
			h.congestion.OnPacketLost(p.Value.PacketNumber, p.Value.Length, h.bytesInFlight)
		}
	}
}

func (h *sentPacketHandler) SetInflightAsLost() {
	var lostPackets []*PacketElement
	for el := h.packetHistory.Front(); el != nil; el = el.Next() {
		packet := el.Value

		if packet.PacketNumber > h.LargestAcked { // 为什么大于的时候脱离循环呢？
			break
		}

		h.losses++
		lostPackets = append(lostPackets, el)
	}

	if len(lostPackets) > 0 {
		for _, p := range lostPackets {
			h.queuePacketForRetransmission(p) // 将所有被视作为丢失的报文给缓存起来
			// XXX (QDC): should we?
			h.congestion.OnPacketLost(p.Value.PacketNumber, p.Value.Length, h.bytesInFlight)
		}
	}
}

func (h *sentPacketHandler) OnAlarm() {
	// Do we really have packet to retransmit?
	if !h.hasOutstandingRetransmittablePacket() {
		// Cancel then the alarm
		h.alarm = time.Time{}
		return
	}

	// TODO(#496): Handle handshake packets separately
	if !h.lossTime.IsZero() {
		// Early retransmit or time loss detection
		h.detectLostPackets()

	} else if h.tlpCount < maxTailLossProbes {
		// TLP
		h.retransmitTLP()
		h.tlpCount++
	} else {//超时的情况
		// RTO
		potentiallyFailed := false
		if h.onRTOCallback != nil {
			potentiallyFailed = h.onRTOCallback(h.lastSentTime)
		}
		if potentiallyFailed {
			h.retransmitAllPackets()
		} else {
			h.retransmitOldestTwoPackets()
		}
		h.rtoCount++
	}

	h.updateLossDetectionAlarm()
}

func (h *sentPacketHandler) GetAlarmTimeout() time.Time {
	return h.alarm
}

func (h *sentPacketHandler) onPacketAcked(packetElement *PacketElement) {
	h.bytesInFlight -= packetElement.Value.Length
	h.rtoCount = 0
	h.tlpCount = 0
	h.packetHistory.Remove(packetElement)
}
// 这个函数是为了拿出一个需要重传的报文
func (h *sentPacketHandler) DequeuePacketForRetransmission() *Packet {
	if len(h.retransmissionQueue) == 0 {
		return nil
	}
	packet := h.retransmissionQueue[0]
	// Shift the slice and don't retain anything that isn't needed.
	copy(h.retransmissionQueue, h.retransmissionQueue[1:])
	h.retransmissionQueue[len(h.retransmissionQueue)-1] = nil
	h.retransmissionQueue = h.retransmissionQueue[:len(h.retransmissionQueue)-1]
	// Update statistics
	h.retransmissions++
	return packet
}
// 拿到最小的未被确认的报文
func (h *sentPacketHandler) GetLeastUnacked() protocol.PacketNumber {
	return h.largestInOrderAcked() + 1
}
//
func (h *sentPacketHandler) GetStopWaitingFrame(force bool) *wire.StopWaitingFrame {
	return h.stopWaitingManager.GetStopWaitingFrame(force)
}
// 查询本path是否还允许发送报文，会查看本条路径上有没有需要重传的报文、拥塞窗口是否还够等，如果本条路径有需要重传的报文，那么必定返回true
func (h *sentPacketHandler) SendingAllowed() bool {
	congestionLimited := h.bytesInFlight > h.congestion.GetCongestionWindow()
	maxTrackedLimited := protocol.PacketNumber(len(h.retransmissionQueue)+h.packetHistory.Len()) >= protocol.MaxTrackedSentPackets
	if congestionLimited {
		utils.Debugf("Congestion limited: bytes in flight %d, window %d",
			h.bytesInFlight,
			h.congestion.GetCongestionWindow())
	}
	// Workaround for #555:
	// Always allow sending of retransmissions. This should probably be limited
	// to RTOs, but we currently don't have a nice way of distinguishing them.
	haveRetransmissions := len(h.retransmissionQueue) > 0
	return !maxTrackedLimited && (!congestionLimited || haveRetransmissions)
}

func (h *sentPacketHandler) retransmitTLP() {
	if p := h.packetHistory.Back(); p != nil {
		h.queuePacketForRetransmission(p)
	}
}

func (h *sentPacketHandler) retransmitAllPackets() {//这个意思是需要重传所有的报文？
	for h.packetHistory.Len() > 0 {
		h.queueRTO(h.packetHistory.Front())
	}
	h.congestion.OnRetransmissionTimeout(true)
}

func (h *sentPacketHandler) retransmitOldestPacket() {//重传最老的报文
	if p := h.packetHistory.Front(); p != nil {
		h.queueRTO(p)
	}
}

func (h *sentPacketHandler) retransmitOldestTwoPackets() {
	h.retransmitOldestPacket()
	h.retransmitOldestPacket()
	h.congestion.OnRetransmissionTimeout(true)
}

func (h *sentPacketHandler) queueRTO(el *PacketElement) {
	packet := &el.Value
	utils.Debugf(
		"\tQueueing packet 0x%x for retransmission (RTO), %d outstanding",
		packet.PacketNumber,
		h.packetHistory.Len(),
	)
	h.queuePacketForRetransmission(el)
	h.losses++
	h.congestion.OnPacketLost(packet.PacketNumber, packet.Length, h.bytesInFlight)
}

func (h *sentPacketHandler) queuePacketForRetransmission(packetElement *PacketElement) {
	packet := &packetElement.Value
	h.bytesInFlight -= packet.Length
	h.retransmissionQueue = append(h.retransmissionQueue, packet)
	h.packetHistory.Remove(packetElement)
	h.stopWaitingManager.QueuedRetransmissionForPacketNumber(packet.PacketNumber)
}

func (h *sentPacketHandler) DuplicatePacket(packet *Packet) {
	h.retransmissionQueue = append(h.retransmissionQueue, packet)
}

func (h *sentPacketHandler) computeRTOTimeout() time.Duration {
	rto := h.congestion.RetransmissionDelay()
	if rto == 0 {
		rto = defaultRTOTimeout
	}
	rto = utils.MaxDuration(rto, minRTOTimeout)
	// Exponential backoff
	rto = rto << h.rtoCount
	return utils.MinDuration(rto, maxRTOTimeout)
}

func (h *sentPacketHandler) hasMultipleOutstandingRetransmittablePackets() bool {
	return h.packetHistory.Front() != nil && h.packetHistory.Front().Next() != nil
}

func (h *sentPacketHandler) computeTLPTimeout() time.Duration {
	rtt := h.congestion.SmoothedRTT()
	if h.hasMultipleOutstandingRetransmittablePackets() {
		return utils.MaxDuration(2*rtt, rtt*3/2+minRetransmissionTime/2)
	}
	return utils.MaxDuration(2*rtt, minTailLossProbeTimeout)
}

func (h *sentPacketHandler) skippedPacketsAcked(ackFrame *wire.AckFrame) bool { //如果这个frame确认了自己从未发送过的报文序号，那肯定是要报错的
	for _, p := range h.skippedPackets {
		if ackFrame.AcksPacket(p) {
			return true
		}
	}
	return false
}

func (h *sentPacketHandler) skippedPacketsAckedClosePath(closePathFrame *wire.ClosePathFrame) bool {
	for _, p := range h.skippedPackets {
		if closePathFrame.AcksPacket(p) {
			return true
		}
	}
	return false
}

func (h *sentPacketHandler) garbageCollectSkippedPackets() {
	lioa := h.largestInOrderAcked()
	deleteIndex := 0
	for i, p := range h.skippedPackets {
		if p <= lioa {
			deleteIndex = i + 1
		}
	}
	h.skippedPackets = h.skippedPackets[deleteIndex:]
}
