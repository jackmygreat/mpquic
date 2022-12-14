package wire

import (
	"github.com/yyleeshine/mpquic/repository/lucas-clemente/quic-go/internal/protocol"
	"github.com/yyleeshine/mpquic/repository/lucas-clemente/quic-go/internal/utils"
	. "github.com/yyleeshine/mpquic/repository/onsi/ginkgo"
	. "github.com/yyleeshine/mpquic/repository/onsi/gomega"

	"testing"
)

func TestCrypto(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Wire Suite")
}

const (
	versionLittleEndian = protocol.Version37 // a QUIC version that uses little endian encoding
	versionBigEndian    = protocol.Version39 // a QUIC version that uses big endian encoding
)

var _ = BeforeSuite(func() {
	Expect(utils.GetByteOrder(versionLittleEndian)).To(Equal(utils.LittleEndian))
	Expect(utils.GetByteOrder(versionBigEndian)).To(Equal(utils.BigEndian))
})
