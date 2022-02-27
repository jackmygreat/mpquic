package quic

import (
	"time"

	"github.com/yyleeshine/mpquic/repository/lucas-clemente/quic-go/ackhandler"
	"github.com/yyleeshine/mpquic/repository/lucas-clemente/quic-go/congestion"
	"github.com/yyleeshine/mpquic/repository/lucas-clemente/quic-go/internal/protocol"
	"github.com/yyleeshine/mpquic/repository/lucas-clemente/quic-go/internal/utils"
	"github.com/yyleeshine/mpquic/repository/lucas-clemente/quic-go/internal/wire"
	"github.com/yyleeshine/mpquic/repository/lucas-clemente/quic-go/qerr"
)

const (
	minPathTimer = 10 * time.Millisecond
	// XXX (QDC): To avoid idling...
	maxPathTimer = 1 * time.Second
)

type path struct {
	pathID protocol.PathID
	conn   connection //关于该path的那个套接字
	sess   *session

	rttStats *congestion.RTTStats

	sentPacketHandler     ackhandler.SentPacketHandler // 发送数据的Handler，所有在该路径上发送的报文都会进行存档，除非收到对应存档的ack之后，这些存档才会被删除
	receivedPacketHandler ackhandler.ReceivedPacketHandler //接收数据的Handler，收到的报文的序号都会被存档，而不会保留报文的数据，到达一定的阶段之后，会发送ACK

	open      utils.AtomicBool // 用来查询该路径是开启的还是关闭的
	closeChan chan *qerr.QuicError // 当发生一些错误的时候，会用该管道通知该path
	runClosed chan struct{}

	potentiallyFailed utils.AtomicBool // 该路径很可能down掉了？

	sentPacket          chan struct{}  //比如一个session使用该path发送过数据之后，那么就会使用该管道通知该path，该path会更新一些计时器之类的东西

	// It is now the responsibility of the path to keep its packet number
	packetNumberGenerator *packetNumberGenerator
	// 上一次收到的报文的序号
	lastRcvdPacketNumber protocol.PacketNumber
	// Used to calculate the next packet number from the truncated wire
	// representation, and sent back in public reset packets
	// 所收到的最大的报文序号
	largestRcvdPacketNumber protocol.PacketNumber
	// 最小的未确认的报文序号
	leastUnacked protocol.PacketNumber

	lastNetworkActivityTime time.Time

	timer           *utils.Timer
}

// setup initializes values that are independent of the perspective
func (p *path) setup(oliaSenders map[protocol.PathID]*congestion.OliaSender) {
	p.rttStats = &congestion.RTTStats{}

	var cong congestion.SendAlgorithm

	if p.sess.version >= protocol.VersionMP && oliaSenders != nil && p.pathID != protocol.InitialPathID {// 如果是多路径的QUIC, 那么就要创建对应的多路径拥塞算法
		cong = congestion.NewOliaSender(oliaSenders, p.rttStats, protocol.InitialCongestionWindow, protocol.DefaultMaxCongestionWindow)
		oliaSenders[p.pathID] = cong.(*congestion.OliaSender)
	}

	sentPacketHandler := ackhandler.NewSentPacketHandler(p.rttStats, cong, p.onRTO)// 创建发送的处理器

	now := time.Now()

	p.sentPacketHandler = sentPacketHandler
	p.receivedPacketHandler = ackhandler.NewReceivedPacketHandler(p.sess.version) //创建接收的处理器

	p.packetNumberGenerator = newPacketNumberGenerator(protocol.SkipPacketAveragePeriodLength) //创建报文序号的生成器

	p.closeChan = make(chan *qerr.QuicError, 1)
	p.runClosed = make(chan struct{}, 1)
	p.sentPacket = make(chan struct{}, 1)//创建对应的三种管道

	p.timer = utils.NewTimer() //为该路径创建一个计时器
	p.lastNetworkActivityTime = now //上次该条路径出现网络活动的时间

	p.open.Set(true)  // 初始化的时候，该路径的状态肯定被设置为打开状态
	p.potentiallyFailed.Set(false) // 初始化的时候，该路径的状态被设置为false

	// Once the path is setup, run it
	go p.run()
}

func (p *path) close() error {
	p.open.Set(false)
	return nil
}
// 其实这个run函数的目的就是用来接收各种来自session的事件通知
func (p *path) run() {
	// XXX (QDC): relay everything to the session, maybe not the most efficient
runLoop:
	for {
		// Close immediately if requested
		select {
		case <-p.closeChan://发生错误了，这个path会被关闭，要跳出循环
			break runLoop
		default:
		}

		p.maybeResetTimer()//每一轮都要更新计时器

		select {
		case <-p.closeChan:
			break runLoop
		case <-p.timer.Chan()://当发生超时的时候
			p.timer.SetRead()
			select {
			case p.sess.pathTimers <- p:
			// XXX (QDC): don't remain stuck here!
			case <-p.closeChan:
				break runLoop
			case <-p.sentPacket:
				// Don't remain stuck here!
			}
		case <-p.sentPacket://当这个路径被用来发送数据后，那么，
			// Used to reset the path timer
		}
	}
	p.close()
	p.runClosed <- struct{}{}
}

func (p *path) SendingAllowed() bool { // 查询该路径是否允许发送数据
	return p.open.Get() && p.sentPacketHandler.SendingAllowed()
}

