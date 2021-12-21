package progress_fixture_test

import (
	. "github.com/yyleeshine/mpquic/repository/onsi/ginkgo"
	. "github.com/yyleeshine/mpquic/repository/onsi/gomega"

	"testing"
)

func TestProgressFixture(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "ProgressFixture Suite")
}
