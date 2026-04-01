/*
 * This file is part of the KubeVirt project
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 * Copyright 2023 Red Hat, Inc.
 *
 */

package domain_test

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"

	networkv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	vmschema "kubevirt.io/api/core/v1"

	"kubevirt.io/kubevirt/cmd/sidecars/network-vdpa-binding/domain"

	"kubevirt.io/kubevirt/pkg/network/downwardapi"
	domainschema "kubevirt.io/kubevirt/pkg/virt-launcher/virtwrap/api"
)

const DEFAULT_CONT_NAME = "containername"

var DEFAULT_SYMLINK_DIR = path.Join(
	"/var/run/kubevirt-hooks/",
	DEFAULT_CONT_NAME,
)

func newNetInfo(networkName, vdpaPath, mac string) *downwardapi.NetworkInfo {
	return &downwardapi.NetworkInfo{
		Interfaces: []downwardapi.Interface{
			{
				Network: networkName,
				Mac:     mac,
				DeviceInfo: &networkv1.DeviceInfo{
					Type: networkv1.DeviceInfoTypeVDPA,
					Vdpa: &networkv1.VdpaDevice{Path: vdpaPath},
				},
			},
		},
	}
}

func slPath(name string) string {
	hash := sha256.New()
	_, _ = io.WriteString(hash, name)
	hashedName := fmt.Sprintf("pod%x", hash.Sum(nil))[:14]
	return path.Join(DEFAULT_SYMLINK_DIR, hashedName)
}

