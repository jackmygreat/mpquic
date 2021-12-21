package h2quic

import (
	. "github.com/yyleeshine/mpquic/repository/onsi/ginkgo"
	. "github.com/yyleeshine/mpquic/repository/onsi/gomega"

	"testing"
)

func TestH2quic(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "H2quic Suite")
}
