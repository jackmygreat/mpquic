package codelocation_test

import (
	. "github.com/yyleeshine/mpquic/repository/onsi/ginkgo"
	. "github.com/yyleeshine/mpquic/repository/onsi/gomega"

	"testing"
)

func TestCodelocation(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "CodeLocation Suite")
}
