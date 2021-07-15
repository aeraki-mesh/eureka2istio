// Copyright Aeraki Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package model

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	eureka "github.com/huanghuangzym/eureka-client"
	istio "istio.io/api/networking/v1alpha3"
	"istio.io/pkg/log"
)

const (
	eurekaPortName = "http"

	eurekaRegistry = "spring-eureka"

	defaultServiceAccount = "default"

	eurekaNameLabel = "eurekaName"

	// aerakiFieldManager is the FileldManager for Aeraki CRDs
	asmFieldManager = "asm"
)

var labelRegexp *regexp.Regexp

func init() {
	labelRegexp = regexp.MustCompile("^(([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9])?$")
}

// ConvertServiceEntry converts eureka app instance to a service entry
func ConvertServiceEntry(eurekaName, service string, eurekaInstances []eureka.Instance) (map[string]*v1alpha3.ServiceEntry, error) {
	serviceEntries := make(map[string]*v1alpha3.ServiceEntry)

	for _, provider := range eurekaInstances {
		eurekaAttributes := parseProvider(provider)
		eurekaApp := eurekaAttributes["eureka-app"]
		instanceIP := eurekaAttributes["ip"]
		instancePort := eurekaAttributes["port"]
		if eurekaApp == "" || instanceIP == "" || instancePort == "" {
			return nil, fmt.Errorf("failed to convert eureka instance to serviceEntry, parameters service, "+
				"host, ip or port is missing: %v", provider)
		}

		host := constructIGV(eurekaAttributes)

		if host == "" {
			log.Warn("host is nil")
			continue
		}
		err := ValidateFQDN(host)
		if err != nil {
			log.Warnf("host not valid: %s , err %v", host, err)
			continue
		}

		ns := "istio-system"
		// All the providers of a dubbo service should be deployed in the same namespace
		if se, exist := serviceEntries[host]; exist {
			if ns != se.Namespace {
				log.Errorf("found provider in multiple namespaces: %s %s, ignore provider %s", se.Namespace, ns, provider)
				continue
			}
		}

		port, err := strconv.Atoi(instancePort)
		if err != nil {
			log.Errorf("failed to convert dubbo port to int:  %s, ignore provider %s", instancePort, provider)
			continue
		}

		labels := eurekaAttributes
		delete(labels, "service")
		delete(labels, "ip")
		delete(labels, "port")
		delete(labels, "aeraki_meta_app_service_account")
		delete(labels, "aeraki_meta_app_namespace")
		delete(labels, "aeraki_meta_workload_selector")
		delete(labels, "aeraki_meta_locality")

		delete(labels, "aeraki_meta_app_version")
		for key, value := range labels {
			if isInvalidLabel(key, value) {
				delete(labels, key)
				log.Infof("drop invalid label: key %s, value: %s", key, value)
			}
		}
		// to distinguish endpoints from different zk clusters
		labels[eurekaNameLabel] = eurekaName
		serviceEntry, exist := serviceEntries[host]
		if !exist {
			serviceEntry = createServiceEntry(ns, host, eurekaApp)
			serviceEntries[host] = serviceEntry
		}
		serviceEntry.Spec.Endpoints = append(serviceEntry.Spec.Endpoints,
			createWorkloadEntry(instanceIP, uint32(port), labels))
	}

	log.Infof("the serviceEntries is %v ", serviceEntries)
	return serviceEntries, nil
}

func constructIGV(attributes map[string]string) string {

	if attributes["asm-hostname"] != "" {
		return attributes["asm-hostname"]
	} else if attributes["hostname"] != "" {
		return attributes["hostname"]
	}

	return ""
}

func createServiceEntry(namespace string, host string, eurekaApp string) *v1alpha3.ServiceEntry {
	spec := &istio.ServiceEntry{
		Hosts:      []string{host},
		Ports:      []*istio.Port{convertPort()},
		Resolution: istio.ServiceEntry_STATIC,
		Location:   istio.ServiceEntry_MESH_INTERNAL,
		Endpoints:  make([]*istio.WorkloadEntry, 0),
	}

	serviceEntry := &v1alpha3.ServiceEntry{
		ObjectMeta: v1.ObjectMeta{
			Name:      ConstructServiceEntryName(host),
			Namespace: namespace,
			Labels: map[string]string{
				"manager":  asmFieldManager,
				"registry": eurekaRegistry,
			},
			Annotations: map[string]string{
				"interface": eurekaApp,
			},
		},
		Spec: *spec,
	}
	return serviceEntry
}

func createWorkloadEntry(ip string, port uint32,
	labels map[string]string) *istio.WorkloadEntry {
	return &istio.WorkloadEntry{
		Address: ip,
		Ports:   map[string]uint32{eurekaPortName: port},
		//ServiceAccount: serviceAccount,
		//Locality:       locality,
		Labels: labels,
	}
}

func isInvalidLabel(key string, value string) bool {
	return !labelRegexp.MatchString(key) || !labelRegexp.MatchString(value) || len(key) > 63 || len(value) > 63
}

func convertPort() *istio.Port {
	return &istio.Port{
		//always use 80 ,because spring cloud default support http
		Number:   80,
		Protocol: "HTTP",
		Name:     eurekaPortName,
	}
}

func parseProvider(provider eureka.Instance) map[string]string {
	eurekaAttributes := make(map[string]string)

	eurekaAttributes["ip"] = provider.IPAddr
	eurekaAttributes["port"] = strconv.Itoa(provider.Port.Port)
	eurekaAttributes["hostname"] = provider.HostName
	eurekaAttributes["eureka-app"] = provider.App

	if provider.Metadata["asm-serviceentry"] != nil {
		eurekaAttributes["asm-hostname"] = (provider.Metadata["asm-serviceentry"]).(string)
	}

	return eurekaAttributes
}

// ConstructServiceEntryName constructs the service entry name for a given dubbo service
func ConstructServiceEntryName(service string) string {
	validDNSName := strings.ReplaceAll(strings.ToLower(service), ".", "-")
	return asmFieldManager + "-" + validDNSName
}
