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
 * Copyright The KubeVirt Authors.
 *
 */

package cert

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"sync"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"kubevirt.io/client-go/log"

	"kubevirt.io/kubevirt/pkg/certificates/triple"
	"kubevirt.io/kubevirt/pkg/certificates/triple/cert"
)

const (
	certDuration    = 365 * 24 * time.Hour                       // certificate validity period (1 year)
	certRenewBefore = time.Duration(float64(certDuration) * 0.2) // renew when 20% of validity remains
	retryInterval   = 1 * time.Minute                            // retry delay after renewal failure
)

type certManager struct {
	mutex        sync.RWMutex
	currentCert  *tls.Certificate
	caCertPEM    []byte
	svcName      string
	svcNamespace string
	clientset    kubernetes.Interface
	webhookName  string
}

func NewCertManager(serviceName, serviceNamespace, webhookName string) (*certManager, error) {
	clusterConfig, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("not running in cluster: %v", err)
	}
	clientset, err := kubernetes.NewForConfig(clusterConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes clientset: %v", err)
	}

	cm := &certManager{
		svcName:      serviceName,
		svcNamespace: serviceNamespace,
		clientset:    clientset,
		webhookName:  webhookName,
	}

	if err := cm.renewCertificates(); err != nil {
		return nil, fmt.Errorf("initial certificate generation failed: %v", err)
	}
	return cm, nil
}

// Go's TLS stack calls this whenever a client opens a new HTTPS connection.
// We return whatever cert we're serving right now (after a renewal, callers get the new one).
func (cm *certManager) GetCertificate(_ *tls.ClientHelloInfo) (*tls.Certificate, error) {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()
	return cm.currentCert, nil
}

func (cm *certManager) renewCertificates() error {
	caCertPEM, tlsCert, err := generateCertificates(cm.svcName, cm.svcNamespace)
	if err != nil {
		return fmt.Errorf("failed to generate certificates: %v", err)
	}

	cm.mutex.Lock()
	defer cm.mutex.Unlock()
	if err := patchWebhookCABundle(cm.clientset, caCertPEM, cm.webhookName); err != nil {
		return fmt.Errorf("failed to patch caBundle: %v", err)
	}

	cm.currentCert = &tlsCert
	cm.caCertPEM = caCertPEM

	log.Log.Infof("Certificates renewed successfully, new expiry: %s", tlsCert.Leaf.NotAfter.Format(time.RFC3339))
	return nil
}

func (cm *certManager) nextRenewalDeadline() time.Time {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()
	if cm.currentCert == nil || cm.currentCert.Leaf == nil {
		return time.Now()
	}
	return cm.currentCert.Leaf.NotAfter.Add(-certRenewBefore)
}

// Background goroutine: sleep until it's time to renew, then call renewCertificates().
// If that fails, wait a bit and try again. Stops when the process is shutting down.
func (cm *certManager) RunRenewalLoop(ctx context.Context) {
	for {
		deadline := cm.nextRenewalDeadline()
		timeUntilRenewal := time.Until(deadline)
		if timeUntilRenewal < 0 {
			timeUntilRenewal = 0
		}

		log.Log.Infof("Next certificate renewal scheduled for %s (%s from now)", deadline.Format(time.RFC3339), timeUntilRenewal)

		renewalTimer := time.NewTimer(timeUntilRenewal)

		select {
		case <-renewalTimer.C:
			log.Log.Info("Certificate renewal deadline reached, renewing certificates...")
			if err := cm.renewCertificates(); err != nil {
				log.Log.Reason(err).Errorf("Certificate renewal failed, will retry in %s", retryInterval)
				select {
				case <-time.After(retryInterval):
				case <-ctx.Done():
					return
				}
			}
		case <-ctx.Done():
			renewalTimer.Stop()
			log.Log.Info("Certificate renewal loop stopped due to shutdown")
			return
		}
	}
}

func generateCertificates(svcName, svcNamespace string) (caCertPEM []byte, tlsCert tls.Certificate, err error) {
	caKeyPair, err := triple.NewCA("vdpa-webhook.kubevirt.io", certDuration)
	if err != nil {
		return nil, tls.Certificate{}, fmt.Errorf("failed to create CA: %v", err)
	}
	caCertPEM = cert.EncodeCertPEM(caKeyPair.Cert)

	serverKeyPair, err := triple.NewServerKeyPair(
		caKeyPair,
		fmt.Sprintf("%s.%s.pod.cluster.local", svcName, svcNamespace),
		svcName,
		svcNamespace,
		"cluster.local",
		nil,
		nil,
		certDuration,
	)
	if err != nil {
		return nil, tls.Certificate{}, fmt.Errorf("failed to create server cert: %v", err)
	}

	serverCertPEM := cert.EncodeCertPEM(serverKeyPair.Cert)
	serverKeyPEM := cert.EncodePrivateKeyPEM(serverKeyPair.Key)

	tlsCert, err = tls.X509KeyPair(serverCertPEM, serverKeyPEM)
	if err != nil {
		return nil, tls.Certificate{}, fmt.Errorf("failed to load TLS key pair: %v", err)
	}

	tlsCert.Leaf, err = x509.ParseCertificate(tlsCert.Certificate[0])
	if err != nil {
		return nil, tls.Certificate{}, fmt.Errorf("failed to parse leaf certificate: %v", err)
	}

	return caCertPEM, tlsCert, nil
}
