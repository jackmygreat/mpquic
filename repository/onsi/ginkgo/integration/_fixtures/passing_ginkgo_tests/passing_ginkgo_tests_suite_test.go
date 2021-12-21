package passing_ginkgo_tests_test

import (
	. "github.com/yyleeshine/mpquic/repository/onsi/ginkgo"
	. "github.com/yyleeshine/mpquic/repository/onsi/gomega"

	"testing"
)

func TestPassing_ginkgo_tests(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Passing_ginkgo_tests Suite")
}
