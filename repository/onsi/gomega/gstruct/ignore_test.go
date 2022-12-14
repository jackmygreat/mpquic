package gstruct_test

import (
	. "github.com/yyleeshine/mpquic/repository/onsi/ginkgo"
	. "github.com/yyleeshine/mpquic/repository/onsi/gomega"
	. "github.com/yyleeshine/mpquic/repository/onsi/gomega/gstruct"
)

var _ = Describe("Ignore", func() {
	It("should always succeed", func() {
		Ω(nil).Should(Ignore())
		Ω(struct{}{}).Should(Ignore())
		Ω(0).Should(Ignore())
		Ω(false).Should(Ignore())
	})

	It("should always fail", func() {
		Ω(nil).ShouldNot(Reject())
		Ω(struct{}{}).ShouldNot(Reject())
		Ω(1).ShouldNot(Reject())
		Ω(true).ShouldNot(Reject())
	})
})
