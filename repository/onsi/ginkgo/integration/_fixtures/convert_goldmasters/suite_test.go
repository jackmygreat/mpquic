package tmp_test

import (
	"testing"

	. "github.com/yyleeshine/mpquic/repository/onsi/ginkgo"
	. "github.com/yyleeshine/mpquic/repository/onsi/gomega"
)

func TestConvertFixtures(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "ConvertFixtures Suite")
}
