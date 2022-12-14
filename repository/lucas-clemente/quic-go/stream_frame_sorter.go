package quic

import (
	"errors"
	"fmt"
	"github.com/yyleeshine/mpquic/repository/lucas-clemente/quic-go/internal/protocol"
	"github.com/yyleeshine/mpquic/repository/lucas-clemente/quic-go/internal/utils"
	"github.com/yyleeshine/mpquic/repository/lucas-clemente/quic-go/internal/wire"
	"log"
	"os"
	"time"
)

type streamFrameSorter struct {
	queuedFrames     map[protocol.ByteCount]*wire.StreamFrame
	maxOffset        protocol.ByteCount
	queuedTime       map[protocol.ByteCount]time.Time
	sortedOffset     []protocol.ByteCount
	cnt              protocol.ByteCount
	cntZero          protocol.ByteCount
	readPosition     protocol.ByteCount
	gaps             *utils.ByteIntervalList
	sess             *session
	SID              protocol.StreamID
	unreliableMarker bool
}

var (
	errTooManyGapsInReceivedStreamData = errors.New("Too many gaps in received StreamFrame data")
	errDuplicateStreamData             = errors.New("Duplicate Stream Data")
	errEmptyStreamData                 = errors.New("Stream Data empty")
)

func newStreamFrameSorter() *streamFrameSorter {
	s := streamFrameSorter{
		gaps:         utils.NewByteIntervalList(),
		queuedTime:   make(map[protocol.ByteCount]time.Time, 10),
		sortedOffset: make([]protocol.ByteCount, 0, 10),
		maxOffset:    0,
		queuedFrames: make(map[protocol.ByteCount]*wire.StreamFrame),
	}
	logFile, err := os.OpenFile("./timeStamp.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		fmt.Println("open log file failed, err:", err)
	}
	log.SetOutput(logFile)
	log.SetFlags(log.Llongfile | log.Lmicroseconds | log.Ldate)
	s.gaps.PushFront(utils.ByteInterval{Start: 0, End: protocol.MaxByteCount})
	return &s
}

func (s *streamFrameSorter) Push(frame *wire.StreamFrame, flag bool) error {
	timeStamp := time.Now()

	/*if s.unreliableMarker && s.cnt2>70000{
		fmt.Println("received frames:",s.cnt2)
	}*/
	//fmt.Println("Queue:", s.unreliableMarker,len(s.queuedFrames))
	if frame.DataLen() == 0 {
		if frame.FinBit {
			s.queuedFrames[frame.Offset] = frame
			return nil
		}
		return errEmptyStreamData
	}

	var wasCut bool
	if oldFrame, ok := s.queuedFrames[frame.Offset]; ok { //如果已经存在一个老的frame，offset和当前frame的offset相同
		if frame.DataLen() <= oldFrame.DataLen() {
			return errDuplicateStreamData
		}
		frame.Data = frame.Data[oldFrame.DataLen():]
		frame.Offset += oldFrame.DataLen()
		wasCut = true
	}

	start := frame.Offset
	end := frame.Offset + frame.DataLen()

	// skip all gaps that are before this stream frame
	var gap *utils.ByteIntervalElement
	for gap = s.gaps.Front(); gap != nil; gap = gap.Next() {
		// the frame is a duplicate. Ignore it
		if end <= gap.Value.Start {
			log.Println(end)
			return errDuplicateStreamData
		}
		if end > gap.Value.Start && start <= gap.Value.End { //证明存在交叉
			break
		}
	}

	if gap == nil {
		return errors.New("StreamFrameSorter BUG: no gap found")
	}

	if start < gap.Value.Start { //左长，和该gap左对齐，情况一和情况三版本
		add := gap.Value.Start - start
		frame.Offset += add
		start += add
		frame.Data = frame.Data[add:]
		wasCut = true
	}

	// find the highest gaps whose Start lies before the end of the frame
	endGap := gap
	for end >= endGap.Value.End { //情况二和情况三的场景
		nextEndGap := endGap.Next()
		if nextEndGap == nil {
			return errors.New("StreamFrameSorter BUG: no end gap found")
		}
		if endGap != gap {
			s.gaps.Remove(endGap)
		}
		if end <= nextEndGap.Value.Start { //落在一个endgap之外
			break
		}
		// delete queued frames completely covered by the current frame
		delete(s.queuedFrames, endGap.Value.End)
		endGap = nextEndGap
	}

	if end > endGap.Value.End { // 这个判断语句是因为break的原因，落在一个endgap之外，因此最后一个endgap直接被删除了
		cutLen := end - endGap.Value.End
		len := frame.DataLen() - cutLen
		end -= cutLen
		frame.Data = frame.Data[:len]
		wasCut = true
	}

	if start == gap.Value.Start { //此时证明已经左对齐过了，情况一和三
		if end >= gap.Value.End { //情况三
			// the frame completely fills this gap
			// delete the gap
			s.gaps.Remove(gap) //直接将该gap移除掉就可以了
		}
		if end < endGap.Value.End { //情况一和三
			// the frame covers the beginning of the gap
			// adjust the Start value to shrink the gap
			endGap.Value.Start = end //此时需要调整gap
		}
	} else if end == endGap.Value.End { //看97行，情况二
		// the frame covers the end of the gap
		// adjust the End value to shrink the gap
		gap.Value.End = start
	} else { //情况四
		if gap == endGap {
			// the frame lies within the current gap, splitting it into two
			// insert a new gap and adjust the current one
			intv := utils.ByteInterval{Start: end, End: gap.Value.End}
			s.gaps.InsertAfter(intv, gap) //插入新的gap
			gap.Value.End = start         //调整以前的gap
		} else {
			gap.Value.End = start
			endGap.Value.Start = end
		}
	}

	if s.gaps.Len() > protocol.MaxStreamFrameSorterGaps {
		return errTooManyGapsInReceivedStreamData
	}

	if wasCut { //调整缓冲区代销
		data := make([]byte, frame.DataLen())
		copy(data, frame.Data)
		frame.Data = data
	}

	s.queuedFrames[frame.Offset] = frame
	s.cnt += frame.DataLen()
	if flag && frame.Offset >= s.maxOffset {
		s.maxOffset = frame.Offset
		s.sortedOffset = append(s.sortedOffset, s.maxOffset) //排序的
		s.queuedTime[s.maxOffset] = timeStamp
	}
	return nil
}

