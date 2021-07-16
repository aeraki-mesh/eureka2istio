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
	"time"

	eureka "github.com/huanghuangzym/eureka-client"
	istioclient "istio.io/client-go/pkg/clientset/versioned"
	"istio.io/pkg/log"
)

// ServiceWatcher watches for newly created spring cloud eureka services and creates a providerWatcher for each service
type ServiceWatcher struct {
	conn             *eureka.Client
	ic               *istioclient.Clientset
	providerWatchers map[string]*ProviderWatcher
	eurekaName       string
}

// NewWatcher creates a ServiceWatcher
func NewServiceWatcher(conn *eureka.Client, clientset *istioclient.Clientset, eurekaName string) *ServiceWatcher {
	return &ServiceWatcher{
		ic:               clientset,
		conn:             conn,
		providerWatchers: make(map[string]*ProviderWatcher),
		eurekaName:       eurekaName,
	}
}

// Run starts the ServiceWatcher until it receives a message over the stop chanel
// This method blocks the caller
func (w *ServiceWatcher) Run(stop <-chan struct{}) {
	tickTimer := time.NewTicker(10 * time.Second)
	w.watchProviders(stop)
	for {
		select {
		case <-tickTimer.C:
			log.Infof("received time ticker :  %d ", len(w.conn.Applications.Applications))
			w.watchProviders(stop)
		case <-stop:
			log.Info("recieve stop chan,stoped")
			return
		}
	}
}

func (w *ServiceWatcher) watchProviders(stop <-chan struct{}) {

	providerWatcher := NewProviderWatcher(w.ic, w.conn, w.eurekaName)
	log.Infof("start to refresh service %s on eureka", w.eurekaName)
	go providerWatcher.Run(stop)

	return
}
