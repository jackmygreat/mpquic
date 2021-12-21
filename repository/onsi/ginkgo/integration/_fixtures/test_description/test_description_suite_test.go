package test_description_test

import (
	. "github.com/yyleeshine/mpquic/repository/onsi/ginkgo"
	. "github.com/yyleeshine/mpquic/repository/onsi/gomega"

	"testing"
)

func TestTestDescription(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "TestDescription Suite")
}
