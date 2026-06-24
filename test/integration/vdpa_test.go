package integration

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"kubevirt.io/api/core/v1"
	"kubevirt.io/client-go/kubecli"
	"kubevirt.io/kubevirt/pkg/libvmi"
	"kubevirt.io/kubevirt/tests/console"
	"kubevirt.io/kubevirt/tests/flags"
	"kubevirt.io/kubevirt/tests/libmigration"
	"kubevirt.io/kubevirt/tests/libvmops"
)

const vdpaTestNamespace = "vdpa-network-binding-plugin-tests"

const vdpasimnetNetworkAttachDefUnique1 = "default/vdpasimnet-unique-device-1"
const vdpasimnetNetworkAttachDefUnique2 = "default/vdpasimnet-unique-device-2"

const vdpasimnetUnique1MacAddr = "02:01:01:01:01:01"
const vdpasimnetUnique2MacAddr = "02:02:02:02:02:02"

var _ = Describe("guest with vdpa interfaces", func() {

	var vmiName string
	var vmi *v1.VirtualMachineInstance
	var virClient kubecli.KubevirtClient
	var err error

	flags.NormalizeFlags()

	BeforeEach(func() {
		vmiName = RandomVMIName()
		virClient, err = kubecli.GetKubevirtClient()
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		deleteVMIAndWait(vmi, virClient)
	})

	Context("guest gets a properly configured virtio device", func() {
		DescribeTable("guest reads expected interface attributes",
			func(networks []string, expectedMacAddresses []string) {
				By("run VMI with VDPA interfaces")
				vmi = newAlpineVDPAVMI(networks, domainNicNames(len(networks)),
					libvmi.WithNamespace(vdpaTestNamespace),
					libvmi.WithName(vmiName),
				)
				vmi = libvmops.RunVMIAndExpectLaunch(vmi, libvmops.StartupTimeoutSecondsSmall)

				By("logging in")
				Expect(console.LoginToAlpine(vmi)).To(Succeed())

				By("check guest interfaces")
				checkGuestIfaces(vmi, expectedMacAddresses)
			},
			Entry("with an unique vdpa device",
				[]string{vdpasimnetNetworkAttachDefUnique1},
				[]string{vdpasimnetUnique1MacAddr},
			),
			Entry("with multiple vdpa devices from different NADs",
				[]string{
					vdpasimnetNetworkAttachDefUnique1,
					vdpasimnetNetworkAttachDefUnique2,
				},
				[]string{vdpasimnetUnique1MacAddr, vdpasimnetUnique2MacAddr},
			),
		)
	})

	DescribeTable("VMI with vdpa devices attached is live migratable",
		func(networks []string, expectedMacAddresses []string) {
			By("run VMI with VDPA interfaces")
			vmi = newAlpineVDPAVMI(networks, domainNicNames(len(networks)),
				libvmi.WithNamespace(vdpaTestNamespace),
				libvmi.WithName(vmiName),
			)
			vmi = libvmops.RunVMIAndExpectLaunch(vmi, libvmops.StartupTimeoutSecondsSmall)

			By("VM booted in src")
			Expect(console.LoginToAlpine(vmi)).To(Succeed())

			By("starting the migration")
			migration := libmigration.New(vmi.Name, vmi.Namespace)
			migration = libmigration.RunMigrationAndExpectToCompleteWithDefaultTimeout(virClient, migration)
			libmigration.ConfirmVMIPostMigration(virClient, vmi, migration)

			By("logging in dst")
			Expect(console.LoginToAlpine(vmi)).To(Succeed())

			By("check guest interfaces in dst")
			checkGuestIfaces(vmi, expectedMacAddresses)
		},
		Entry("unique device",
			[]string{vdpasimnetNetworkAttachDefUnique1},
			[]string{vdpasimnetUnique1MacAddr},
		),
		Entry("multiple devices from different NADs",
			[]string{
				vdpasimnetNetworkAttachDefUnique1,
				vdpasimnetNetworkAttachDefUnique2,
			},
			[]string{vdpasimnetUnique1MacAddr, vdpasimnetUnique2MacAddr},
		),
	)
})
