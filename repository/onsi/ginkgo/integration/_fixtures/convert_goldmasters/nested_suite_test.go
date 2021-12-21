package nested_test

import (
	"testing"

	. "github.com/yyleeshine/mpquic/repository/onsi/ginkgo"
	. "github.com/yyleeshine/mpquic/repository/onsi/gomega"
)

func TestNested(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Nested Suite")
}
