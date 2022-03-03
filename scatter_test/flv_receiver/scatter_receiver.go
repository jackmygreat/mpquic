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
	"github.com/q191201771/naza/pkg/bele"
	"github.com/q191201771/naza/pkg/nazalog"
	"github.com/yyleeshine/mpquic/repository/lucas-clemente/quic-go"
	"io"

	"math/big"
	"os"
	"time"
)

//The reciever function that recieves the frames from the sender
//input args - the directory to store the frames. Run the viewer function to show the video
var FilePath string

func ParseCommandLine() {
	flag.StringVar(&FilePath, "f", " ", "-f file.flv")
	flag.Parse()
}

const quicServerAddr = "127.0.0.1:5252"

var elapsed time.Duration
var size int64

func HandleError(err error) {
	if err != nil {
		fmt.Println("App elapsed: ", elapsed)
		fmt.Println("Error: ", err)
		os.Exit(1)
	}
}

func main() {
	ParseCommandLine()
	if len(FilePath) <= 1 {
		fmt.Println("./flvparse -f filename.flv")
		return
	}
	fmt.Println(len(FilePath))
	f, err := os.OpenFile("./clientlog.txt", os.O_CREATE|os.O_RDWR|os.O_APPEND, 0666)
	defer f.Close()
	if err != nil {
		panic(err)
	}
	now := time.Now()
	pullflv(quicServerAddr, FilePath)
	fmt.Println(time.Since(now))
}

func pullflv(url, filename string) {
	var (
		w   httpflv.FLVFileWriter
		err error
	)

	err = w.Open(filename)
	nazalog.Assert(nil, err)
	defer w.Dispose()
	err = w.WriteRaw(httpflv.FLVHeader)
	nazalog.Assert(nil, err)

	quicConfig := &quic.Config{
		CreatePaths: true,
	}
	sess, err := quic.DialAddr(url, &tls.Config{InsecureSkipVerify: true}, quicConfig)
	HandleError(err)

	//---------------------------------------------
	//打开三个流：key帧流、控制流、pb流
	controlstream, err := sess.AcceptStream()
	defer controlstream.Close()
	HandleError(err)
	keystream, err := sess.AcceptStream()
	defer keystream.Close()
	HandleError(err)
	videostream, err := sess.AcceptStream()
	defer videostream.Close()
	HandleError(err)
	//----------------------------------------------
	time.Sleep(time.Millisecond * 200)
	//---------------------------------------------
	//第一块为了接收metaTag
	metacontrolinfo := make([]byte, 11+4)

	_, err2 := io.ReadFull(controlstream, metacontrolinfo) //recieve the size
	HandleError(err2)
	metatag := httpflv.Tag{}
	metatag.Header = parseTagHeader(metacontrolinfo[0:11])
	metatag.Raw = make([]byte, metatag.Header.DataSize+15)
	copy(metatag.Raw[0:11], metacontrolinfo[0:11])

	_, err3 := io.ReadFull(controlstream, metatag.Raw[11:11+metatag.Header.DataSize])
	HandleError(err3)
	copy(metatag.Raw[metatag.Header.DataSize+11:metatag.Header.DataSize+15], metacontrolinfo[11:15])
	w.WriteTag(metatag)
	//----------------------------------------------

	preTagTS := uint32(0)
	for {
		controlinfo := make([]byte, 20) //一个tagHeader 一个pretagsize videotagdata:前5个字节

		_, err := io.ReadFull(controlstream, controlinfo) // recieve the size
		str := string(controlinfo[0:3])
		if str == "fin" {
			fmt.Println("正常结束！！！")
			break
		}
		HandleError(err)

		tag := httpflv.Tag{}
		tag.Header = parseTagHeader(controlinfo[0:11]) // 解析tagHeader

		tag.Raw = make([]byte, tag.Header.DataSize+15) //  原始数据
		copy(tag.Raw[0:16], controlinfo[0:16])
		copy(tag.Raw[tag.Header.DataSize+11:tag.Header.DataSize+15], controlinfo[16:20])
		//判断该帧类型是keyFrame或者是其他的Frame类型
		if tag.Header.Type == httpflv.TagTypeVideo && tag.Raw[httpflv.TagHeaderSize] == httpflv.AVCKeyFrame {
			// keyFrame，使用可靠流传输

			io.ReadFull(keystream, tag.Raw[16:11+tag.Header.DataSize])
			//fmt.Println("key:",len(tag.Raw))
		} else {
			// 非keyFrame，使用非可靠流传输

			io.ReadFull(videostream, tag.Raw[16:11+tag.Header.DataSize])
			//fmt.Println("nonekey:",len(tag.Raw))
		}
		w.WriteTag(tag)
		dura := time.Duration(tag.Header.Timestamp-preTagTS) * time.Millisecond
		preTagTS = tag.Header.Timestamp
		time.Sleep(dura)

	}

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

func parseTagHeader(rawHeader []byte) httpflv.TagHeader {
	var h httpflv.TagHeader
	h.Type = rawHeader[0]
	h.DataSize = bele.BEUint24(rawHeader[1:])
	h.Timestamp = (uint32(rawHeader[7]) << 24) + bele.BEUint24(rawHeader[4:])
	return h
}
