package fail_fixture_test

import (
	. "github.com/yyleeshine/mpquic/repository/onsi/ginkgo"
	. "github.com/yyleeshine/mpquic/repository/onsi/gomega"

	"testing"
)

func TestFail_fixture(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Skip_fixture Suite")
}
