package A_test

import (
	. "github.com/yyleeshine/mpquic/repository/onsi/ginkgo/integration/_fixtures/watch_fixtures/A"

	. "github.com/yyleeshine/mpquic/repository/onsi/ginkgo"
	. "github.com/yyleeshine/mpquic/repository/onsi/gomega"
)

var _ = Describe("A", func() {
	It("should do it", func() {
		Ω(DoIt()).Should(Equal("done!"))
	})
})
