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
	if len(FilePath)<=1 {
		fmt.Println("./flvparse -f filename.flv")
		return
	}
	fmt.Println(len(FilePath))
	f, err := os.OpenFile("./clientlog.txt", os.O_CREATE|os.O_RDWR|os.O_APPEND, 0666)
	defer f.Close()
	if err != nil {
		panic(err)
	}
	pullflv(quicServerAddr,FilePath)
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
	listener, err := quic.ListenAddr(quicServerAddr, generateTLSConfig(), quicConfig)
	HandleError(err)
	sess, err := listener.Accept() //accepting a session from sender
	HandleError(err)
	videostream, err := sess.AcceptStream()
	HandleError(err)
	controlstream, err := sess.AcceptStream()
	HandleError(err)
	for {
		controlinfo := make([]byte,11+4)
		_, err := io.ReadFull(controlstream, controlinfo) //recieve the size
		HandleError(err)
		tag := httpflv.Tag{}
		/*buf.Write(controlinfo[0:13])
		errs := binary.Read(buf,binary.BigEndian,&(tag.Header))
		fmt.Println(tag.Header)*/
		//HandleError(errs)
		//streamSender.Write(siz)
		//sliceChan<-siz
		tag.Header = parseTagHeader(controlinfo[0:11])
		tag.Raw = make([]byte, tag.Header.DataSize+15)
		len2, err := io.ReadFull(videostream, tag.Raw[11:11+tag.Header.DataSize]) // recieve image

		copy(tag.Raw[0:11],controlinfo[0:11])
		copy(tag.Raw[tag.Header.DataSize+11:tag.Header.DataSize+15],controlinfo[11:15])
		//sliceChan2<-buff
		HandleError(err)

		//if empty buffer
		if len2 == 0 {
			defer videostream.Close()
			defer controlstream.Close()
			return
		}
		w.WriteTag(tag)
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