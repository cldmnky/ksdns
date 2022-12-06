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
	"fmt"
	"net"

	rfc1035v1alpha1 "github.com/cldmnky/ksdns/pkg/zupd/api/v1alpha1"
	"github.com/miekg/dns"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// TypeAvailableKsdns represents the status of the Deployment reconciliation
	TypeAvailableKsdns = "Available"
	// TypeAvailableKsdns represents the status used when the custom resource is deleted and the finalizer operations are must to occur.
	TypeDegradedKsdns = "Degraded"
	nsName            = "ns.dns"
)

// KsdnsSpec defines the desired state of Ksdns
type KsdnsSpec struct {
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	Zones   []Zone  `json:"zones,omitempty"`
	CoreDNS CoreDNS `json:"coredns,omitempty"`
	// +kubebuilder:validation:Optional
	Secret *corev1.LocalObjectReference `json:"secret,omitempty"`
}

type CoreDNS struct {
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// +kubebuilder:validation:Optional
	// +kubebuilder:default:="quay.io/ksdns/zupd:latest"
	Image           string                      `json:"image,omitempty"`
	ImagePullPolicy corev1.PullPolicy           `json:"imagePullPolicy,omitempty"`
	Resources       corev1.ResourceRequirements `json:"resources,omitempty"`
	NodeSelector    map[string]string           `json:"nodeSelector,omitempty"`
	Tolerations     []corev1.Toleration         `json:"tolerations,omitempty"`
	Affinity        *corev1.Affinity            `json:"affinity,omitempty"`
	// +kubebuilder:validation:Optional
	// +kubebuilder:default:=2
	Replicas int32 `json:"replicas,omitempty"`
}

type Zone struct {
	Origin  string   `json:"origin,omitempty"`
	Records []Record `json:"records,omitempty"`
}

func (z *Zone) ToRfc1035Zone(nsIP net.IP) (*rfc1035v1alpha1.ZoneSpec, error) {
	if z.Origin == "" {
		return nil, fmt.Errorf("origin cannot be empty")
	}
	soa, err := newSOARecord(z.Origin)
	if err != nil {
		return nil, err
	}
	ns, extra, err := newNSRecord(z.Origin, nsIP)
	if err != nil {
		return nil, err
	}
	records := append(soa, ns...)
	records = append(records, extra...)
	var rfc1035Zone string
	for _, record := range records {
		// concatenate all records into a single string
		rfc1035Zone += fmt.Sprintf("%s\n", record.String())
	}
	for _, record := range z.Records {
		r, err := record.String()
		if err != nil {
			return nil, err
		}
		rfc1035Zone += fmt.Sprintf("%s\n", r)
	}
	return &rfc1035v1alpha1.ZoneSpec{
		Zone: rfc1035Zone,
	}, nil
}

type Record struct {
	// +kubebuilder:validation:Required
	Name string `json:"name,omitempty"`
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=A;CNAME;SRV;TXT
	Type string `json:"type,omitempty"`
	// optional
	// +kubebuilder:validation:Optional
	// default is 30
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=2147483647
	// +kubebuilder:default=30
	TTL int `json:"ttl,omitempty"`
	// required if SRV
	// +kubebuilder:validation:Optional
	Priority uint16 `json:"priority,omitempty"`
	// required if SRV
	// +kubebuilder:validation:Optional
	Weight uint16 `json:"weight,omitempty"`
	// required if SRV
	// +kubebuilder:validation:Optional
	Port uint16 `json:"port,omitempty"`
	// +kubebuilder:validation:Optional
	Target string `json:"data,omitempty"`
	// Required if TXT
	// +kubebuilder:validation:Optional
	Text string `json:"text,omitempty"`
}

func (r *Record) String() (string, error) {
	var rr dns.RR
	switch r.Type {
	case "A":
		rr = newA(r.Name, uint32(r.TTL), net.ParseIP(r.Target))
	case "CNAME":
		rr = newCNAME(r.Name, uint32(r.TTL), r.Target)
	case "SRV":
		rr = newSRV(r.Name, uint32(r.TTL), r.Target, r.Weight, r.Priority, r.Port)
	case "TXT":
		rr = newTXT(r.Name, uint32(r.TTL), r.Text)
	default:
		return "", fmt.Errorf("unsupported record type: %s", r.Type)
	}

	//rr, err := dns.NewRR(fmt.Sprintf("%s %d %s %s %s", r.Name, r.TTL, r.Class, r.Type, r.Target))

	return rr.String(), nil
}

// KsdnsStatus defines the observed state of Ksdns
type KsdnsStatus struct {
	// +operator-sdk:csv:customresourcedefinitions:type=status
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Ksdns is the Schema for the ksdns API
type Ksdns struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KsdnsSpec   `json:"spec,omitempty"`
	Status KsdnsStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// KsdnsList contains a list of Ksdns
type KsdnsList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Ksdns `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Ksdns{}, &KsdnsList{})
}
