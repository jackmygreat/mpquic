package matchers_test

import (
	"io/ioutil"
	"os"

	. "github.com/yyleeshine/mpquic/repository/onsi/ginkgo"
	. "github.com/yyleeshine/mpquic/repository/onsi/gomega"
	. "github.com/yyleeshine/mpquic/repository/onsi/gomega/matchers"
)

var _ = Describe("BeAnExistingFileMatcher", func() {
	Context("when passed a string", func() {
		It("should do the right thing", func() {
			Ω("/dne/test").ShouldNot(BeAnExistingFile())

			tmpFile, err := ioutil.TempFile("", "gomega-test-tempfile")
			Ω(err).ShouldNot(HaveOccurred())
			defer os.Remove(tmpFile.Name())
			Ω(tmpFile.Name()).Should(BeAnExistingFile())

			tmpDir, err := ioutil.TempDir("", "gomega-test-tempdir")
			Ω(err).ShouldNot(HaveOccurred())
			defer os.Remove(tmpDir)
			Ω(tmpDir).Should(BeAnExistingFile())
		})
	})

	Context("when passed something else", func() {
		It("should error", func() {
			success, err := (&BeAnExistingFileMatcher{}).Match(nil)
			Ω(success).Should(BeFalse())
			Ω(err).Should(HaveOccurred())

			success, err = (&BeAnExistingFileMatcher{}).Match(true)
			Ω(success).Should(BeFalse())
			Ω(err).Should(HaveOccurred())
		})
	})
})
