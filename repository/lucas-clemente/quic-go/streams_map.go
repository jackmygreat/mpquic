package quic

import (
	"errors"
	"fmt"
	"sync"

	"github.com/yyleeshine/mpquic/repository/lucas-clemente/quic-go/internal/handshake"
	"github.com/yyleeshine/mpquic/repository/lucas-clemente/quic-go/internal/protocol"
	"github.com/yyleeshine/mpquic/repository/lucas-clemente/quic-go/qerr"
)

type streamsMap struct {
	mutex sync.RWMutex

	perspective          protocol.Perspective
	connectionParameters handshake.ConnectionParametersManager

	streams map[protocol.StreamID]*stream
	// needed for round-robin scheduling
	openStreams          []protocol.StreamID
	roundRobinIndex      uint32
	unreliableRobinIndex uint32
	// a table that marks if a stream is unreliable or not
	unreliableStreamMark map[protocol.StreamID]bool

	nextStream                protocol.StreamID // StreamID of the next Stream that will be returned by OpenStream()
	highestStreamOpenedByPeer protocol.StreamID
	nextStreamOrErrCond       sync.Cond
	openStreamOrErrCond       sync.Cond

	closeErr           error
	nextStreamToAccept protocol.StreamID

	newStream newStreamLambda

	numOutgoingStreams uint32
	numIncomingStreams uint32
}

type streamLambda func(*stream) (bool, error)
type newStreamLambda func(protocol.StreamID) *stream

var (
	errMapAccess = errors.New("streamsMap: Error accessing the streams map")
)

func newStreamsMap(newStream newStreamLambda, pers protocol.Perspective, connectionParameters handshake.ConnectionParametersManager) *streamsMap {
	sm := streamsMap{
		perspective:          pers,
		streams:              map[protocol.StreamID]*stream{},
		unreliableStreamMark: map[protocol.StreamID]bool{},
		openStreams:          make([]protocol.StreamID, 0),
		newStream:            newStream,
		connectionParameters: connectionParameters,
	}
	sm.nextStreamOrErrCond.L = &sm.mutex
	sm.openStreamOrErrCond.L = &sm.mutex

	if pers == protocol.PerspectiveClient {
		sm.nextStream = 1
		sm.nextStreamToAccept = 2
	} else {
		sm.nextStream = 2
		sm.nextStreamToAccept = 1
	}

	return &sm
}

// request this stream is reliable or unreliable
func (m *streamsMap) GetStreasmType(id protocol.StreamID) (bool, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	if _, ok := m.streams[id]; !ok {
		return false, fmt.Errorf("a stream with ID %d already exists", id)
	}
	if val, ok := m.unreliableStreamMark[id]; ok {
		return val, nil
	} else {
		return false, nil
	}
}

// GetOrOpenStream either returns an existing stream, a newly opened stream, or nil if a stream with the provided ID is already closed.
// Newly opened streams should only originate from the client. To open a stream from the server, OpenStream should be used.
// 是为了打开远端不属于自己发起的流
func (m *streamsMap) GetOrOpenStream(id protocol.StreamID) (*stream, error) {
	m.mutex.RLock()
	s, ok := m.streams[id]
	m.mutex.RUnlock()
	if ok {
		return s, nil // s may be nil
	}

	// ... we don't have an existing stream
	m.mutex.Lock()
	defer m.mutex.Unlock()
	// We need to check whether another invocation has already created a stream (between RUnlock() and Lock()).
	s, ok = m.streams[id]
	if ok {
		return s, nil
	}
	// 下面所有的情况说明了该流不存在

	//下面的两个大的判断语句是在将那些自己侧发起的流和已经结束的流给排除掉
	if m.perspective == protocol.PerspectiveServer {
		if id%2 == 0 {
			if id <= m.nextStream { // this is a server-side stream that we already opened. Must have been closed already
				return nil, nil
			}
			return nil, qerr.Error(qerr.InvalidStreamID, fmt.Sprintf("attempted to open stream %d from client-side", id))
		}
		if id <= m.highestStreamOpenedByPeer { // this is a client-side stream that doesn't exist anymore. Must have been closed already
			return nil, nil
		}
	}
	if m.perspective == protocol.PerspectiveClient {
		if id%2 == 1 {
			if id <= m.nextStream { // this is a client-side stream that we already opened.
				return nil, nil
			}
			return nil, qerr.Error(qerr.InvalidStreamID, fmt.Sprintf("attempted to open stream %d from server-side", id))
		}
		if id <= m.highestStreamOpenedByPeer { // this is a server-side stream that doesn't exist anymore. Must have been closed already
			return nil, nil
		}
	}

	// 走到下面的代码后，说明了所需要的流还没有打开，并且不是自己侧发起的流
	// 还没有流打开的话，highestStreamOpenedByPeer一开始等于0，服务器的流计数从2开始，客户端的流计数从1开始
	// sid is the next stream that will be opened
	sid := m.highestStreamOpenedByPeer + 2
	// if there is no stream opened yet, and this is the server, stream 1 should be openend
	if sid == 2 && m.perspective == protocol.PerspectiveServer {
		sid = 1
	}

	for ; sid <= id; sid += 2 {
		_, err := m.openRemoteStream(sid)
		if err != nil {
			return nil, err
		}
	}

	m.nextStreamOrErrCond.Broadcast()
	return m.streams[id], nil
}

