package utils

import (
	"github.com/yyleeshine/mpquic/repository/lucas-clemente/quic-go/internal/protocol"
	. "github.com/yyleeshine/mpquic/repository/onsi/ginkgo"
	. "github.com/yyleeshine/mpquic/repository/onsi/gomega"
)

var _ = Describe("Byte Order", func() {
	It("says little Little Endian before QUIC 39", func() {
		Expect(GetByteOrder(protocol.Version37)).To(Equal(LittleEndian))
		Expect(GetByteOrder(protocol.Version38)).To(Equal(LittleEndian))
	})

	It("says little Little Endian for QUIC 39", func() {
		Expect(GetByteOrder(protocol.Version39)).To(Equal(BigEndian))
	})
})
