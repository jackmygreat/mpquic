package protocol

import (
	. "github.com/yyleeshine/mpquic/repository/onsi/ginkgo"
	. "github.com/yyleeshine/mpquic/repository/onsi/gomega"

	"testing"
)

func TestProtocol(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Protocol Suite")
}