func (p *path) GetStopWaitingFrame(force bool) *wire.StopWaitingFrame { // stopWaitingFrame究竟是用来干什么的？
	return p.sentPacketHandler.GetStopWaitingFrame(force)
}

func (p *path) GetAckFrame() *wire.AckFrame {// 从receivedPacketHandler当中拿到ACK frame
	ack := p.receivedPacketHandler.GetAckFrame()
	if ack != nil {
		ack.PathID = p.pathID
	}

	return ack
}

func (p *path) GetClosePathFrame() *wire.ClosePathFrame { // 从receivedPacketHandler当中拿到close path frame
	closePathFrame := p.receivedPacketHandler.GetClosePathFrame()
	if closePathFrame != nil {
		closePathFrame.PathID = p.pathID
	}

	return closePathFrame
}

func (p *path) maybeResetTimer() {
	deadline := p.lastNetworkActivityTime.Add(p.idleTimeout())

	if ackAlarm := p.receivedPacketHandler.GetAlarmTimeout(); !ackAlarm.IsZero() {
		deadline = ackAlarm
	}
	if lossTime := p.sentPacketHandler.GetAlarmTimeout(); !lossTime.IsZero() {
		deadline = utils.MinTime(deadline, lossTime)
	}

	deadline = utils.MinTime(utils.MaxTime(deadline, time.Now().Add(minPathTimer)), time.Now().Add(maxPathTimer))

	p.timer.Reset(deadline)
}

func (p *path) idleTimeout() time.Duration {
	// TODO (QDC): probably this should be refined at path level
	cryptoSetup := p.sess.cryptoSetup
	if cryptoSetup != nil {
		if p.open.Get() && (p.pathID != 0 || p.sess.handshakeComplete) {
			return p.sess.connectionParameters.GetIdleConnectionStateLifetime()
		}
		return p.sess.config.HandshakeTimeout
	}
	return time.Second
}
// 每当session拿到一个接收到的packet的时候，都会根据其pathID发送到对应的path
func (p *path) handlePacketImpl(pkt *receivedPacket) error {
	if !p.open.Get() {//查询该路径是否开启了
		// Path is closed, ignore packet
		return nil
	}

	if !pkt.rcvTime.IsZero() {
		p.lastNetworkActivityTime = pkt.rcvTime//更新该路径上的最新接收到报文的时间
	}

	hdr := pkt.publicHeader
	data := pkt.data

	// We just received a new packet on that path, so it works
	p.potentiallyFailed.Set(false)

	// Calculate packet number
	hdr.PacketNumber = protocol.InferPacketNumber(
		hdr.PacketNumberLen,
		p.largestRcvdPacketNumber,
		hdr.PacketNumber,
	)

	packet, err := p.sess.unpacker.Unpack(hdr.Raw, hdr, data)//传入了header的原始数据、header结构体和报文的payload数据。得到了解密过后的报文
	if utils.Debug() {
		if err != nil {
			utils.Debugf("<- Reading packet 0x%x (%d bytes) for connection %x on path %x", hdr.PacketNumber, len(data)+len(hdr.Raw), hdr.ConnectionID, p.pathID)
		} else {
			utils.Debugf("<- Reading packet 0x%x (%d bytes) for connection %x on path %x, %s", hdr.PacketNumber, len(data)+len(hdr.Raw), hdr.ConnectionID, p.pathID, packet.encryptionLevel)
		}
	}

	// if the decryption failed, this might be a packet sent by an attacker
	// don't update the remote address
	if quicErr, ok := err.(*qerr.QuicError); ok && quicErr.ErrorCode == qerr.DecryptionFailure {
		return err
	}
	if p.sess.perspective == protocol.PerspectiveServer {
		// update the remote address, even if unpacking failed for any other reason than a decryption error
		p.conn.SetCurrentRemoteAddr(pkt.remoteAddr)
	}
	if err != nil {
		return err
	}

	p.lastRcvdPacketNumber = hdr.PacketNumber//该路径上上一次收到的报文序号
	// Only do this after decrypting, so we are sure the packet is not attacker-controlled
	// 这条路径上收到的最大报文序号
	p.largestRcvdPacketNumber = utils.MaxPacketNumber(p.largestRcvdPacketNumber, hdr.PacketNumber)

	isRetransmittable := ackhandler.HasRetransmittableFrames(packet.frames)//如果存在streamFrame的话，那就是可重传的报文
	if err = p.receivedPacketHandler.ReceivedPacket(hdr.PacketNumber, isRetransmittable); err != nil {// isRetransmittable代表了是否需要重传，是否需要ack
		return err
	}

	if err != nil {
		return err
	}

	return p.sess.handleFrames(packet.frames, p)
}
//如果出现超时的话，那么会
func (p *path) onRTO(lastSentTime time.Time) bool {
	// Was there any activity since last sent packet?
	if p.lastNetworkActivityTime.Before(lastSentTime) {
		p.potentiallyFailed.Set(true) // 如果出现超时的话，那么这条路径就可能down掉了，因此会被设置为true
		p.sess.schedulePathsFrame() // 一旦一个路径超时的话，那么就会发送一些path frame
		return true
	}
	return false
}

func (p *path) SetLeastUnacked(leastUnacked protocol.PacketNumber) {
	p.leastUnacked = leastUnacked
}
