package integration

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	k8s "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	k8smeta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"

	"kubevirt.io/api/core/v1"
	"kubevirt.io/client-go/kubecli"

	"kubevirt.io/kubevirt/pkg/libvmi"
	fg "kubevirt.io/kubevirt/pkg/virt-config/featuregate"
	"kubevirt.io/kubevirt/tests/console"
	"kubevirt.io/kubevirt/tests/libvmifact"
	"kubevirt.io/kubevirt/tests/libvmops"
	"kubevirt.io/kubevirt/tests/libwait"

	g "github.com/onsi/gomega"
)

const TestVMINamePrefix = "testvmi"
const sysNetPath = "/sys/class/net/"

func interfaceDeviceWithVDPABinding(name string) v1.Interface {
	return v1.Interface{
		Name:    name,
		Binding: &v1.PluginBinding{Name: "vdpa"},
	}
}

func newAlpineVDPAVMI(nads []string, ifaceNames []string, opts ...libvmi.Option) *v1.VirtualMachineInstance {
	options := []libvmi.Option{}

	for i, nadName := range nads {
		ifaceName := ifaceNames[i]
		options = append(options,
			libvmi.WithInterface(interfaceDeviceWithVDPABinding(ifaceName)),
			libvmi.WithNetwork(libvmi.MultusNetwork(ifaceName, nadName)),
		)
	}
	opts = append(options, opts...)
	return libvmifact.NewAlpine(opts...)
}

func createTestNamespace(name string, client kubecli.KubevirtClient) {
	ns := &k8s.Namespace{
		ObjectMeta: k8smeta.ObjectMeta{
			Name: name,
		},
	}

	_, err := client.CoreV1().Namespaces().Create(context.Background(), ns, k8smeta.CreateOptions{})
	if k8serrors.IsAlreadyExists(err) {
		return
	}
	g.Expect(err).ToNot(g.HaveOccurred())

	g.Eventually(func() k8s.NamespacePhase {
		ns, err := client.CoreV1().Namespaces().Get(context.Background(), name, k8smeta.GetOptions{})
		g.Expect(err).ToNot(g.HaveOccurred())
		return ns.Status.Phase
	}).WithTimeout(2 * time.Minute).WithPolling(5 * time.Second).Should(g.Equal(k8s.NamespaceActive))
}

func deleteTestNamespaceAndWait(name string, client kubecli.KubevirtClient) {
	err := client.CoreV1().Namespaces().Delete(context.Background(), name, k8smeta.DeleteOptions{})
	g.Expect(err).ToNot(g.HaveOccurred())

	g.Eventually(func() bool {
		_, err := client.CoreV1().Namespaces().Get(context.Background(), name, k8smeta.GetOptions{})
		return k8serrors.IsNotFound(err)
	}).WithTimeout(2 * time.Minute).WithPolling(5 * time.Second).Should(g.BeTrue())
}

func enableReservedOverheadMemlockFeatureGate(client kubecli.KubevirtClient) {
	kv, err := client.KubeVirt("kubevirt").Get(context.Background(), "kubevirt", k8smeta.GetOptions{})
	g.Expect(err).ToNot(g.HaveOccurred())

	if kv.Spec.Configuration.DeveloperConfiguration != nil {
		if slices.Contains(
			kv.Spec.Configuration.DeveloperConfiguration.FeatureGates,
			fg.ReservedOverheadMemlock,
		) {
			return
		}
	} else {
		kv.Spec.Configuration.DeveloperConfiguration = &v1.DeveloperConfiguration{}
	}

	kv.Spec.Configuration.DeveloperConfiguration.FeatureGates = append(
		kv.Spec.Configuration.DeveloperConfiguration.FeatureGates,
		"ReservedOverheadMemlock",
	)

	_, err = client.KubeVirt("kubevirt").Update(context.Background(), kv, k8smeta.UpdateOptions{})
	g.Expect(err).ToNot(g.HaveOccurred())
}

func disableReservedOverheadMemlockFeatureGate(client kubecli.KubevirtClient) {
	kv, err := client.KubeVirt("kubevirt").Get(context.Background(), "kubevirt", k8smeta.GetOptions{})
	g.Expect(err).ToNot(g.HaveOccurred())

	if kv.Spec.Configuration.DeveloperConfiguration == nil {
		return
	}

	i := slices.Index(kv.Spec.Configuration.DeveloperConfiguration.FeatureGates, fg.ReservedOverheadMemlock)
	if i == -1 {
		return
	}

	kv.Spec.Configuration.DeveloperConfiguration.FeatureGates = slices.Delete(
		kv.Spec.Configuration.DeveloperConfiguration.FeatureGates, i, i+1,
	)

	_, err = client.KubeVirt("kubevirt").Update(context.Background(), kv, k8smeta.UpdateOptions{})
	g.Expect(err).ToNot(g.HaveOccurred())
}

func RandomVMIName() string {
	return fmt.Sprintf("%s-%s", TestVMINamePrefix, rand.String(6))
}

func deleteVMIAndWait(vmi *v1.VirtualMachineInstance, client kubecli.KubevirtClient) {
	err := client.VirtualMachineInstance(vmi.Namespace).Delete(context.Background(), vmi.Name, k8smeta.DeleteOptions{})
	g.Expect(err).ToNot(g.HaveOccurred())

	g.Expect(libwait.WaitForVirtualMachineToDisappearWithTimeout(vmi, 2*time.Minute)).To(g.Succeed())
}

func guestNetDevNames(vmi *v1.VirtualMachineInstance) []string {
	netDevStr, err := console.RunCommandAndStoreOutput(
		vmi,
		fmt.Sprintf("ls --color=never %s", sysNetPath),
		libvmops.StartupTimeoutSecondsTiny*time.Second,
	)
	g.Expect(err).To(g.BeNil())
	return strings.Fields(netDevStr)
}

func withoutLoopbackInterface(guestNetDevNames []string) []string {
	loIfaceIndex := slices.Index(guestNetDevNames, "lo")
	g.Expect(loIfaceIndex).NotTo(g.Equal(-1), "loopback interface is not present in the guest")
	return slices.Delete(guestNetDevNames, loIfaceIndex, loIfaceIndex+1)
}

func guestNetDevMacAddress(vmi *v1.VirtualMachineInstance, iface string) string {
	macAddr, err := console.RunCommandAndStoreOutput(
		vmi,
		fmt.Sprintf("cat %s", filepath.Join(sysNetPath, iface, "address")),
		libvmops.StartupTimeoutSecondsTiny*time.Second,
	)
	g.Expect(err).To(g.BeNil())
	return strings.TrimSpace(macAddr)
}

func checkGuestIfaces(
	vmi *v1.VirtualMachineInstance,
	macAddresses []string,
) {
	netIfaceNames := guestNetDevNames(vmi)
	netIfaceNames = withoutLoopbackInterface(netIfaceNames)
	g.Expect(len(netIfaceNames)).To(g.Equal(len(macAddresses)))

	for i, guestVdpaIface := range netIfaceNames {
		guestMacAddr := guestNetDevMacAddress(vmi, guestVdpaIface)
		g.Expect(guestMacAddr).To(g.Equal(macAddresses[i]))
	}

}

func domainNicNames(amount int) []string {
	var nicNames []string

	for i := range amount {
		nicNames = append(nicNames, fmt.Sprintf("vdpanet%d", i))
	}

	return nicNames
}
