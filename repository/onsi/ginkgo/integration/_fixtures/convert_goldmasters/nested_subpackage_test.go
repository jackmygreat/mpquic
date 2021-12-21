package subpackage

import (
	. "github.com/yyleeshine/mpquic/repository/onsi/ginkgo"
)

var _ = Describe("Testing with Ginkgo", func() {
	It("nested sub packages", func() {
		GinkgoT().Fail(true)
	})
})
