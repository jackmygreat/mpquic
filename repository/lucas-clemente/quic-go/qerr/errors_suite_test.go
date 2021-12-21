package qerr

import (
	"testing"

	. "github.com/yyleeshine/mpquic/repository/onsi/ginkgo"
	. "github.com/yyleeshine/mpquic/repository/onsi/gomega"
)

func TestErrorcodes(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Errors Suite")
}
