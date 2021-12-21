package flowcontrol

import (
	"github.com/yyleeshine/mpquic/repository/golang/mock/gomock"
	. "github.com/yyleeshine/mpquic/repository/onsi/ginkgo"
	. "github.com/yyleeshine/mpquic/repository/onsi/gomega"

	"testing"
)

func TestCrypto(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "FlowControl Suite")
}

var mockCtrl *gomock.Controller

var _ = BeforeEach(func() {
	mockCtrl = gomock.NewController(GinkgoT())
})

var _ = AfterEach(func() {
	mockCtrl.Finish()
})
