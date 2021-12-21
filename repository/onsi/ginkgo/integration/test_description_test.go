package integration_test

import (
	. "github.com/yyleeshine/mpquic/repository/onsi/ginkgo"
	. "github.com/yyleeshine/mpquic/repository/onsi/gomega"
	"github.com/yyleeshine/mpquic/repository/onsi/gomega/gbytes"
	"github.com/yyleeshine/mpquic/repository/onsi/gomega/gexec"
)

var _ = Describe("TestDescription", func() {
	var pathToTest string

	BeforeEach(func() {
		pathToTest = tmpPath("test_description")
		copyIn("test_description", pathToTest)
	})

	It("should capture and emit information about the current test", func() {
		session := startGinkgo(pathToTest, "--noColor")
		Eventually(session).Should(gexec.Exit(1))

		Ω(session).Should(gbytes.Say("TestDescription should pass:false"))
		Ω(session).Should(gbytes.Say("TestDescription should fail:true"))
	})
})
