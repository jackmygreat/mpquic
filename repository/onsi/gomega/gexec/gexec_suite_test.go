package gexec_test

import (
	. "github.com/yyleeshine/mpquic/repository/onsi/ginkgo"
	. "github.com/yyleeshine/mpquic/repository/onsi/gomega"
	"github.com/yyleeshine/mpquic/repository/onsi/gomega/gexec"

	"testing"
)

var fireflyPath string

func TestGexec(t *testing.T) {
	BeforeSuite(func() {
		var err error
		fireflyPath, err = gexec.Build("./_fixture/firefly")
		Ω(err).ShouldNot(HaveOccurred())
	})

	AfterSuite(func() {
		gexec.CleanupBuildArtifacts()
	})

	RegisterFailHandler(Fail)
	RunSpecs(t, "Gexec Suite")
}
