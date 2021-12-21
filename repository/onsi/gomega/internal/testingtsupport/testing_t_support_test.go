package testingtsupport_test

import (
	. "github.com/yyleeshine/mpquic/repository/onsi/gomega"

	"testing"
)

func TestTestingT(t *testing.T) {
	RegisterTestingT(t)
	Ω(true).Should(BeTrue())
}
