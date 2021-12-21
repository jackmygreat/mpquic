package crypto

import (
	. "github.com/yyleeshine/mpquic/repository/onsi/ginkgo"
	. "github.com/yyleeshine/mpquic/repository/onsi/gomega"

	"testing"
)

func TestCrypto(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Crypto Suite")
}
