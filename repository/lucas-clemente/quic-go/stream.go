package quic

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/yyleeshine/mpquic/repository/lucas-clemente/quic-go/internal/flowcontrol"
	"github.com/yyleeshine/mpquic/repository/lucas-clemente/quic-go/internal/protocol"
	"github.com/yyleeshine/mpquic/repository/lucas-clemente/quic-go/internal/utils"
	"github.com/yyleeshine/mpquic/repository/lucas-clemente/quic-go/internal/wire"
)

// A Stream assembles the data from StreamFrames and provides a super-convenient Read-Interface
//
// Read() and Write() may be called concurrently, but multiple calls to Read() or Write() individually must be synchronized manually.
type stream struct {
	mutex sync.Mutex

	ctx       context.Context
	ctxCancel context.CancelFunc

	streamID protocol.StreamID
	onData   func()
	// onReset is a callback that should send a RST_STREAM
	// 这个函数是一个回调函数，目的是当该stream被关闭的时候发送一个RST_STREAM 的Frame
	onReset func(protocol.StreamID, protocol.ByteCount)

	readPosInFrame int
	writeOffset    protocol.ByteCount // 写入的offset
	readOffset     protocol.ByteCount// 读取的offset

	// Once set, the errors must not be changed!
	err error

	// cancelled is set when Cancel() is called
	cancelled utils.AtomicBool
	// finishedReading is set once we read a frame with a FinBit
	finishedReading utils.AtomicBool
	// finisedWriting is set once Close() is called
	finishedWriting utils.AtomicBool
	// resetLocally is set if Reset() is called
	resetLocally utils.AtomicBool
	// resetRemotely is set if RegisterRemoteError() is called
	resetRemotely utils.AtomicBool

	frameQueue   *streamFrameSorter
	readChan     chan struct{}
	readDeadline time.Time

	dataForWriting []byte
	finSent        utils.AtomicBool
	rstSent        utils.AtomicBool
	writeChan      chan struct{}
	writeDeadline  time.Time
	sess *session
	flowControlManager flowcontrol.FlowControlManager
}

var _ Stream = &stream{}

type deadlineError struct{}

func (deadlineError) Error() string   { return "deadline exceeded" }
func (deadlineError) Temporary() bool { return true }
func (deadlineError) Timeout() bool   { return true }

var errDeadline net.Error = &deadlineError{}

