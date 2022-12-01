# ksdns

An Operator for serving delegated zones that can be updated using rfc2136 (dynamic updates)

## Description

`ksdns` deploys `CoreDNS` and `zupd` (a CoreDNS server with RFC2136 support) to a Kubernetes cluster.

Zones are created and managed using `CRDs`.

## Use Case

`ksdns` can provide "service domains" for clusters. A service domain is a delegated domain that may be used by external-dns to update records dynamically. This also enables the use of cert-manager to provide public let's encrypt certificates for internal services.

### Getting started

1. Register a domain in AWS R53 (Or any supported provider for cert-manager)
2. Deploy `ksdns` and setup a delegated zone pointing to the `CoreDNS` service external-ip.

    ```zone
    blahonga.me NS  Simple                      -   xxx.awsdns-62.co.uk.
                                                    xxx.awsdns-62.net.
                                                    xxx.awsdns-40.com.
                                                    xxx.awsdns-28.org.
    blahonga.me SOA Simple                      -   xxx.awsdns-62.co.uk. awsdns-hostmaster.amazon.com. 1 7200 900 1209600 86400
    
    ksdns.blahonga.me   A   Simple  -   192.168.1.1 ; glue record pointing to ksdns
    service.blahonga.me NS  Simple  -   ksdns.blahonga.me ; delegated domain
    ```

    Create the zone object for ksdns:

    ```yaml
    apiVersion: rfc1035.ksdns.io/v1alpha1
    kind: Zone
    metadata:
        labels:
            app.kubernetes.io/name: zone
            app.kubernetes.io/instance: zone-service.blahonga.me
        name: service.blahonga.me
    spec:
    zone: |
        ; service.blahonga.me zone
        $ORIGIN service.blahonga.me.
        @                      3600 SOA   ksdns.blahonga.me (
                                    zone-admin.blahonga.corp.     ; address of responsible party
                                    20160727                   ; serial number, not used
                                    3600                       ; refresh period
                                    600                        ; retry period
                                    604800                     ; expire time
                                    1800                     ) ; minimum ttl
                              86400 NS    ksdns.blahonga.me.
        mail                  14400 A     204.13.248.106
        vpn                      60 A     216.146.45.240
        webapp                   60 A     216.146.46.10
        webapp                   60 A     216.146.46.11
        service               IN    SRV   8080 10 10 @
    ```

3. Deploy external-dns in a cluster and setup a RFC2136 provider using the `zupd` service.
4. Deploy cert-manager and setup dns verification for the public zone in R53.

External-dns will now create records in the (internal) delegated zone for the cluster. The records should be resolvable form the internal network only.

If you need a let's encrypt cert, request a cert for a record in `ksdns`. Cert-manager will setup the DNS verification in the public R53 zone and `ksdns` will make sure that the service is resolvable inside your network.

zone: blahonga.me in R53, <cluster>.service.blahonga.me delegation setup in R53, pointing to the `ksdns` `CoreDNS`deployment.

## Getting Started

You’ll need a Kubernetes cluster to run against. You can use [KIND](https://sigs.k8s.io/kind) to get a local cluster for testing, or run against a remote cluster.
**Note:** Your controller will automatically use the current context in your kubeconfig file (i.e. whatever cluster `kubectl cluster-info` shows).

### Running on the cluster

1. Install Instances of Custom Resources:

```sh
kubectl apply -f config/samples/
```

2. Build and push your image to the location specified by `IMG`:
 
```sh
make docker-build docker-push IMG=<some-registry>/ksdns:tag
```
 
3. Deploy the controller to the cluster with the image specified by `IMG`:

```sh
make deploy IMG=<some-registry>/ksdns:tag
```

### Uninstall CRDs

To delete the CRDs from the cluster:

```sh
make uninstall
```

### Undeploy controller

UnDeploy the controller to the cluster:

```sh
make undeploy
```

## Contributing

// TODO(user): Add detailed information on how you would like others to contribute to this project

### How it works

This project aims to follow the Kubernetes [Operator pattern](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/)

It uses [Controllers](https://kubernetes.io/docs/concepts/architecture/controller/)
which provides a reconcile function responsible for synchronizing resources untile the desired state is reached on the cluster

### Test It Out

1. Install the CRDs into the cluster:

```sh
make install
```

2. Run your controller (this will run in the foreground, so switch to a new terminal if you want to leave it running):

```sh
make run
```

**NOTE:** You can also run this in one step by running: `make install run`

### Modifying the API definitions

If you are editing the API definitions, generate the manifests such as CRs or CRDs using:

```sh
make manifests
```

**NOTE:** Run `make --help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## License

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
