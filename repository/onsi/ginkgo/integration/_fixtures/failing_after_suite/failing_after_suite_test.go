package failing_before_suite_test

import (
	. "github.com/yyleeshine/mpquic/repository/onsi/ginkgo"
)

var _ = Describe("FailingBeforeSuite", func() {
	It("should run", func() {
		println("A TEST")
	})

	It("should run", func() {
		println("A TEST")
	})
})
