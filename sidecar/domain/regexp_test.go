package domain_test

import (
	"testing"

	"kubevirt.io/kubevirt/cmd/sidecars/network-vdpa-binding/domain"
)

// TestHelloName calls greetings.Hello with a name, checking
// for a valid return value.
func TestVdpaDevice(t *testing.T) {
	vdpaDevice := domain.ExtractVdpaDevice("{\"0000:65:00.2\":{\"generic\":{\"deviceID\":\"0000:65:00.2\"},\"vdpa\":{\"mount\":\"/dev/vhost-vdpa-0\"}}}")

	if vdpaDevice != "/dev/vhost-vdpa-0" {
		t.Fatal("not good")
	}

	vdpaDevice = domain.ExtractVdpaDevice("{\"0000:65:00.3\":{\"generic\":{\"deviceID\":\"0000:65:00.3\"},\"vdpa\":{\"mount\":\"/dev/vhost-vdpa-1\"}}}")

	if vdpaDevice != "/dev/vhost-vdpa-1" {
		t.Fatal("not good")
	}
}
