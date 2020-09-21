# Kubernetes API, RDS example

_Small golang binary watching Istio Gateway resources on Kubernetes API and serving them as Envoy RDS configuration_

## Introduction

This project is a proof of concept demonstrating how to dynamically control an Envoy Proxy based on Istio `Gateway` resources 
on Kubernetes API.

## Getting started

### Build & Start Golang binary

```bash
go build -o k8s-watcher-rds-server
# Assuming you have a valid kubeconfig locally
./k8s-watcher-rds-server watch
# or use a specific ./k8s-watcher-rds-server watch -c /etc/rancher/k3s/k3s.yaml 
```

### Start Envoy listening to watcher for RDS configuration

RDS stands for [Route Discovery Service](https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_conn_man/rds).


```bash
docker run --rm --net=host \
            -v $(pwd)/deploy/envoy-config-rds.yaml:/etc/front-envoy.yaml \
            envoyproxy/envoy:v1.15.0 /usr/local/bin/envoy \
                     -c /etc/front-envoy.yaml \
                     --service-cluster front-proxy \
                     --service-node router~172.17.42.42~front-proxy.ns~ns.svc.cluster.local \
                     -l debug
```

### Deploy Istio Gateway CRD & a basic gateway on your cluster

```bash
kubectl apply -f https://raw.githubusercontent.com/Stoakes/k8s-watcher-rds-server/master/deploy/istio.yaml
```

Once deployed, you can now browse to [http://localhost:8001/config_dump](http://localhost:8001/config_dump) 
and check Envoy configuration, you should see a route on `hello.example.org`

You can also run `curl http://localhost:8000/` and get Envoy admin HTML (normally exposed on port 8001).