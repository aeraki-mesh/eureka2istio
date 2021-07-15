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

package main

import (
	"flag"
	watcher "github.com/aeraki-framework/eureka2istio/pkg/springcloud/eureka/watcher"
	eureka "github.com/huanghuangzym/eureka-client"
	istioclient "istio.io/client-go/pkg/clientset/versioned"
	"istio.io/pkg/log"
	"os"
	"os/signal"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"strings"
	"syscall"
)

const (
	defaultEurekaAddr = "http://10.244.0.16:8761/eureka/,http://10.244.0.17:8761/eureka/"
	defaultEurekaMode = "simple"
)

func main() {
	var ekAddr string
	var mode string
	flag.StringVar(&ekAddr, "ekaddr", defaultEurekaAddr, "eureka address")
	flag.StringVar(&mode, "mode", defaultEurekaMode, "eureka mode")
	flag.Parse()

	hosts := strings.Split(ekAddr, ",")
	if len(hosts) == 0 || hosts[0] == "" {
		log.Errorf("please specify eureka address")
		return
	}

	clientMap := make(map[string]*eureka.Client)

	if mode == defaultEurekaMode {
		//if in simple mode,we do not watch any crd, only read the commandline eureka address
		for _, host := range hosts {
			client := eureka.NewClient(&eureka.Config{
				DefaultZone:           host,
				App:                   "eureka2istio",
				Port:                  10000,
				RenewalIntervalInSecs: 10,
				DurationInSecs:        30,
				Metadata: map[string]interface{}{
					"VERSION":              "0.1.0",
					"NODE_GROUP_ID":        0,
					"PRODUCT_CODE":         "DEFAULT",
					"PRODUCT_VERSION_CODE": "DEFAULT",
					"PRODUCT_ENV_CODE":     "DEFAULT",
					"SERVICE_VERSION_CODE": "DEFAULT",
				},
			})
			// start client, register、heartbeat、refresh
			err := client.Connect()
			if err != nil {
				log.Errorf("failed to connect to eureka server %s: %v", ekAddr, err)
				return
			}
			go client.Refresh()
			go client.Heartbeat()
			clientMap[host] = client
		}
	} else {
		//we will watch eureka instance crd to generate rules

	}

	ic, err := getIstioClient()
	if err != nil {
		log.Errorf("failed to create istio client: %v", err)
		return
	}

	stopChan := make(chan struct{}, 1)

	for _, client := range clientMap {
		serviceWatcher := watcher.NewServiceWatcher(client, ic, "eureka")
		go serviceWatcher.Run(stopChan)
	}

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	log.Info("wait for stop chan")
	<-signalChan
	log.Info("receive stop chan for stop chan")
	for _, client := range clientMap {
		client.Running = false
		client.DoUnRegister()
	}

	stopChan <- struct{}{}
}

func getIstioClient() (*istioclient.Clientset, error) {
	config, err := config.GetConfig()
	if err != nil {
		return nil, err
	}

	ic, err := istioclient.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	return ic, nil
}
