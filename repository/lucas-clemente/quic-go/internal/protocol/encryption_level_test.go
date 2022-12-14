package protocol

import (
	. "github.com/yyleeshine/mpquic/repository/onsi/ginkgo"
	. "github.com/yyleeshine/mpquic/repository/onsi/gomega"
)

var _ = Describe("Encryption Level", func() {
	It("has the correct string representation", func() {
		Expect(EncryptionUnspecified.String()).To(Equal("unknown"))
		Expect(EncryptionUnencrypted.String()).To(Equal("unencrypted"))
		Expect(EncryptionSecure.String()).To(Equal("encrypted (not forward-secure)"))
		Expect(EncryptionForwardSecure.String()).To(Equal("forward-secure"))
	})
})
