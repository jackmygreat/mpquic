package utils

import (
	. "github.com/yyleeshine/mpquic/repository/onsi/ginkgo"
	. "github.com/yyleeshine/mpquic/repository/onsi/gomega"
)

var _ = Describe("Connection ID generation", func() {
	It("generates random connection IDs", func() {
		c1, err := GenerateConnectionID()
		Expect(err).ToNot(HaveOccurred())
		Expect(c1).ToNot(BeZero())
		c2, err := GenerateConnectionID()
		Expect(err).ToNot(HaveOccurred())
		Expect(c1).ToNot(Equal(c2))
	})
})
