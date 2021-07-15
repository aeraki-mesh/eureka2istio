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

package eureka

import (
	"context"
	"encoding/json"
	"time"

	"github.com/aeraki-framework/eureka2istio/pkg/springcloud/eureka/model"

	eureka "github.com/huanghuangzym/eureka-client"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	istioclient "istio.io/client-go/pkg/clientset/versioned"
	"istio.io/pkg/log"
	"k8s.io/apimachinery/pkg/api/errors"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// aerakiFieldManager is the FileldManager for Aeraki CRDs
	aerakiFieldManager = "aeraki"

	// debounceAfter is the delay added to events to wait after a registry event for debouncing.
	// This will delay the push by at least this interval, plus the time getting subsequent events.
	// If no change is detected the push will happen, otherwise we'll keep delaying until things settle.
	debounceAfter = 500 * time.Millisecond

	// debounceMax is the maximum time to wait for events while debouncing.
	// Defaults to 10 seconds. If events keep showing up with no break for this time, we'll trigger a push.
	debounceMax = 10 * time.Second

	// the maximum retries if failed to sync dubbo services to Istio
	maxRetries = 10
)

// ProviderWatcher watches changes on dubbo service providers and synchronize the changed dubbo providers to the Istio
// control plane via service entries
type ProviderWatcher struct {
	conn           *eureka.Client
	ic             *istioclient.Clientset
	serviceEntryNS map[string]string //key service entry name, value namespace
	eurekaName     string
}

// NewWatcher creates a ProviderWatcher
func NewProviderWatcher(ic *istioclient.Clientset, conn *eureka.Client, ekName string) *ProviderWatcher {
	return &ProviderWatcher{
		conn:           conn,
		ic:             ic,
		serviceEntryNS: make(map[string]string),
		eurekaName:     ekName,
	}
}

// Run starts the ProviderWatcher until it receives a message over the stop chanel
// This method blocks the caller
func (w *ProviderWatcher) Run(stop <-chan struct{}) {
	if w.conn.Applications == nil {
		return
	}
	w.syncServices2IstioUntilMaxRetries(w.conn.Applications.Applications)
}

func (w *ProviderWatcher) syncServices2IstioUntilMaxRetries(applications []eureka.Application) {
	if len(applications) == 0 {
		log.Warn("eureka has no regist services providers, ignore synchronize job")
		return
	}

	for _, app := range applications {
		if app.Name == "EUREKA" || app.Name == "EUREKA2ISTIO" {
			continue
		}

		log.Infof("get app %v", app)

		serviceEntries, err := model.ConvertServiceEntry(w.eurekaName, app.Name, app.Instances)
		if err != nil {
			log.Errorf("Failed to synchronize dubbo services to Istio: %v", err)
		}

		for _, new := range serviceEntries {
			err := w.syncService2Istio(new)
			retries := 0
			for err != nil {
				if isRetryableError(err) && retries < maxRetries {
					log.Errorf("Failed to synchronize dubbo services to Istio, error: %v,  retrying %v ...", err, retries)
					err = w.syncService2Istio(new)
					retries++
				} else {
					log.Errorf("Failed to synchronize dubbo services to Istio: %v", err)
					err = nil
				}
			}
		}

	}

}

func (w *ProviderWatcher) syncService2Istio(new *v1alpha3.ServiceEntry) error {
	// delete old service entry if multiple service entries found in different namespaces.
	// Aeraki doesn't support deploying providers of the same dubbo interface in multiple namespaces because interface
	// is used as the global dns name for dubbo service across the whole mesh
	if oldNS, exist := w.serviceEntryNS[new.Name]; exist {
		if oldNS != new.Namespace {
			log.Errorf("found service entry %s in two namespaces : %s %s ,delete the older one %s/%s", new.Name, oldNS,
				new.Namespace, oldNS, new.Name)
			if err := w.ic.NetworkingV1alpha3().ServiceEntries(oldNS).Delete(context.TODO(), new.Name,
				metav1.DeleteOptions{}); err != nil {
				if isRealError(err) {
					log.Errorf("failed to delete service entry: %s/%s", oldNS, new.Name)
				}
			}
		}
	}

	existingServiceEntry, err := w.ic.NetworkingV1alpha3().ServiceEntries(new.Namespace).Get(context.TODO(), new.Name,
		metav1.GetOptions{},
	)

	if isRealError(err) {
		return err
	} else if isNotFound(err) {
		return w.createServiceEntry(new)
	} else {
		mergeServiceEntryEndpoints(w.eurekaName, new, existingServiceEntry)
		return w.updateServiceEntry(new, existingServiceEntry)
	}

	return nil
}

func (w *ProviderWatcher) createServiceEntry(serviceEntry *v1alpha3.ServiceEntry) error {
	_, err := w.ic.NetworkingV1alpha3().ServiceEntries(serviceEntry.Namespace).Create(context.TODO(), serviceEntry,
		metav1.CreateOptions{FieldManager: aerakiFieldManager})
	if err == nil {
		w.serviceEntryNS[serviceEntry.Name] = serviceEntry.Namespace
		log.Infof("service entry %s has been created: %s", serviceEntry.Name, struct2JSON(serviceEntry))
	}
	return err
}

func (w *ProviderWatcher) updateServiceEntry(new *v1alpha3.ServiceEntry,
	old *v1alpha3.ServiceEntry) error {
	new.Spec.Ports = old.Spec.Ports
	new.ResourceVersion = old.ResourceVersion
	_, err := w.ic.NetworkingV1alpha3().ServiceEntries(new.Namespace).Update(context.TODO(), new,
		metav1.UpdateOptions{})
	if err == nil {
		log.Infof("service entry %s has been updated: %s", new.Name, struct2JSON(new))
	} else {
		log.Errorf("service entry %s has been updated: %s get err %v", new.Name, struct2JSON(new), err)
	}
	return err
}

func isRealError(err error) bool {
	return err != nil && !errors.IsNotFound(err)
}

func isRetryableError(err error) bool {
	return errors.IsInternalError(err) || errors.IsResourceExpired(err) || errors.IsServerTimeout(err) ||
		errors.IsServiceUnavailable(err) || errors.IsTimeout(err) || errors.IsTooManyRequests(err) ||
		errors.ReasonForError(err) == metav1.StatusReasonUnknown
}

func isNotFound(err error) bool {
	return err != nil && errors.IsNotFound(err)
}

func struct2JSON(ojb interface{}) interface{} {
	b, err := json.Marshal(ojb)
	if err != nil {
		return ojb
	}
	return string(b)
}

func mergeServiceEntryEndpoints(ekName string, new *v1alpha3.ServiceEntry, old *v1alpha3.ServiceEntry) error {
	if old == nil {
		return nil
	}
	endpoints := new.Spec.Endpoints
	for _, ep := range old.Spec.Endpoints {
		if ep.Labels["eurekaName"] != ekName {
			endpoints = append(endpoints, ep)
		}
	}
	new.Spec.Endpoints = endpoints

	return nil
}