func (s *streamFrameSorter) Pop() *wire.StreamFrame {
	frame := s.Head()
	if frame != nil {
		s.readPosition += frame.DataLen() // 下一个读取的位置
		s.cnt -= frame.DataLen()
		delete(s.queuedFrames, frame.Offset)

		if len(s.sortedOffset) >= 1 && s.sortedOffset[0] == frame.Offset {
			delete(s.queuedTime, s.sortedOffset[0])
			s.sortedOffset = append(s.sortedOffset[:0], s.sortedOffset[1:]...)
		}
	}
	return frame
}

func (s *streamFrameSorter) Head() *wire.StreamFrame {
	frame, ok := s.queuedFrames[s.readPosition]
	if ok { //如果存在的话
		return frame
	} else if s.unreliableMarker { //证明是不可靠流,且在索要数据的时候，没有该数据，那么在读取的时候就要填入零了
		left := 0
		right := len(s.sortedOffset)
		var val time.Duration
		if right >= 2 {
			val = s.queuedTime[s.sortedOffset[right-1]].Sub(s.queuedTime[s.sortedOffset[left]])
		} else {
			return nil
		}
		lenTime := time.Millisecond * 70
		if val <= lenTime {
			log.Println("bytes:", s.cnt, len(s.queuedFrames))

			return nil
		}

		elem := s.gaps.Front() //拿到第一个gap
		var res *wire.StreamFrame
		var dataPadding []byte //填充的空白数据
		/*if elem.Value.Start-elem.Value.End+1<100{
			dataPadding = make([]byte,elem.Value.Start-elem.Value.End+1)
			res = &wire.StreamFrame{Offset: elem.Value.Start,Data: dataPadding}
			s.Push(res)
		}else {*/
		dataPadding = make([]byte, -elem.Value.Start+elem.Value.End+1, -elem.Value.Start+elem.Value.End+1)
		s.cntZero += -elem.Value.Start + elem.Value.End + 1
		log.Println("zero filled:", s.cntZero)
		res = &wire.StreamFrame{Offset: elem.Value.Start, Data: dataPadding}
		s.Push(res, false)
		//}

		return s.queuedFrames[s.readPosition]
	}
	return nil
}
