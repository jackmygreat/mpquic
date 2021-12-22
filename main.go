package main

import (
	"flag"
	"fmt"
	"github.com/yyleeshine/flvParse/flv"
	"os"
)

var GConfFile string

func ParseCommandLine() {
	flag.StringVar(&GConfFile, "f", " ", "-f file.flv")
	flag.Parse()
}

func main(){
	ParseCommandLine()
	if len(GConfFile)<=1 {
		fmt.Println("./flvparse -f filename.flv")
		return
	}
	fmt.Println(len(GConfFile))
	r, err := os.Open(GConfFile)
	if err != nil {
		fmt.Println("err:",err)
	}

	fdm:=flv.NewDemuxer(r)
	err=fdm.FlvParse()
	fmt.Println(err)
}
