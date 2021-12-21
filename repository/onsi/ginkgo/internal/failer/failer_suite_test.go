package failer_test

import (
	. "github.com/yyleeshine/mpquic/repository/onsi/ginkgo"
	. "github.com/yyleeshine/mpquic/repository/onsi/gomega"

	"testing"
)

func TestFailer(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Failer Suite")
}
