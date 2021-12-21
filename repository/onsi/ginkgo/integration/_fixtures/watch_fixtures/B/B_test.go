package B_test

import (
	. "github.com/yyleeshine/mpquic/repository/onsi/ginkgo/integration/_fixtures/watch_fixtures/B"

	. "github.com/yyleeshine/mpquic/repository/onsi/ginkgo"
	. "github.com/yyleeshine/mpquic/repository/onsi/gomega"
)

var _ = Describe("B", func() {
	It("should do it", func() {
		Ω(DoIt()).Should(Equal("done!"))
	})
})
