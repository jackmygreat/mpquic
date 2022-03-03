package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"flag"
	"fmt"
	"github.com/q191201771/lal/pkg/httpflv"
	"github.com/q191201771/lal/pkg/remux"
	"github.com/q191201771/naza/pkg/nazalog"
	quic "github.com/yyleeshine/mpquic/repository/lucas-clemente/quic-go"
	"math/big"
	"os"
	"time"
)

func HandleError(err error) {
	if err != nil {
		fmt.Println("Error: ", err)
		os.Exit(1)
	}
}

var FilePath string

func ParseCommandLine() {
	flag.StringVar(&FilePath, "f", " ", "-f file.flv")
	flag.Parse()
}

//a sender function that generates frames and sends them over mpquic to the reciever.
//input args - deviceID and mpquic-server address

func main() {
	ParseCommandLine()
	if len(FilePath) <= 1 {
		fmt.Println("./flvparse -f filename.flv")
		return
	}
	fmt.Println(len(FilePath))
	//r, err := os.Open(GConfFile)
	//if err != nil {
	//	fmt.Println("err:",err)
	//}
	//fdm:= flv.NewDemuxer(r)
	//err = fdm.FlvParse()
	f, err := os.OpenFile("./clientlog.txt", os.O_CREATE|os.O_RDWR|os.O_APPEND, 0666)
	defer f.Close()
	if err != nil {
		panic(err)
	}

	quicServerAddr := "127.0.0.1:5252"
	pushflv(quicServerAddr, FilePath)
	HandleError(err)

}
func pushflv(url, filename string) {
	//mpquic config
	quicConfig := &quic.Config{
		CreatePaths: true, //要求创建多路径
	}
	listener, err := quic.ListenAddr(url, generateTLSConfig(), quicConfig)
	HandleError(err)
	sess, err := listener.Accept() //accepting a session from sender
	HandleError(err)
	tags, err := httpflv.ReadAllTagsFromFLVFile(filename)
	HandleError(err)

	controlstream, err := sess.OpenStream()
	HandleError(err)
	defer controlstream.Close()

	keystream, err := sess.OpenStream()
	defer keystream.Close()
	HandleError(err)

	pbstream, err := sess.OpenStream()
	HandleError(err)
	defer pbstream.Close()

	if err != nil || len(tags) == 0 {
		nazalog.Fatalf("read tags from flv file failed. err=%+v", err)
	}
	nazalog.Infof("read tags from flv file succ. len of tags=%d", len(tags))
	now := time.Now()
	loopPush(tags, pbstream, controlstream, keystream)
	fmt.Println(time.Since(now))
	time.Sleep(time.Second * 2)
}
func loopPush(tags []httpflv.Tag, pbstream quic.Stream, controlstream quic.Stream, keystream quic.Stream) {
	var (
		totalBaseTS        uint32 // 每轮最后更新
		prevTS             uint32 // 上一个tag
		hasReadThisBaseTS  bool
		thisBaseTS         uint32 // 每轮第一个tag
		hasTraceFirstTagTS bool
		firstTagTS         uint32 // 所有轮第一个tag
		firstTagTick       int64  // 所有轮第一个tag的物理发送时间
	)

	// 1. 保证metadata只在最初发送一次
	// 2. 多轮，时间戳会翻转，需要处理，让它线性增长

	// 多轮，一个循环代表一次完整文件的发送
	for {
		hasReadThisBaseTS = false

		// 一轮，遍历文件的所有tag数据
		for _, tag := range tags {
			h := remux.FLVTagHeader2RTMPHeader(tag.Header)

			//-------------------------------------------------
			// metadata只发送一次,这一部分不要更改
			if tag.IsMetadata() {
				if totalBaseTS == 0 {
					h.TimestampAbs = 0

					controlInfo := make([]byte, 11+4)                   //控制信息
					_ = copy(controlInfo[11:15], tag.Raw[h.MsgLen+11:]) // 控制信息+previousTag（4字节）
					_ = copy(controlInfo[0:11], tag.Raw[0:11])
					if _, err := controlstream.Write(controlInfo); err != nil {
						nazalog.Errorf("write data error. err=%v", err)
						return
					}

					if _, err := controlstream.Write(tag.Raw[11 : 11+h.MsgLen]); err != nil { //传输数据
						nazalog.Errorf("write data error. err=%v", err)
						return
					}
					//fmt.Println(controlInfo,tag.Raw[11:11+h.MsgLen])
				}
				continue
			}
			//-----------------------------------------------------

			//------------------------------------------------------
			if hasReadThisBaseTS {
				// 本轮非第一个tag

				// 之前已经读到了这轮读文件的base值，ts要减去base
				h.TimestampAbs = tag.Header.Timestamp - thisBaseTS + totalBaseTS
			} else {
				// 本轮第一个tag

				// 设置base，ts设置为上一轮读文件的值
				thisBaseTS = tag.Header.Timestamp
				h.TimestampAbs = totalBaseTS
				hasReadThisBaseTS = true
			}

			if h.TimestampAbs < prevTS {
				// ts比上一个包的还小，直接设置为上一包的值，并且不sleep直接发送
				h.TimestampAbs = prevTS
				nazalog.Errorf("this tag timestamp less than prev timestamp. h.TimestampAbs=%d, prevTS=%d", h.TimestampAbs, prevTS)
			}

			//chunks := rtmp.Message2Chunks(tag.Raw[11:11+h.MsgLen], &h)

			if hasTraceFirstTagTS {
				// 所有轮的非第一个tag

				// 当前距离第一个tag的物理发送时间，以及距离第一个tag的时间戳
				// 如果物理时间短，就睡眠相应的时间
				n := time.Now().UnixNano() / 1000000
				diffTick := n - firstTagTick
				diffTS := h.TimestampAbs - firstTagTS
				if diffTick < int64(diffTS) {
					//time.Sleep(time.Duration(int64(diffTS)-diffTick) * time.Millisecond)
				}
			} else {
				// 所有轮的第一个tag

				// 记录所有轮的第一个tag的物理发送时间，以及数据的时间戳
				firstTagTick = time.Now().UnixNano() / 1000000
				firstTagTS = h.TimestampAbs
				hasTraceFirstTagTS = true
			}
			//---------------------------------------------------------

			controlInfo := make([]byte, 20)                     //控制信息
			_ = copy(controlInfo[16:20], tag.Raw[h.MsgLen+11:]) // 控制信息+previousTag（4字节）
			_ = copy(controlInfo[0:16], tag.Raw[0:16])
			if _, err := controlstream.Write(controlInfo); err != nil {
				nazalog.Errorf("write data error. err=%v", err)
				return
			}
			if tag.Header.Type == httpflv.TagTypeVideo && tag.Raw[httpflv.TagHeaderSize] == httpflv.AVCKeyFrame {
				// keyFrame，使用可靠流传输
				if _, err := keystream.Write(tag.Raw[16 : 11+h.MsgLen]); err != nil { //传输数据
					nazalog.Errorf("write data error. err=%v", err)
					return
				}
			} else {
				//非keyFrame，使用非可靠流传输
				if _, err := pbstream.Write(tag.Raw[16 : 11+h.MsgLen]); err != nil { //传输数据
					nazalog.Errorf("write data error. err=%v", err)
					return
				}
			}

			//fmt.Println(controlInfo,tag.Raw[11:11+h.MsgLen])

			prevTS = h.TimestampAbs
		} // tags for loop

		totalBaseTS = prevTS + 1
		controlInfo := make([]byte, 11+4)
		copy(controlInfo, []byte("fin"))
		controlstream.Write(controlInfo)
		break
	} // tag 发送完

}

func generateTLSConfig() *tls.Config {
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		panic(err)
	}
	template := x509.Certificate{SerialNumber: big.NewInt(1)}
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		panic(err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		panic(err)
	}

	return &tls.Config{Certificates: []tls.Certificate{tlsCert}}
}
