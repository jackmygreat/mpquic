package writer_test

import (
	. "github.com/yyleeshine/mpquic/repository/onsi/ginkgo"
	. "github.com/yyleeshine/mpquic/repository/onsi/gomega"

	"testing"
)

func TestWriter(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Writer Suite")
}
