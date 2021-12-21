package suite_command_test

import (
	. "github.com/yyleeshine/mpquic/repository/onsi/ginkgo"
	. "github.com/yyleeshine/mpquic/repository/onsi/gomega"

	"testing"
)

func TestSuiteCommand(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Suite Command Suite")
}
