package hanging_suite_test

import (
	. "github.com/yyleeshine/mpquic/repository/onsi/ginkgo"
	. "github.com/yyleeshine/mpquic/repository/onsi/gomega"

	"testing"
)

func TestHangingSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "HangingSuite Suite")
}