// newStream creates a new Stream
func newStream(StreamID protocol.StreamID,
	onData func(),
	onReset func(protocol.StreamID, protocol.ByteCount),
	flowControlManager flowcontrol.FlowControlManager) *stream {
	s := &stream{
		onData:             onData,
		onReset:            onReset,
		streamID:           StreamID,
		flowControlManager: flowControlManager,
		frameQueue:         newStreamFrameSorter(),
		readChan:           make(chan struct{}, 1),
		writeChan:          make(chan struct{}, 1),
	}
	s.ctx, s.ctxCancel = context.WithCancel(context.Background())
	return s
}
// newStream creates a new Stream
func newStreamType(StreamID protocol.StreamID,
	onData func(),
	onReset func(protocol.StreamID, protocol.ByteCount),
	flowControlManager flowcontrol.FlowControlManager, sess *session) *stream {
	s := &stream{
		onData:             onData,
		onReset:            onReset,
		streamID:           StreamID,
		flowControlManager: flowControlManager,
		frameQueue:         newStreamFrameSorter(),
		readChan:           make(chan struct{}, 1),
		writeChan:          make(chan struct{}, 1),
		sess: sess,
	}
	s.frameQueue.sess = sess
	s.frameQueue.SID = StreamID
	s.frameQueue.unreliableMarker = s.frameQueue.sess.streamsMap.unreliableStreamMark[StreamID]
	s.ctx, s.ctxCancel = context.WithCancel(context.Background())
	return s
}
// Read implements io.Reader. It is not thread safe!
func (s *stream) Read(p []byte) (int, error) {// 读取一定的字节数到[]byte当中
	s.mutex.Lock()
	err := s.err
	s.mutex.Unlock()
	if s.cancelled.Get() || s.resetLocally.Get() {//如果该流被重置了，或者是被关闭了，那么该流就返回零
		return 0, err
	}
	if s.finishedReading.Get() {
		return 0, io.EOF
	}

	bytesRead := 0
	for bytesRead < len(p) {//向字节数组当中填写数据，直到其被填满
		s.mutex.Lock()
		frame := s.frameQueue.Head()//从frameQueue.Head()拿数据
		if frame == nil && bytesRead > 0 {//如果暂时没有可用的帧的话，且已经读取到了部分的数据
			err = s.err
			s.mutex.Unlock()
			return bytesRead, err
		}

		var err error
		for {
			// Stop waiting on errors
			if s.resetLocally.Get() || s.cancelled.Get() {
				err = s.err
				break
			}

			deadline := s.readDeadline
			if !deadline.IsZero() && !time.Now().Before(deadline) {//如果已经超时的话
				err = errDeadline//会返回超时错误
				break
			}

			if frame != nil {
				//readOffset代表了现在读取到的偏移量，frame.offset 代表了该帧的偏移量，因此readPosInFrame代表了在该帧内以帧首为0，读取到的偏移量
				s.readPosInFrame = int(s.readOffset - frame.Offset)
				break
			}

			s.mutex.Unlock()
			if deadline.IsZero() {//这说明读取的deadline没有被设置，那就不存在超时的问题，因此就一直等待数据的到来吧！！！
				<-s.readChan//一旦放入了一个frame就会通知,但是就算放入了一个frame，这个frame也可能不在前面，可能造成frame = s.frameQueue.Head()拿到的是nil
			} else {
				select {
				case <-s.readChan:
				case <-time.After(deadline.Sub(time.Now()))://超时的话
				}
			}
			s.mutex.Lock()
			frame = s.frameQueue.Head()//这个函数并不会消耗frameQueue的frame数量
		}
		s.mutex.Unlock()

		if err != nil {
			return bytesRead, err
		}

		m := utils.Min(len(p)-bytesRead, int(frame.DataLen())-s.readPosInFrame)//第一个是缓存还有多大空间要读，第二个是指本帧内还能提供多大的内容供读取

		if bytesRead > len(p) {
			return bytesRead, fmt.Errorf("BUG: bytesRead (%d) > len(p) (%d) in stream.Read", bytesRead, len(p))
		}
		if s.readPosInFrame > int(frame.DataLen()) {
			return bytesRead, fmt.Errorf("BUG: readPosInFrame (%d) > frame.DataLen (%d) in stream.Read", s.readPosInFrame, frame.DataLen())
		}


		copy(p[bytesRead:], frame.Data[s.readPosInFrame:])//复制内容

		s.readPosInFrame += m
		bytesRead += m
		s.readOffset += protocol.ByteCount(m)

		// when a RST_STREAM was received, the was already informed about the final byteOffset for this stream
		if !s.resetRemotely.Get() {
			s.flowControlManager.AddBytesRead(s.streamID, protocol.ByteCount(m))
		}
		s.onData() // so that a possible WINDOW_UPDATE is sent

		if s.readPosInFrame >= int(frame.DataLen()) {//怎么可能大于？？？,应该是等于吧？也就是本帧读完
			fin := frame.FinBit
			s.mutex.Lock()
			s.frameQueue.Pop()//只有该帧内的内容被读取完了才会调用该函数
			s.mutex.Unlock()
			if fin {
				s.finishedReading.Set(true)
				return bytesRead, io.EOF
			}
		}
	}

	return bytesRead, nil
}
// func (s *session) scheduleSending() {
//	select {
//	case s.sendingScheduled <- struct{}{}:
//	default:
//	}
// }
func (s *stream) Write(p []byte) (int, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.resetLocally.Get() || s.err != nil {
		return 0, s.err
	}
	if s.finishedWriting.Get() {
		return 0, fmt.Errorf("write on closed stream %d", s.streamID)
	}
	if len(p) == 0 {
		return 0, nil
	}

	s.dataForWriting = make([]byte, len(p))//将写入的数据放入到缓冲区里面
	copy(s.dataForWriting, p)
	s.onData()//通知session存在数据要发送了

	var err error
	for {
		deadline := s.writeDeadline//一般这个是为0的
		if !deadline.IsZero() && !time.Now().Before(deadline) {//如果deadline不是0，且已经超时的话
			err = errDeadline
			break
		}
		if s.dataForWriting == nil || s.err != nil {//此时证明缓冲区已经被发送完毕了，因此可以退出for循环了
			break
		}

		s.mutex.Unlock()
		if deadline.IsZero() {
			<-s.writeChan//一旦有人从缓冲区中拿数据，都会通过该管道通知的
		} else {
			select {
			case <-s.writeChan:
			case <-time.After(deadline.Sub(time.Now())):
			}
		}
		s.mutex.Lock()
	}

	if err != nil {
		return 0, err
	}
	if s.err != nil {//如果报错的话
		return len(p) - len(s.dataForWriting), s.err//返回发出的报文字节数和错误的原因
	}
	return len(p), nil
}

func (s *stream) lenOfDataForWriting() protocol.ByteCount {
	s.mutex.Lock()
	var l protocol.ByteCount
	if s.err == nil {
		l = protocol.ByteCount(len(s.dataForWriting))
	}
	s.mutex.Unlock()
	return l
}
// 某一个接口会通过这个拿到数据
func (s *stream) getDataForWriting(maxBytes protocol.ByteCount) []byte {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.err != nil || s.dataForWriting == nil {
		return nil
	}

	var ret []byte
	if protocol.ByteCount(len(s.dataForWriting)) > maxBytes {//如果缓冲区当中的数据大于所需要的话，那么久拿到所需要的数据
		ret = s.dataForWriting[:maxBytes]
		s.dataForWriting = s.dataForWriting[maxBytes:]
	} else {
		ret = s.dataForWriting
		s.dataForWriting = nil
		s.signalWrite()
	}
	s.writeOffset += protocol.ByteCount(len(ret))
	return ret
}

// Close implements io.Closer
func (s *stream) Close() error {
	s.finishedWriting.Set(true)
	s.ctxCancel()
	s.onData()
	return nil
}

