package eventually_failing_test

import (
	. "github.com/yyleeshine/mpquic/repository/onsi/ginkgo"
	. "github.com/yyleeshine/mpquic/repository/onsi/gomega"

	"testing"
)

func TestEventuallyFailing(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "EventuallyFailing Suite")
}