var _ = Describe("pod network configurator", func() {

	Context("generate domain spec interface", func() {
		//  These tests validate how the vDPA network configurator mutates the domain XML
		//  for different VMI interface configurations, using test helpers to avoid filesystem dependencies.
		DescribeTable("should fail to create configurator given",
			func(ifaces []vmschema.Interface, networks []vmschema.Network) {
				netInfo := &downwardapi.NetworkInfo{}
				_, err := domain.NewVdpaNetworkConfigurator(ifaces, networks, netInfo, DEFAULT_CONT_NAME)

				Expect(err).To(HaveOccurred())
			},
			Entry("no pod network",
				nil,
				[]vmschema.Network{{Name: "net-without-iface", NetworkSource: vmschema.NetworkSource{Multus: &vmschema.MultusNetwork{}}}},
			),
			Entry("no corresponding iface",
				[]vmschema.Interface{{Name: "vdpa-mismatch", Binding: &vmschema.PluginBinding{Name: "vdpa"}}},
				[]vmschema.Network{{Name: "net-mismatch", NetworkSource: vmschema.NetworkSource{Multus: &vmschema.MultusNetwork{}}}},
			),
			Entry("interface with no vdpa binding method",
				[]vmschema.Interface{{Name: "bridge-net", InterfaceBindingMethod: vmschema.InterfaceBindingMethod{Bridge: &vmschema.InterfaceBridge{}}}},
				[]vmschema.Network{{Name: "bridge-net", NetworkSource: vmschema.NetworkSource{Multus: &vmschema.MultusNetwork{}}}},
			),
			Entry("interface with no vdpa binding plugin",
				[]vmschema.Interface{{Name: "no-vdpa-plugin", Binding: &vmschema.PluginBinding{Name: "no-vdpa"}}},
				[]vmschema.Network{{Name: "no-vdpa-plugin", NetworkSource: vmschema.NetworkSource{Multus: &vmschema.MultusNetwork{}}}},
			),
		)

		It("should fail given interface with invalid PCI address", func() {
			ifaces := []vmschema.Interface{{Name: "invalid-pci", Binding: &vmschema.PluginBinding{Name: "vdpa"},
				PciAddress: "invalid-pci-address"}}
			networks := []vmschema.Network{{Name: "invalid-pci", NetworkSource: vmschema.NetworkSource{Multus: &vmschema.MultusNetwork{}}}}
			// netInfo is only required so NewVdpaNetworkConfigurator can pair the vDPA iface;
			// this test validates guest PCI address parsing, not host VF's PCI address.
			netInfo := newNetInfo("invalid-pci", "/dev/vhost-vdpa-0", "")

			testMutator, err := domain.NewVdpaNetworkConfigurator(ifaces, networks, netInfo, DEFAULT_CONT_NAME)
			Expect(err).ToNot(HaveOccurred())

			_, err = testMutator.Mutate(&domainschema.DomainSpec{})
			Expect(err).To(HaveOccurred())
		})

		DescribeTable("should add interface to domain spec given iface with",
			func(iface *vmschema.Interface, expectedDomainIface *domainschema.Interface, macFromDeviceInfo string) {
				ifaces := []vmschema.Interface{*iface}
				networks := []vmschema.Network{{Name: iface.Name, NetworkSource: vmschema.NetworkSource{Multus: &vmschema.MultusNetwork{}}}}
				netInfo := newNetInfo(iface.Name, "/dev/vhost-vdpa-0", macFromDeviceInfo)

				testMutator, err := domain.NewVdpaNetworkConfigurator(ifaces, networks, netInfo, DEFAULT_CONT_NAME)
				Expect(err).ToNot(HaveOccurred())

				mutatedDomSpec, err := testMutator.Mutate(&domainschema.DomainSpec{})
				Expect(err).ToNot(HaveOccurred())
				Expect(mutatedDomSpec.Devices.Interfaces).To(Equal([]domainschema.Interface{*expectedDomainIface}))
			},
			Entry("vdpa binding plugin",
				&vmschema.Interface{Name: "vdpa-minimal", Binding: &vmschema.PluginBinding{Name: "vdpa"}},
				&domainschema.Interface{
					Alias:  domainschema.NewUserDefinedAlias("vdpa-minimal"),
					Type:   "vdpa",
					Source: domainschema.InterfaceSource{Device: slPath("vdpa-minimal")},
					Model:  &domainschema.Model{Type: "virtio"},
					MAC:    nil,
				},
				"",
			),
			Entry("PCI address",
				&vmschema.Interface{Name: "vdpa-with-pci", Binding: &vmschema.PluginBinding{Name: "vdpa"},
					PciAddress: "0000:02:02.0"},
				&domainschema.Interface{
					Alias:   domainschema.NewUserDefinedAlias("vdpa-with-pci"),
					Type:    "vdpa",
					Source:  domainschema.InterfaceSource{Device: slPath("vdpa-with-pci")},
					Model:   &domainschema.Model{Type: "virtio"},
					Address: &domainschema.Address{Type: "pci", Domain: "0x0000", Bus: "0x02", Slot: "0x02", Function: "0x0"},
					MAC:     nil,
				},
				"",
			),
			Entry("MAC address",
				&vmschema.Interface{Name: "vdpa-with-mac", Binding: &vmschema.PluginBinding{Name: "vdpa"},
					MacAddress: "02:02:02:02:02:02"},
				&domainschema.Interface{
					Alias:  domainschema.NewUserDefinedAlias("vdpa-with-mac"),
					Type:   "vdpa",
					Source: domainschema.InterfaceSource{Device: slPath("vdpa-with-mac")},
					Model:  &domainschema.Model{Type: "virtio"},
					MAC:    &domainschema.MAC{MAC: "02:02:02:02:02:02"},
				},
				"",
			),
			Entry("ACPI address",
				&vmschema.Interface{Name: "vdpa-with-acpi", Binding: &vmschema.PluginBinding{Name: "vdpa"},
					ACPIIndex: 2},
				&domainschema.Interface{
					Alias:  domainschema.NewUserDefinedAlias("vdpa-with-acpi"),
					Type:   "vdpa",
					Source: domainschema.InterfaceSource{Device: slPath("vdpa-with-acpi")},
					Model:  &domainschema.Model{Type: "virtio"},
					ACPI:   &domainschema.ACPI{Index: uint(2)},
				},
				"",
			),
			Entry("MAC address from deviceinfo",
				&vmschema.Interface{Name: "deviceinfo-mac", Binding: &vmschema.PluginBinding{Name: "vdpa"}},
				&domainschema.Interface{
					Alias:  domainschema.NewUserDefinedAlias("deviceinfo-mac"),
					Type:   "vdpa",
					Source: domainschema.InterfaceSource{Device: slPath("deviceinfo-mac")},
					Model:  &domainschema.Model{Type: "virtio"},
					MAC:    &domainschema.MAC{MAC: "de:ad:00:00:be:af"},
				},
				"de:ad:00:00:be:af",
			),
			Entry("VMI MAC should override DeviceInfo MAC",
				&vmschema.Interface{Name: "mac-override", Binding: &vmschema.PluginBinding{Name: "vdpa"},
					MacAddress: "02:02:02:02:02:02",
				},
				&domainschema.Interface{
					Alias:  domainschema.NewUserDefinedAlias("mac-override"),
					Type:   "vdpa",
					Source: domainschema.InterfaceSource{Device: slPath("mac-override")},
					Model:  &domainschema.Model{Type: "virtio"},
					MAC:    &domainschema.MAC{MAC: "02:02:02:02:02:02"},
				},
				"de:ad:00:00:be:af",
			),
		)

		It("should not override other interfaces", func() {
			ifaces := []vmschema.Interface{
				{Name: "vdpa-iface", Binding: &vmschema.PluginBinding{Name: "vdpa"}},
				{Name: "bridge-iface", InterfaceBindingMethod: vmschema.InterfaceBindingMethod{Bridge: &vmschema.InterfaceBridge{}}},
			}
			networks := []vmschema.Network{
				{Name: "vdpa-iface", NetworkSource: vmschema.NetworkSource{Multus: &vmschema.MultusNetwork{}}},
				{Name: "bridge-iface", NetworkSource: vmschema.NetworkSource{Multus: &vmschema.MultusNetwork{}}},
			}
			netInfo := newNetInfo("vdpa-iface", "/dev/vhost-vdpa-0", "")

			expectedDomainIface := &domainschema.Interface{
				Alias:  domainschema.NewUserDefinedAlias("vdpa-iface"),
				Type:   "vdpa",
				Source: domainschema.InterfaceSource{Device: slPath("vdpa-iface")},
				Model:  &domainschema.Model{Type: "virtio"},
			}

			testMutator, err := domain.NewVdpaNetworkConfigurator(ifaces, networks, netInfo, DEFAULT_CONT_NAME)
			Expect(err).ToNot(HaveOccurred())

			existingIface := &domainschema.Interface{Alias: domainschema.NewUserDefinedAlias("bridge-iface")}
			testDomSpec := &domainschema.DomainSpec{
				Devices: domainschema.Devices{
					Interfaces: []domainschema.Interface{*existingIface}}}

			mutatedDomSpec, err := testMutator.Mutate(testDomSpec)
			Expect(err).ToNot(HaveOccurred())
			Expect(mutatedDomSpec.Devices.Interfaces).To(Equal([]domainschema.Interface{*existingIface, *expectedDomainIface}))
		})

		It("should set domain interface correctly when executed more than once", func() {
			ifaces := []vmschema.Interface{{Name: "vdpa-idempotent", Binding: &vmschema.PluginBinding{Name: "vdpa"}}}
			networks := []vmschema.Network{{Name: "vdpa-idempotent", NetworkSource: vmschema.NetworkSource{Multus: &vmschema.MultusNetwork{}}}}
			netInfo := newNetInfo("vdpa-idempotent", "/dev/vhost-vdpa-0", "")

			expectedDomainIface := &domainschema.Interface{
				Alias:  domainschema.NewUserDefinedAlias("vdpa-idempotent"),
				Type:   "vdpa",
				Source: domainschema.InterfaceSource{Device: slPath("vdpa-idempotent")},
				Model:  &domainschema.Model{Type: "virtio"},
				MAC:    nil,
			}

			testMutator, err := domain.NewVdpaNetworkConfigurator(ifaces, networks, netInfo, DEFAULT_CONT_NAME)
			Expect(err).ToNot(HaveOccurred())

			firstResult, err := testMutator.Mutate(&domainschema.DomainSpec{})

			Expect(err).ToNot(HaveOccurred())
			Expect(firstResult.Devices.Interfaces).To(Equal([]domainschema.Interface{*expectedDomainIface}))

			secondResult, err := testMutator.Mutate(firstResult)
			Expect(err).ToNot(HaveOccurred())
			Expect(secondResult).To(Equal(firstResult))
		})

		It("should correctly pair multiple interfaces with MAC and vhost-vdpa path from netInfo", func() {
			netInfo := &downwardapi.NetworkInfo{
				Interfaces: []downwardapi.Interface{
					{
						Network: "vdpa-net-1",
						DeviceInfo: &networkv1.DeviceInfo{
							Type: networkv1.DeviceInfoTypeVDPA,
							Vdpa: &networkv1.VdpaDevice{Path: "/dev/vhost-vdpa-0"},
						},
						Mac: "aa:bb:cc:dd:ee:01",
					},
					{
						Network: "vdpa-net-2",
						DeviceInfo: &networkv1.DeviceInfo{
							Type: networkv1.DeviceInfoTypeVDPA,
							Vdpa: &networkv1.VdpaDevice{Path: "/dev/vhost-vdpa-1"},
						},
						Mac: "aa:bb:cc:dd:ee:02",
					},
				},
			}

			ifaces := []vmschema.Interface{
				{Name: "vdpa-net-1", Binding: &vmschema.PluginBinding{Name: "vdpa"}},
				{Name: "vdpa-net-2", Binding: &vmschema.PluginBinding{Name: "vdpa"}},
			}
			networks := []vmschema.Network{
				{Name: "vdpa-net-1", NetworkSource: vmschema.NetworkSource{Multus: &vmschema.MultusNetwork{}}},
				{Name: "vdpa-net-2", NetworkSource: vmschema.NetworkSource{Multus: &vmschema.MultusNetwork{}}},
			}

			expectedDomainIfaces := []domainschema.Interface{
				{
					Alias:  domainschema.NewUserDefinedAlias("vdpa-net-1"),
					Type:   "vdpa",
					Source: domainschema.InterfaceSource{Device: slPath("vdpa-net-1")},
					Model:  &domainschema.Model{Type: "virtio"},
					MAC:    &domainschema.MAC{MAC: "aa:bb:cc:dd:ee:01"},
				},
				{
					Alias:  domainschema.NewUserDefinedAlias("vdpa-net-2"),
					Type:   "vdpa",
					Source: domainschema.InterfaceSource{Device: slPath("vdpa-net-2")},
					Model:  &domainschema.Model{Type: "virtio"},
					MAC:    &domainschema.MAC{MAC: "aa:bb:cc:dd:ee:02"},
				},
			}

			testMutator, err := domain.NewVdpaNetworkConfigurator(ifaces, networks, netInfo, DEFAULT_CONT_NAME)
			Expect(err).ToNot(HaveOccurred())

			mutatedDomSpec, err := testMutator.Mutate(&domainschema.DomainSpec{})
			Expect(err).ToNot(HaveOccurred())
			Expect(mutatedDomSpec.Devices.Interfaces).To(Equal(expectedDomainIfaces))
		})

		It("should fail when not all vdpa interfaces are found in netInfo", func() {
			ifaces := []vmschema.Interface{
				{Name: "vdpa-net-1", Binding: &vmschema.PluginBinding{Name: "vdpa"}},
				{Name: "vdpa-net-2", Binding: &vmschema.PluginBinding{Name: "vdpa"}},
			}
			networks := []vmschema.Network{
				{Name: "vdpa-net-1", NetworkSource: vmschema.NetworkSource{Multus: &vmschema.MultusNetwork{}}},
				{Name: "vdpa-net-2", NetworkSource: vmschema.NetworkSource{Multus: &vmschema.MultusNetwork{}}},
			}

			netInfo := newNetInfo("vdpa-net-1", "/dev/vhost-vdpa-0", "")

			_, err := domain.NewVdpaNetworkConfigurator(ifaces, networks, netInfo, DEFAULT_CONT_NAME)
			Expect(err).To(HaveOccurred())
		})

		It("should set vdpa path correctly if after network-info update", func() {
			ifaces := []vmschema.Interface{{Name: "vdpa-path-update", Binding: &vmschema.PluginBinding{Name: "vdpa"}}}
			networks := []vmschema.Network{{Name: "vdpa-path-update", NetworkSource: vmschema.NetworkSource{Multus: &vmschema.MultusNetwork{}}}}

			netInfo1 := newNetInfo("vdpa-path-update", "/dev/vhost-vdpa-1", "")
			expectedDomainIface1 := &domainschema.Interface{
				Alias:  domainschema.NewUserDefinedAlias("vdpa-path-update"),
				Type:   "vdpa",
				Source: domainschema.InterfaceSource{Device: slPath("vdpa-path-update")},
				Model:  &domainschema.Model{Type: "virtio"},
				MAC:    nil,
			}

			netInfo2 := newNetInfo("vdpa-path-update", "/dev/vhost-vdpa-2", "")

			testMutator1, err := domain.NewVdpaNetworkConfigurator(ifaces, networks, netInfo1, DEFAULT_CONT_NAME)
			Expect(err).ToNot(HaveOccurred())
			result1, err := testMutator1.Mutate(&domainschema.DomainSpec{})
			Expect(err).ToNot(HaveOccurred())
			Expect(result1.Devices.Interfaces).To(Equal([]domainschema.Interface{*expectedDomainIface1}))

			testMutator2, err := domain.NewVdpaNetworkConfigurator(ifaces, networks, netInfo2, DEFAULT_CONT_NAME)
			Expect(err).ToNot(HaveOccurred())
			result2, err := testMutator2.Mutate(result1)
			Expect(err).ToNot(HaveOccurred())
			// Even if /dev/vhost-vdpa-* path changes in networkinfo,
			// the symlink path should remain the same
			Expect(result2.Devices.Interfaces).To(Equal([]domainschema.Interface{*expectedDomainIface1}))
		})

		It("should assign guest pci addresses correctly if one of them is empty", func() {
			netInfo := &downwardapi.NetworkInfo{
				Interfaces: []downwardapi.Interface{
					{
						Network: "vdpa-net-1",
						DeviceInfo: &networkv1.DeviceInfo{
							Type: networkv1.DeviceInfoTypeVDPA,
							Vdpa: &networkv1.VdpaDevice{Path: "/dev/vhost-vdpa-0"},
						},
					},
					{
						Network: "vdpa-net-2",
						DeviceInfo: &networkv1.DeviceInfo{
							Type: networkv1.DeviceInfoTypeVDPA,
							Vdpa: &networkv1.VdpaDevice{Path: "/dev/vhost-vdpa-1"},
						},
					},
				},
			}

			ifaces := []vmschema.Interface{
				{Name: "vdpa-net-1", Binding: &vmschema.PluginBinding{Name: "vdpa"}, PciAddress: "0000:65:00.1"},
				{Name: "vdpa-net-2", Binding: &vmschema.PluginBinding{Name: "vdpa"}},
			}
			networks := []vmschema.Network{
				{Name: "vdpa-net-1", NetworkSource: vmschema.NetworkSource{Multus: &vmschema.MultusNetwork{}}},
				{Name: "vdpa-net-2", NetworkSource: vmschema.NetworkSource{Multus: &vmschema.MultusNetwork{}}},
			}

			expectedDomainIfaces := []domainschema.Interface{
				{
					Alias:  domainschema.NewUserDefinedAlias("vdpa-net-1"),
					Type:   "vdpa",
					Source: domainschema.InterfaceSource{Device: slPath("vdpa-net-1")},
					Model:  &domainschema.Model{Type: "virtio"},
					Address: &domainschema.Address{
						Type:     "pci",
						Domain:   "0x0000",
						Bus:      "0x65",
						Slot:     "0x00",
						Function: "0x1",
					},
				},
				{
					Alias:  domainschema.NewUserDefinedAlias("vdpa-net-2"),
					Type:   "vdpa",
					Source: domainschema.InterfaceSource{Device: slPath("vdpa-net-2")},
					Model:  &domainschema.Model{Type: "virtio"},
				},
			}

			testMutator, err := domain.NewVdpaNetworkConfigurator(ifaces, networks, netInfo, DEFAULT_CONT_NAME)
			Expect(err).ToNot(HaveOccurred())

			mutatedDomSpec, err := testMutator.Mutate(&domainschema.DomainSpec{})
			Expect(err).ToNot(HaveOccurred())
			Expect(mutatedDomSpec.Devices.Interfaces).To(Equal(expectedDomainIfaces))
		})
	})

	Context("GetDownwardAPINetworkInfo file handling", func() {
		var (
			tempDir         string
			networkInfoPath string
		)

		BeforeEach(func() {
			tempDir = GinkgoT().TempDir()
			networkInfoPath = filepath.Join(tempDir, "network-info")
		})

		It("should read and unmarshal network-info file", func() {
			expectedNetworkInfo := &downwardapi.NetworkInfo{
				Interfaces: []downwardapi.Interface{{
					Network: "vdpa-net",
					Mac:     "02:02:02:02:02:02",
					DeviceInfo: &networkv1.DeviceInfo{
						Type: networkv1.DeviceInfoTypeVDPA,
						Vdpa: &networkv1.VdpaDevice{Path: "/dev/vhost-vdpa-0"},
					},
				}},
			}
			b, err := json.Marshal(expectedNetworkInfo)
			Expect(err).ToNot(HaveOccurred())
			Expect(os.WriteFile(networkInfoPath, b, 0644)).To(Succeed())

			actualNetworkInfo, err := domain.GetDownwardAPINetworkInfo(networkInfoPath)
			Expect(err).ToNot(HaveOccurred())
			Expect(actualNetworkInfo).To(Equal(expectedNetworkInfo))
		})

		It("should return an error for invalid JSON content", func() {
			Expect(os.WriteFile(networkInfoPath, []byte("{invalid"), 0644)).To(Succeed())
			_, err := domain.GetDownwardAPINetworkInfo(networkInfoPath)
			Expect(err).To(HaveOccurred())
		})

		It("should return an error if the file does not exist", func() {
			_, err := domain.GetDownwardAPINetworkInfo(filepath.Join(tempDir, "does-not-exist"))
			Expect(err).To(HaveOccurred())
		})
	})
})
