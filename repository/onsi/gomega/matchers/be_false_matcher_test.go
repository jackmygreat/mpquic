package matchers_test

import (
	. "github.com/yyleeshine/mpquic/repository/onsi/ginkgo"
	. "github.com/yyleeshine/mpquic/repository/onsi/gomega"
	. "github.com/yyleeshine/mpquic/repository/onsi/gomega/matchers"
)

var _ = Describe("BeFalse", func() {
	It("should handle true and false correctly", func() {
		Ω(true).ShouldNot(BeFalse())
		Ω(false).Should(BeFalse())
	})

	It("should only support booleans", func() {
		success, err := (&BeFalseMatcher{}).Match("foo")
		Ω(success).Should(BeFalse())
		Ω(err).Should(HaveOccurred())
	})
})
