package A_test

import (
	. "github.com/yyleeshine/mpquic/repository/onsi/ginkgo"
	. "github.com/yyleeshine/mpquic/repository/onsi/gomega"

	"testing"
)

func TestA(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "A Suite")
}
