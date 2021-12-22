module github.com/yyleeshine/mpquic

require (
	github.com/golang/protobuf v1.5.2
	github.com/q191201771/lal v0.26.0
	github.com/q191201771/naza v0.18.5
	github.com/yyleeshine/flvParse v0.0.1
	go4.org v0.0.0-20201209231011-d4a079459e60
	gocv.io/x/gocv v0.29.0
	golang.org/x/sys v0.0.0-20211216021012-1d35b9e2eb4e
	golang.org/x/tools v0.1.8
)

require google.golang.org/protobuf v1.26.0 // indirect

replace (
	github.com/q191201771/lal => github.com/flyaways/lal v0.22.1
	gocv.io/x/gocv => /Users/liyahui/go/src/gocv.io/x/gocv
)

go 1.17
