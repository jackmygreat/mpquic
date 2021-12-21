package crypto

import (
	"github.com/yyleeshine/mpquic/repository/lucas-clemente/quic-go/internal/protocol"
	. "github.com/yyleeshine/mpquic/repository/onsi/ginkgo"
	. "github.com/yyleeshine/mpquic/repository/onsi/gomega"
)

var _ = Describe("NullAEAD", func() {
	It("selects the right FVN variant", func() {
		Expect(NewNullAEAD(protocol.PerspectiveClient, protocol.Version39)).To(Equal(&nullAEADFNV128a{
			perspective: protocol.PerspectiveClient,
		}))
		Expect(NewNullAEAD(protocol.PerspectiveClient, protocol.VersionTLS)).To(Equal(&nullAEADFNV64a{}))
	})
})
