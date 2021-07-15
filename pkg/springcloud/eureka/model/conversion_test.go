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
	"testing"
)

func Test_parseProvider(t *testing.T) {

	//attributes := parseProvider(dubboProvider)
	attributes := make(map[string]string)
	if attributes["ip"] != "172.18.0.9" {
		t.Errorf("parseProvider ip => %v, want %v", attributes["ip"], "172.18.0.9")
	}
	if attributes["port"] != "20880" {
		t.Errorf("parseProvider port => %v, want %v", attributes["port"], "20880")
	}
	if attributes["interface"] != "org.apache.dubbo.samples.basic.api.DemoService" {
		t.Errorf("parseProvider port => %v, want %v", attributes["interface"], "org.apache.dubbo.samples.basic.api.DemoService")
	}
}

func Test_isValidLabel(t *testing.T) {
	tests := []struct {
		key   string
		value string
		want  bool
	}{
		{
			key:   "method",
			value: "testVoid%2CsayHello",
			want:  true,
		},
		{
			key:   "interface",
			value: "org.apache.dubbo.samples.basic.api.DemoService",
			want:  false,
		},
		{
			key:   "interface",
			value: "org.apache_dubbo-samples.basic.api.DemoService",
			want:  false,
		},
	}
	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			if got := isInvalidLabel(tt.key, tt.value); got != tt.want {
				t.Errorf("isValidLabel() = %v, want %v", got, tt.want)
			}
		})
	}
}
