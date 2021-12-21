package C_test

import (
	. "github.com/yyleeshine/mpquic/repository/onsi/ginkgo/integration/_fixtures/watch_fixtures/C"

	. "github.com/yyleeshine/mpquic/repository/onsi/ginkgo"
	. "github.com/yyleeshine/mpquic/repository/onsi/gomega"
)

var _ = Describe("C", func() {
	It("should do it", func() {
		Ω(DoIt()).Should(Equal("done!"))
	})
})
