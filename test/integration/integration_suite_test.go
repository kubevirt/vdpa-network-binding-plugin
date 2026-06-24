package integration

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"kubevirt.io/client-go/kubecli"
)

func TestVdpaIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "vDPA Network Binding Plugin integration test suite")
}

var _ = BeforeSuite(func() {
	client, err := kubecli.GetKubevirtClient()
	Expect(err).ToNot(HaveOccurred())

	createTestNamespace(vdpaTestNamespace, client)
	enableReservedOverheadMemlockFeatureGate(client)
})

var _ = AfterSuite(func() {
	client, err := kubecli.GetKubevirtClient()
	Expect(err).ToNot(HaveOccurred())

	disableReservedOverheadMemlockFeatureGate(client)
})

var _ = ReportAfterSuite("cleanup test namespace", func(report Report) {
	client, err := kubecli.GetKubevirtClient()
	Expect(err).ToNot(HaveOccurred())

	deleteTestNamespaceAndWait(vdpaTestNamespace, client)
})