// GetOrOpenStream either returns an existing stream, a newly opened stream, or nil if a stream with the provided ID is already closed.
// Newly opened streams should only originate from the client. To open a stream from the server, OpenStream should be used.
// 是为了打开远端不属于自己发起的流
func (m *streamsMap) GetOrOpenStreamType(id protocol.StreamID, marker bool) (*stream, error) {
	m.mutex.RLock()
	s, ok := m.streams[id]
	m.mutex.RUnlock()
	if ok {
		return s, nil // s may be nil
	}

	// ... we don't have an existing stream
	m.mutex.Lock()
	defer m.mutex.Unlock()
	// We need to check whether another invocation has already created a stream (between RUnlock() and Lock()).
	s, ok = m.streams[id]
	if ok {
		return s, nil
	}
	// 下面所有的情况说明了该流不存在

	//下面的两个大的判断语句是在将那些自己侧发起的流和已经结束的流给排除掉
	if m.perspective == protocol.PerspectiveServer {
		if id%2 == 0 {
			if id <= m.nextStream { // this is a server-side stream that we already opened. Must have been closed already
				return nil, nil
			}
			return nil, qerr.Error(qerr.InvalidStreamID, fmt.Sprintf("attempted to open stream %d from client-side", id))
		}
		if id <= m.highestStreamOpenedByPeer { // this is a client-side stream that doesn't exist anymore. Must have been closed already
			return nil, nil
		}
	}
	if m.perspective == protocol.PerspectiveClient {
		if id%2 == 1 {
			if id <= m.nextStream { // this is a client-side stream that we already opened.
				return nil, nil
			}
			return nil, qerr.Error(qerr.InvalidStreamID, fmt.Sprintf("attempted to open stream %d from server-side", id))
		}
		if id <= m.highestStreamOpenedByPeer { // this is a server-side stream that doesn't exist anymore. Must have been closed already
			return nil, nil
		}
	}

	// 走到下面的代码后，说明了所需要的流还没有打开，并且不是自己侧发起的流
	// 还没有流打开的话，highestStreamOpenedByPeer一开始等于0，服务器的流计数从2开始，客户端的流计数从1开始
	// sid is the next stream that will be opened
	sid := m.highestStreamOpenedByPeer + 2
	// if there is no stream opened yet, and this is the server, stream 1 should be openend
	if sid == 2 && m.perspective == protocol.PerspectiveServer {
		sid = 1
	}

	// open a stream initiated by the peer and send the marker
	_, err := m.openRemoteStreamType(sid, marker)
	if err != nil {
		return nil, err
	}

	m.nextStreamOrErrCond.Broadcast()
	return m.streams[id], nil
}

func (m *streamsMap) openRemoteStream(id protocol.StreamID) (*stream, error) {
	if m.numIncomingStreams >= m.connectionParameters.GetMaxIncomingStreams() {
		return nil, qerr.TooManyOpenStreams
	}
	if id+protocol.MaxNewStreamIDDelta < m.highestStreamOpenedByPeer {
		return nil, qerr.Error(qerr.InvalidStreamID, fmt.Sprintf("attempted to open stream %d, which is a lot smaller than the highest opened stream, %d", id, m.highestStreamOpenedByPeer))
	}

	if m.perspective == protocol.PerspectiveServer {
		m.numIncomingStreams++
	} else {
		m.numOutgoingStreams++
	}

	if id > m.highestStreamOpenedByPeer {
		m.highestStreamOpenedByPeer = id
	}

	s := m.newStream(id)
	m.putStream(s)
	return s, nil
}
func (m *streamsMap) openRemoteStreamType(id protocol.StreamID, marker bool) (*stream, error) {
	if m.numIncomingStreams >= m.connectionParameters.GetMaxIncomingStreams() {
		return nil, qerr.TooManyOpenStreams
	}
	if id+protocol.MaxNewStreamIDDelta < m.highestStreamOpenedByPeer {
		return nil, qerr.Error(qerr.InvalidStreamID, fmt.Sprintf("attempted to open stream %d, which is a lot smaller than the highest opened stream, %d", id, m.highestStreamOpenedByPeer))
	}

	if m.perspective == protocol.PerspectiveServer {
		m.numIncomingStreams++
	} else {
		m.numOutgoingStreams++
	}

	if id > m.highestStreamOpenedByPeer {
		m.highestStreamOpenedByPeer = id
	}

	s := m.newStream(id)
	m.putStreamType(s, marker)
	return s, nil
}
func (m *streamsMap) openStreamImpl() (*stream, error) {
	id := m.nextStream
	if m.numOutgoingStreams >= m.connectionParameters.GetMaxOutgoingStreams() {
		return nil, qerr.TooManyOpenStreams
	}

	if m.perspective == protocol.PerspectiveServer {
		m.numOutgoingStreams++
	} else {
		m.numIncomingStreams++
	}

	m.nextStream += 2
	s := m.newStream(id)
	m.putStreamType(s, false)
	return s, nil
}

