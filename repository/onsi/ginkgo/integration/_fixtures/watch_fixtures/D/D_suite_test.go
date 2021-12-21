package D_test

import (
	. "github.com/yyleeshine/mpquic/repository/onsi/ginkgo"
	. "github.com/yyleeshine/mpquic/repository/onsi/gomega"

	"testing"
)

func TestD(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "D Suite")
}
