package congestion

import (
	"time"

	. "github.com/yyleeshine/mpquic/repository/onsi/ginkgo"
	. "github.com/yyleeshine/mpquic/repository/onsi/gomega"
)

var _ = Describe("Bandwidth", func() {
	It("converts from time delta", func() {
		Expect(BandwidthFromDelta(1, time.Millisecond)).To(Equal(1000 * BytesPerSecond))
	})
})