func (m *streamsMap) openUnreliableStreamImpl() (*stream, error) {
	id := m.nextStream
	if m.numOutgoingStreams >= m.connectionParameters.GetMaxOutgoingStreams() {
		return nil, qerr.TooManyOpenStreams
	}

	if m.perspective == protocol.PerspectiveServer {
		m.numOutgoingStreams++
	} else {
		m.numIncomingStreams++
	}

	m.nextStream += 2
	s := m.newStream(id) //调用session当中的newStream方法
	m.putStreamType(s, true)
	return s, nil
}

// OpenStream opens the next available stream
func (m *streamsMap) OpenStream() (*stream, error) { //发起新的stream
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.closeErr != nil {
		return nil, m.closeErr
	}
	return m.openStreamImpl()
}

// mark: open an unreliable Stream
func (m *streamsMap) OpenUnreliableStream() (*stream, error) { //发起新的不可靠传输的stream
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.closeErr != nil {
		return nil, m.closeErr
	}
	return m.openUnreliableStreamImpl()
}

func (m *streamsMap) OpenStreamSync() (*stream, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	for {
		if m.closeErr != nil {
			return nil, m.closeErr
		}
		str, err := m.openStreamImpl()
		if err == nil {
			return str, err
		}
		if err != nil && err != qerr.TooManyOpenStreams {
			return nil, err
		}
		m.openStreamOrErrCond.Wait()
	}
}

// AcceptStream returns the next stream opened by the peer
// it blocks until a new stream is opened
func (m *streamsMap) AcceptStream() (*stream, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	var str *stream
	for {
		var ok bool
		if m.closeErr != nil {
			return nil, m.closeErr
		}
		str, ok = m.streams[m.nextStreamToAccept]
		if ok {
			break
		}
		m.nextStreamOrErrCond.Wait()
	}
	m.nextStreamToAccept += 2
	return str, nil
}

func (m *streamsMap) Iterate(fn streamLambda) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	openStreams := append([]protocol.StreamID{}, m.openStreams...)

	for _, streamID := range openStreams {
		cont, err := m.iterateFunc(streamID, fn)
		if err != nil {
			return err
		}
		if !cont {
			break
		}
	}
	return nil
}