func (s *stream) shouldSendReset() bool {
	if s.rstSent.Get() {
		return false
	}
	return (s.resetLocally.Get() || s.resetRemotely.Get()) && !s.finishedWriteAndSentFin()
}

func (s *stream) shouldSendFin() bool {
	s.mutex.Lock()
	res := s.finishedWriting.Get() && !s.finSent.Get() && s.err == nil && s.dataForWriting == nil
	s.mutex.Unlock()
	return res
}

func (s *stream) sentFin() {
	s.finSent.Set(true)
}

// AddStreamFrame adds a new stream frame
// 添加一个stream frame
func (s *stream) AddStreamFrame(frame *wire.StreamFrame) error {
	maxOffset := frame.Offset + frame.DataLen()
	err := s.flowControlManager.UpdateHighestReceived(s.streamID, maxOffset)
	if err != nil {
		return err
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()
	err = s.frameQueue.Push(frame)
	if err != nil && err != errDuplicateStreamData {
		return err
	}
	s.signalRead()
	return nil
}

// signalRead performs a non-blocking send on the readChan
func (s *stream) signalRead() {
	select {
	case s.readChan <- struct{}{}:
	default:
	}
}

// signalRead performs a non-blocking send on the writeChan
func (s *stream) signalWrite() {
	select {
	case s.writeChan <- struct{}{}:
	default:
	}
}

func (s *stream) SetReadDeadline(t time.Time) error {
	s.mutex.Lock()
	oldDeadline := s.readDeadline
	s.readDeadline = t
	s.mutex.Unlock()
	// if the new deadline is before the currently set deadline, wake up Read()
	if t.Before(oldDeadline) {
		s.signalRead()
	}
	return nil
}

func (s *stream) SetWriteDeadline(t time.Time) error {
	s.mutex.Lock()
	oldDeadline := s.writeDeadline
	s.writeDeadline = t
	s.mutex.Unlock()
	if t.Before(oldDeadline) {
		s.signalWrite()
	}
	return nil
}

func (s *stream) SetDeadline(t time.Time) error {
	_ = s.SetReadDeadline(t)  // SetReadDeadline never errors
	_ = s.SetWriteDeadline(t) // SetWriteDeadline never errors
	return nil
}

// CloseRemote makes the stream receive a "virtual" FIN stream frame at a given offset
func (s *stream) CloseRemote(offset protocol.ByteCount) {
	if val, ok := s.sess.streamsMap.unreliableStreamMark[s.streamID]; ok {
		s.AddStreamFrame(&wire.StreamFrame{UnreliableMarker:val,FinBit: true, Offset: offset,})
	}else{
		s.AddStreamFrame(&wire.StreamFrame{FinBit: true, Offset: offset})
	}

}

// Cancel is called by session to indicate that an error occurred
// The stream should will be closed immediately
func (s *stream) Cancel(err error) {
	s.mutex.Lock()
	s.cancelled.Set(true)
	s.ctxCancel()
	// errors must not be changed!
	if s.err == nil {
		s.err = err
		s.signalRead()
		s.signalWrite()
	}
	s.mutex.Unlock()
}

// resets the stream locally
func (s *stream) Reset(err error) {
	if s.resetLocally.Get() {
		return
	}
	s.mutex.Lock()
	s.resetLocally.Set(true)
	s.ctxCancel()
	// errors must not be changed!
	if s.err == nil {
		s.err = err
		s.signalRead()
		s.signalWrite()
	}
	if s.shouldSendReset() {
		s.onReset(s.streamID, s.writeOffset)
		s.rstSent.Set(true)
	}
	s.mutex.Unlock()
}

// resets the stream remotely
func (s *stream) RegisterRemoteError(err error) {
	if s.resetRemotely.Get() {
		return
	}
	s.mutex.Lock()
	s.resetRemotely.Set(true)
	s.ctxCancel()
	// errors must not be changed!
	if s.err == nil {
		s.err = err
		s.signalWrite()
	}
	if s.shouldSendReset() {
		s.onReset(s.streamID, s.writeOffset)
		s.rstSent.Set(true)
	}
	s.mutex.Unlock()
}

func (s *stream) finishedWriteAndSentFin() bool {
	return s.finishedWriting.Get() && s.finSent.Get()
}

func (s *stream) finished() bool {
	return s.cancelled.Get() ||
		(s.finishedReading.Get() && s.finishedWriteAndSentFin()) ||
		(s.resetRemotely.Get() && s.rstSent.Get()) ||
		(s.finishedReading.Get() && s.rstSent.Get()) ||
		(s.finishedWriteAndSentFin() && s.resetRemotely.Get())
}

func (s *stream) Context() context.Context {
	return s.ctx
}

func (s *stream) StreamID() protocol.StreamID {
	return s.streamID
}

func (s *stream) GetBytesSent() (protocol.ByteCount, error) {
	return s.flowControlManager.GetBytesSent(s.streamID)
}

func (s *stream) GetBytesRetrans() (protocol.ByteCount, error) {
	return s.flowControlManager.GetBytesRetrans(s.streamID)
}
