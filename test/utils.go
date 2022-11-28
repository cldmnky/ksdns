package test

import (
	"os"
	"path/filepath"
	"sync"

	_ "github.com/cldmnky/ksdns/pkg/zupd/core/plugin" // Load all managed plugins.
	"github.com/coredns/caddy"
	_ "github.com/coredns/coredns/core" // Hook in CoreDNS.
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin/test"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

var mu sync.Mutex

// Directives
var Directives = []string{
	"metadata",
	"geoip",
	"cancel",
	"tls",
	"reload",
	"nsid",
	"bufsize",
	"root",
	"bind",
	"debug",
	"trace",
	"ready",
	"health",
	"pprof",
	"prometheus",
	"errors",
	"log",
	"dnstap",
	"local",
	"dns64",
	"acl",
	"any",
	"chaos",
	"loadbalance",
	"tsig",
	"cache",
	"rewrite",
	"header",
	"dnssec",
	"autopath",
	"minimal",
	"template",
	"transfer",
	"hosts",
	"route53",
	"azure",
	"clouddns",
	"k8s_external",
	"kubernetes",
	"file",
	"dynamicupdate",
	"auto",
	"secondary",
	"etcd",
	"loop",
	"forward",
	"grpc",
	"erratic",
	"whoami",
	"on",
	"sign",
	"view",
	"kubeapi",
}

func init() {
	dnsserver.Directives = Directives
}

// CoreDNSServer returns a CoreDNS test server. It just takes a normal Corefile as input.
func CoreDNSServer(corefile string) (*caddy.Instance, error) {
	mu.Lock()
	defer mu.Unlock()
	caddy.Quiet = true
	dnsserver.Quiet = true
	return caddy.Start(NewInput(corefile))
}

// CoreDNSServerStop stops a server.
func CoreDNSServerStop(i *caddy.Instance) { i.Stop() }

// CoreDNSServerPorts returns the ports the instance is listening on. The integer k indicates
// which ServerListener you want.
func CoreDNSServerPorts(i *caddy.Instance, k int) (udp, tcp string) {
	srvs := i.Servers()
	if len(srvs) < k+1 {
		return "", ""
	}
	u := srvs[k].LocalAddr()
	t := srvs[k].Addr()

	if u != nil {
		udp = u.String()
	}
	if t != nil {
		tcp = t.String()
	}
	return
}

// CoreDNSServerAndPorts combines CoreDNSServer and CoreDNSServerPorts to start a CoreDNS
// server and returns the udp and tcp ports of the first instance.
func CoreDNSServerAndPorts(corefile string) (i *caddy.Instance, udp, tcp string, err error) {
	i, err = CoreDNSServer(corefile)
	if err != nil {
		return nil, "", "", err
	}
	udp, tcp = CoreDNSServerPorts(i, 0)
	return i, udp, tcp, nil
}

// Input implements the caddy.Input interface and acts as an easy way to use a string as a Corefile.
type Input struct {
	corefile []byte
}

// NewInput returns a pointer to Input, containing the corefile string as input.
func NewInput(corefile string) *Input {
	return &Input{corefile: []byte(corefile)}
}

// Body implements the Input interface.
func (i *Input) Body() []byte { return i.corefile }

// Path implements the Input interface.
func (i *Input) Path() string { return "Corefile" }

// ServerType implements the Input interface.
func (i *Input) ServerType() string { return "dns" }

func kubeConfigFromRestConfig(config *rest.Config, namespace string) (string, func(), error) {
	name, rm, err := test.TempFile("/tmp", "")
	if err != nil {
		return "", nil, err
	}
	clusters := make(map[string]*clientcmdapi.Cluster)
	clusters["default-cluster"] = &clientcmdapi.Cluster{
		Server:                   config.Host,
		CertificateAuthorityData: config.CAData,
	}

	contexts := make(map[string]*clientcmdapi.Context)
	contexts["default-context"] = &clientcmdapi.Context{
		Cluster:   "default-cluster",
		Namespace: namespace,
		AuthInfo:  namespace,
	}

	authinfos := make(map[string]*clientcmdapi.AuthInfo)
	authinfos[namespace] = &clientcmdapi.AuthInfo{
		ClientCertificateData: config.CertData,
		ClientKeyData:         config.KeyData,
	}

	clientConfig := clientcmdapi.Config{
		Kind:           "Config",
		APIVersion:     "v1",
		Clusters:       clusters,
		Contexts:       contexts,
		CurrentContext: "default-context",
		AuthInfos:      authinfos,
	}
	clientcmd.WriteToFile(clientConfig, name)
	return name, rm, nil
}

func writeKubeClientCerts(dir string, caData, certData, keyData []byte) (string, func(), error) {
	tempDir, err := os.MkdirTemp(dir, "go-test-pemfiles")
	if err != nil {
		return "", nil, err
	}

	path := filepath.Join(tempDir, "ca.pem")
	if err := os.WriteFile(path, caData, 0644); err != nil {
		return "", nil, err
	}
	path = filepath.Join(tempDir, "cert.pem")
	if err = os.WriteFile(path, certData, 0644); err != nil {
		return "", nil, err
	}
	path = filepath.Join(tempDir, "key.pem")
	if err = os.WriteFile(path, keyData, 0644); err != nil {
		return "", nil, err
	}

	rmFunc := func() { os.RemoveAll(tempDir) }
	return tempDir, rmFunc, nil
}
