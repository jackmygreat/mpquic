package B_test

import (
	. "github.com/yyleeshine/mpquic/repository/onsi/ginkgo"
	. "github.com/yyleeshine/mpquic/repository/onsi/gomega"

	"testing"
)

func TestB(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "B Suite")
}
