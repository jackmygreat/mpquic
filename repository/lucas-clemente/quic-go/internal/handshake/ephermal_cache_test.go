package handshake

import (
	"time"

	"github.com/yyleeshine/mpquic/repository/lucas-clemente/quic-go/internal/crypto"
	"github.com/yyleeshine/mpquic/repository/lucas-clemente/quic-go/internal/protocol"
	. "github.com/yyleeshine/mpquic/repository/onsi/ginkgo"
	. "github.com/yyleeshine/mpquic/repository/onsi/gomega"
)

var _ = Describe("Ephermal KEX", func() {
	It("has a consistent KEX", func() {
		kex1 := getEphermalKEX()
		Expect(kex1).ToNot(BeNil())
		kex2 := getEphermalKEX()
		Expect(kex2).ToNot(BeNil())
		Expect(kex1).To(Equal(kex2))
	})

	It("changes KEX", func() {
		kexLifetime = time.Millisecond
		defer func() {
			kexLifetime = protocol.EphermalKeyLifetime
		}()
		kex := getEphermalKEX()
		Expect(kex).ToNot(BeNil())
		Eventually(func() crypto.KeyExchange { return getEphermalKEX() }).ShouldNot(Equal(kex))
	})
})
