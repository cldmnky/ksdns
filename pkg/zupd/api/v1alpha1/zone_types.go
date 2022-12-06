/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	"github.com/coredns/coredns/plugin/file"
	"github.com/miekg/dns"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ZoneSpec defines the desired state of Zone
type ZoneSpec struct {
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	Zone string `json:"zone,omitempty"`
}

func (zs *ZoneSpec) GetZone() string {
	return zs.Zone
}

// ZoneStatus defines the observed state of Zone
type ZoneStatus struct {
	// +operator-sdk:csv:customresourcedefinitions:type=status
	DynamicRRs []DynamicRR `json:"dynamicRRs,omitempty"`
	Serial     uint32      `json:"serial,omitempty"`
}

func (zs *ZoneStatus) GetDynamicRRs() []DynamicRR {
	return zs.DynamicRRs
}

// AddDynamicRR to ZoneStatus
func (zs *ZoneStatus) SetDynamicRRs(rrs []dns.RR) {
	if zs.DynamicRRs == nil {
		zs.DynamicRRs = make([]DynamicRR, 0)
	}
	newRRs := make([]DynamicRR, 0)
	for _, rr := range rrs {
		newRRs = append(newRRs, DynamicRR{
			RR: rr.String(),
		})
	}
	zs.DynamicRRs = newRRs
}

func (zs *ZoneStatus) GetDynamicRRsAsZone(name string) *file.Zone {
	zone := file.NewZone(name, "")
	for _, rrString := range zs.DynamicRRs {
		if rr, err := dns.NewRR(rrString.RR); err != nil {
			continue
		} else {
			if err := zone.Insert(rr); err != nil {
				continue
			}
		}
	}
	if len(zone.All()) == 0 {
		return nil
	}
	return zone
}

type DynamicRR struct {
	RR string `json:"rr,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Zone is the Schema for the zones API
type Zone struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ZoneSpec   `json:"spec,omitempty"`
	Status ZoneStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ZoneList contains a list of Zone
type ZoneList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Zone `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Zone{}, &ZoneList{})
}
