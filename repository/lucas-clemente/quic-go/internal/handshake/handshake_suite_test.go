package handshake

import (
	. "github.com/yyleeshine/mpquic/repository/onsi/ginkgo"
	. "github.com/yyleeshine/mpquic/repository/onsi/gomega"

	"testing"
)

func TestQuicGo(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Handshake Suite")
}
