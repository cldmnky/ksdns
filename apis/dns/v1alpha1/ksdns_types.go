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
	"time"

	rfc1035v1alpha1 "github.com/cldmnky/ksdns/pkg/zupd/api/v1alpha1"
	"github.com/coredns/coredns/plugin/pkg/dnsutil"
	"github.com/miekg/dns"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// KsdnsSpec defines the desired state of Ksdns
type KsdnsSpec struct {
	Zones []Zone `json:"zones,omitempty"`
}

type Zone struct {
	Origin  string   `json:"origin,omitempty"`
	Records []Record `json:"records,omitempty"`
}

func (z *Zone) ToRfc1035Zone(nsIP net.IP) (*rfc1035v1alpha1.Zone, error) {
	if z.Origin == "" {
		return nil, fmt.Errorf("origin cannot be empty")
	}
	soa, err := newSOARecord(z.Origin)
	if err != nil {
		return nil, err
	}
	ns, extra, err := NewNSRecord(z.Origin, nsIP)
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
	return &rfc1035v1alpha1.Zone{
		ObjectMeta: metav1.ObjectMeta{
			Name: z.Origin,
		},
		Spec: rfc1035v1alpha1.ZoneSpec{
			Zone: rfc1035Zone,
		},
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
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
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

const (
	nsName = "ns.dns"
)

func newSOARecord(zone string) ([]dns.RR, error) {
	if zone == "" {
		return nil, fmt.Errorf("zone cannot be empty")
	}
	zone = dns.Fqdn(zone)
	ttl := uint32(30)
	Mbox := dnsutil.Join("hostmaster", zone)
	Ns := dnsutil.Join(nsName, zone)
	header := dns.RR_Header{Name: zone, Rrtype: dns.TypeSOA, Ttl: ttl, Class: dns.ClassINET}
	soa := &dns.SOA{Hdr: header,
		Mbox:    Mbox,
		Ns:      Ns,
		Serial:  uint32(time.Now().Unix()),
		Refresh: 7200,
		Retry:   1800,
		Expire:  86400,
		Minttl:  ttl,
	}
	return []dns.RR{soa}, nil
}

func NewNSRecord(zone string, ip net.IP) (records, extra []dns.RR, err error) {
	if zone == "" {
		return nil, nil, fmt.Errorf("zone cannot be empty")
	}
	host := dns.Fqdn(dnsutil.Join(nsName, zone))
	zone = dns.Fqdn(zone)
	ttl := uint32(30)
	records = []dns.RR{
		&dns.NS{Hdr: dns.RR_Header{Name: zone, Rrtype: dns.TypeNS, Class: dns.ClassINET, Ttl: ttl}, Ns: host},
	}
	extra = append(extra, newAddress(host, ttl, ip, dns.TypeA))
	return records, extra, nil
}

func newAddress(name string, ttl uint32, ip net.IP, what uint16) dns.RR {
	hdr := dns.RR_Header{Name: name, Rrtype: what, Class: dns.ClassINET, Ttl: ttl}

	if what == dns.TypeA {
		return &dns.A{Hdr: hdr, A: ip}
	}
	// Should always be dns.TypeAAAA
	return &dns.AAAA{Hdr: hdr, AAAA: ip}
}

func newA(name string, ttl uint32, ip net.IP) *dns.A {
	return &dns.A{Hdr: dns.RR_Header{Name: name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: ttl}, A: ip}
}

func newCNAME(name string, ttl uint32, target string) *dns.CNAME {
	return &dns.CNAME{Hdr: dns.RR_Header{Name: name, Rrtype: dns.TypeCNAME, Class: dns.ClassINET, Ttl: ttl}, Target: dns.Fqdn(target)}
}

func newTXT(name string, ttl uint32, text string) *dns.TXT {
	return &dns.TXT{Hdr: dns.RR_Header{Name: name, Rrtype: dns.TypeTXT, Class: dns.ClassINET, Ttl: ttl}, Txt: split255(text)}
}

func newSRV(name string, ttl uint32, target string, weight uint16, priority, port uint16) *dns.SRV {
	host := dns.Fqdn(target)

	return &dns.SRV{Hdr: dns.RR_Header{Name: name, Rrtype: dns.TypeSRV, Class: dns.ClassINET, Ttl: ttl},
		Priority: priority, Weight: weight, Port: port, Target: host}
}

// Split255 splits a string into 255 byte chunks.
func split255(s string) []string {
	if len(s) < 255 {
		return []string{s}
	}
	sx := []string{}
	p, i := 0, 255
	for {
		if i <= len(s) {
			sx = append(sx, s[p:i])
		} else {
			sx = append(sx, s[p:])
			break
		}
		p, i = p+255, i+255
	}

	return sx
}