// RoundRobinIterate executes the streamLambda for every open stream, until the streamLambda returns false
// It uses a round-robin-like scheduling to ensure that every stream is considered fairly
// It prioritizes the crypto- and the header-stream (StreamIDs 1 and 3)
// 优先轮询可靠流
func (m *streamsMap) RoundRobinIterate(fn streamLambda) error { //这里是否需要优先轮询可靠流？
	m.mutex.Lock()
	defer m.mutex.Unlock()

	numStreams := uint32(len(m.streams))
	startIndex := m.roundRobinIndex
	startIndexUnreliable := m.unreliableRobinIndex
	for _, i := range []protocol.StreamID{1, 3} { //优先轮询1和3流
		cont, err := m.iterateFunc(i, fn)
		if err != nil && err != errMapAccess {
			return err
		}
		if !cont {
			return nil
		}
	}
	for i := uint32(0); i < numStreams; i++ {
		streamID := m.openStreams[(i+startIndexUnreliable)%numStreams]
		if streamID == 1 || streamID == 3 {
			continue
		}
		if _, ok := m.unreliableStreamMark[streamID]; ok { // 如果是不可靠的，那就跳过本轮循环
			continue
		}
		cont, err := m.iterateFunc(streamID, fn)
		if err != nil {
			return err
		}
		m.unreliableRobinIndex = (m.unreliableRobinIndex + 1) % numStreams
		if !cont { //是否需要跳出循环？，如果已经获取了所需呀的数据量，那么就跳出循环
			break
		}
	}
	for i := uint32(0); i < numStreams; i++ {
		streamID := m.openStreams[(i+startIndex)%numStreams]
		if streamID == 1 || streamID == 3 {
			continue
		}
		if _, ok := m.unreliableStreamMark[streamID]; !ok { // 如果是可靠的，那就跳过本轮循环
			continue
		}
		cont, err := m.iterateFunc(streamID, fn)
		if err != nil {
			return err
		}
		m.roundRobinIndex = (m.roundRobinIndex + 1) % numStreams
		if !cont {
			break
		}
	}
	return nil
}
func (m *streamsMap) RoundRobinIterate2(fn streamLambda, path *path) error { //这里是否需要优先轮询可靠流？
	m.mutex.Lock()
	defer m.mutex.Unlock()

	numStreams := uint32(len(m.streams))
	startIndex := m.roundRobinIndex
	startIndexUnreliable := m.unreliableRobinIndex
	for _, i := range []protocol.StreamID{1, 3} { //优先轮询1和3流
		cont, err := m.iterateFunc(i, fn)
		if err != nil && err != errMapAccess {
			return err
		}
		if !cont {
			return nil
		}
	}
	if path.sentPacketHandler.SendingAllowed() { //如果该路径允许发送的话，证明是正常的途径到达的,那么先要轮询可靠的再轮询不可靠的
		for i := uint32(0); i < numStreams; i++ {
			streamID := m.openStreams[(i+startIndexUnreliable)%numStreams]
			if streamID == 1 || streamID == 3 {
				continue
			}
			if _, ok := m.unreliableStreamMark[streamID]; ok { // 如果是不可靠的，那就跳过本轮循环
				continue
			}
			cont, err := m.iterateFunc(streamID, fn)
			if err != nil {
				return err
			}
			m.unreliableRobinIndex = (m.unreliableRobinIndex + 1) % numStreams
			if !cont { //是否需要跳出循环？，如果已经获取了所需呀的数据量，那么就跳出循环
				break
			}
		}
	}
	for i := uint32(0); i < numStreams; i++ {
		streamID := m.openStreams[(i+startIndex)%numStreams]
		if streamID == 1 || streamID == 3 {
			continue
		}
		if _, ok := m.unreliableStreamMark[streamID]; !ok { // 如果是可靠的，那就跳过本轮循环
			continue
		}
		cont, err := m.iterateFunc(streamID, fn)
		if err != nil {
			return err
		}
		m.roundRobinIndex = (m.roundRobinIndex + 1) % numStreams
		if !cont {
			break
		}
	}
	return nil
}
func (m *streamsMap) iterateFunc(streamID protocol.StreamID, fn streamLambda) (bool, error) {
	str, ok := m.streams[streamID]
	if !ok {
		return true, errMapAccess
	}
	return fn(str)
}

func (m *streamsMap) putStream(s *stream) error {
	id := s.StreamID()
	if _, ok := m.streams[id]; ok {
		return fmt.Errorf("a stream with ID %d already exists", id)
	}

	m.streams[id] = s
	m.openStreams = append(m.openStreams, id)

	return nil
}

// marker stands for the stream type: reliable stream(false) unrelaible stream(true)
func (m *streamsMap) putStreamType(s *stream, marker bool) error {
	id := s.StreamID()
	if _, ok := m.streams[id]; ok {
		return fmt.Errorf("a stream with ID %d already exists", id)
	}

	m.streams[id] = s
	m.openStreams = append(m.openStreams, id)
	m.unreliableStreamMark[id] = marker
	return nil
}

// Attention: this function must only be called if a mutex has been acquired previously
func (m *streamsMap) RemoveStream(id protocol.StreamID) error {
	s, ok := m.streams[id]
	if !ok || s == nil {
		return fmt.Errorf("attempted to remove non-existing stream: %d", id)
	}

	if id%2 == 0 {
		m.numOutgoingStreams--
	} else {
		m.numIncomingStreams--
	}

	for i, s := range m.openStreams {
		if s == id {
			// delete the streamID from the openStreams slice
			m.openStreams = m.openStreams[:i+copy(m.openStreams[i:], m.openStreams[i+1:])]
			// adjust round-robin index, if necessary
			if uint32(i) < m.roundRobinIndex {
				m.roundRobinIndex--
			}
			break
		}
	}

	delete(m.streams, id)
	m.openStreamOrErrCond.Signal()
	return nil
}

func (m *streamsMap) CloseWithError(err error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.closeErr = err
	m.nextStreamOrErrCond.Broadcast()
	m.openStreamOrErrCond.Broadcast()
	for _, s := range m.openStreams {
		m.streams[s].Cancel(err)
	}
}
