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

package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"kubevirt.io/client-go/log"

	"kubevirt.io/vdpa-network-binding-plugin/webhook/cert"
	"kubevirt.io/vdpa-network-binding-plugin/webhook/mutate"
)

const (
	webhookName = "kubevirt-vdpa-mutating-webhook"
	webhookPath = "/mutate-vdpa"
	healthzPath = "/healthz"
)

func main() {
	log.InitializeLogging(webhookName)

	port := flag.Int("port", 8443, "Webhook listen port")
	svcName := flag.String("service-name", webhookName, "Webhook service name")
	svcNamespace := flag.String("namespace", "kubevirt", "Webhook service namespace")
	flag.Parse()

	certMgr, err := cert.NewCertManager(*svcName, *svcNamespace, webhookName)
	if err != nil {
		log.Log.Reason(err).Errorf("Failed to initialize certificate manager")
		os.Exit(1)
	}

	renewalCtx, stopRenewal := context.WithCancel(context.Background())
	defer stopRenewal()
	go certMgr.RunRenewalLoop(renewalCtx)

	mux := http.NewServeMux()
	mux.HandleFunc(webhookPath, mutate.HandleMutateVDPA)
	mux.HandleFunc(healthzPath, handleHealthz)

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", *port),
		Handler: mux,
		TLSConfig: &tls.Config{
			GetCertificate: certMgr.GetCertificate,
		},
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Log.Infof("Starting vDPA mutating webhook on %s", srv.Addr)
		if err := srv.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
			log.Log.Reason(err).Errorf("Webhook server failed")
		}
	}()

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	<-signalChan

	log.Log.Info("Shutting down gracefully...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Log.Reason(err).Error("Server shutdown error")
	}
	log.Log.Info("Server stopped")
}

func handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}
