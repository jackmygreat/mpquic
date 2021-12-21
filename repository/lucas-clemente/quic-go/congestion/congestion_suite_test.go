package congestion

import (
	. "github.com/yyleeshine/mpquic/repository/onsi/ginkgo"
	. "github.com/yyleeshine/mpquic/repository/onsi/gomega"

	"testing"
)

func TestCongestion(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Congestion Suite")
}
