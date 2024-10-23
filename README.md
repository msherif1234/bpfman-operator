# bpfman-operator

The `bpfman-operator` repository exists to deploy and manage bpfman within a Kubernetes cluster.
This operator was built using some great tooling provided by the
[operator-sdk library](https://sdk.operatorframework.io/).
A great first step in understanding some of the functionality can be to just run `make help`.


[![license](https://img.shields.io/github/license/bpfman/bpfman-operator.svg?maxAge=2592000)](https://github.com/bpfman/bpfman-operator/blob/main/LICENSE)
[![Project maturity: alpha](https://img.shields.io/badge/maturity-alpha-orange.svg)]() 
[![Go report card](https://goreportcard.com/badge/github.com/bpfman/bpfman-operator)](https://goreportcard.com/report/github.com/bpfman/bpfman-operator)

## Deploy bpfman Operator

The `bpfman-operator` is running as a Deployment with a ReplicaSet of one.
It runs on the control plane and is composed of the containers `bpfman-operator` and
`kube-rbac-proxy`.
The operator is responsible for launching the bpfman Daemonset, which runs on every node.
The bpfman Daemonset is composed of the containers `bpfman`, `bpfman-agent`, and `node-driver-registrar`.

### Deploy Locally via KIND

After reviewing the possible make targets it's quick and easy to get `bpfman` deployed locally on your system
via a [KIND cluster](https://kind.sigs.k8s.io/), run:

```bash
make run-on-kind
```

> **NOTE:** By default, bpfman-operator deploys bpfman with CSI enabled.
CSI requires Kubernetes v1.26 due to a PR
([kubernetes/kubernetes#112597](https://github.com/kubernetes/kubernetes/pull/112597))
that addresses a gRPC Protocol Error that was seen in the CSI client code and it doesn't appear to have
been backported.
It is recommended to install kind v0.20.0 or later.

### Deploy To Openshift Cluster

First deploy the operator with one of the following two options:

#### 1. Manually with Kustomize

To manually install with Kustomize and raw manifests, execute the following
commands.
The Openshift cluster needs to be up and running and specified in `~/.kube/config`
file.

```bash
make deploy-openshift
```

To clean up at a later time, run:

```bash
make undeploy-openshift
```

#### 2. Via the OLM bundle

The other option for installing the `bpfman-operator` is through the
[OLM bundle](https://www.redhat.com/en/blog/deploying-operators-olm-bundles).

Use `operator-sdk` to install the bundle like so:

```bash
operator-sdk run bundle quay.io/bpfman/bpfman-operator-bundle:latest --namespace bpfman
```

To clean up at a later time, execute:

```bash
operator-sdk cleanup bpfman-operator
```

#### 3. Deploy as a bundle from the Console's OperatorHub page

This mode is recommended when you want to test the customer experience of navigating through the operators'
catalog and installing/configuring it manually through the UI, prior to committing the bundle to either: -

- https://github.com/redhat-openshift-ecosystem/community-operators-prod
 
or

- https://github.com/k8s-operatorhub/community-operators/pulls

```sh
export BUNDLE_IMG=quay.io/$USER/bpfman-operator-bundle:developement
make bundle bundle-build bundle-push
export CATALOG_IMG=quay.io/$USER/bpfman-operator-catalog:developement
make catalog-build catalog-push catalog-deploy
```

To clean up at a later time, execute:

```bash
make catalog-undeploy
```

## Verify the Installation

Regardless of the deployment method, if the `bpfman-operator` was deployed successfully,
you will see the `bpfman-daemon` and `bpfman-operator` pods running without errors:

```bash
kubectl get pods -n bpfman
NAME                             READY   STATUS    RESTARTS   AGE
bpfman-daemon-w24pr                3/3     Running   0          130m
bpfman-operator-78cf9c44c6-rv7f2   2/2     Running   0          132m
```

## Deploy an eBPF Program to the cluster

To test the deployment simply deploy one of the sample `xdpPrograms`:

```bash
kubectl apply -f config/samples/bpfman.io_v1alpha1_xdp_pass_xdpprogram.yaml
```

If loading of the XDP Program to the selected nodes was successful, it will be reported
back to the user via the `xdpProgram`'s status field:

```bash
kubectl get xdpprogram xdp-pass-all-nodes -o yaml
apiVersion: bpfman.io/v1alpha1
kind: XdpProgram
metadata:
  annotations:
    kubectl.kubernetes.io/last-applied-configuration: |
      {"apiVersion":"bpfman.io/v1alpha1","kind":"XdpProgram","metadata":{"annotations":{},"labels":{"app.kubernetes.io/name":"xdpprogram"},"name":"xdp-pass-all-nodes"},"spec":{"bpffunctionname":"pass","bytecode":{"image":{"url":"quay.io/bpfman-bytecode/xdp_pass:latest"}},"globaldata":{"GLOBAL_u32":[13,12,11,10],"GLOBAL_u8":[1]},"interfaceselector":{"primarynodeinterface":true},"nodeselector":{},"priority":0}}
  creationTimestamp: "2023-11-07T19:16:39Z"
  finalizers:
  - bpfman.io.operator/finalizer
  generation: 2
  labels:
    app.kubernetes.io/name: xdpprogram
  name: xdp-pass-all-nodes
  resourceVersion: "157187"
  uid: 21c71a61-4e73-44eb-9b49-07af2866d25b
spec:
  bpffunctionname: pass
  bytecode:
    image:
      imagepullpolicy: IfNotPresent
      url: quay.io/bpfman-bytecode/xdp_pass:latest
  globaldata:
    GLOBAL_u8: AQ==
    GLOBAL_u32: DQwLCg==
  interfaceselector:
    primarynodeinterface: true
  mapownerselector: {}
  nodeselector: {}
  priority: 0
  proceedon:
  - pass
  - dispatcher_return
status:
  conditions:
  - lastTransitionTime: "2023-11-07T19:16:42Z"
    message: bpfProgramReconciliation Succeeded on all nodes
    reason: ReconcileSuccess
    status: "True"
    type: ReconcileSuccess
```

To view information in listing form simply run:

```bash
kubectl get xdpprogram -o wide
NAME                 BPFFUNCTIONNAME   NODESELECTOR   PRIORITY   INTERFACESELECTOR               PROCEEDON
xdp-pass-all-nodes   pass              {}             0          {"primarynodeinterface":true}   ["pass","dispatcher_return"]
```

## API Types Overview

Refer to [api-spec.md](https://bpfman.io/main/developer-guide/api-spec/) for a detailed description of all the bpfman Kubernetes API types.

### Multiple Program CRDs

The multiple `*Program` CRDs are the bpfman Kubernetes API objects most relevant to users.
They express how and where eBPF programs are to be deployed within a Kubernetes cluster. Currently, 
bpfman supports:

* `fentryProgram`
* `fexitProgram`
* `kprobeProgram`
* `tcProgram`
* `tracepointProgram`
* `uprobeProgram`
* `xdpProgram`

## BpfApplication CRD

The `BpfApplication` CRD is designed for managing eBPF programs at an application level within a Kubernetes cluster.
This CRD allows Kubernetes users to define which eBPF programs are essential for an application's operations and specify
how these programs should be deployed across the cluster.

## BpfProgram CRD

The `BpfProgram` CRD is used internally by the `bpfman-deployment` to keep track of per-node `bpfman` state,
such as map pinpoints, and to report node-specific errors back to the user.
Kubernetes' users/controllers are only allowed to view these objects, NOT create or edit them.

Applications wishing to use `bpfman` to deploy/manage their eBPF programs in Kubernetes will make use of this
object to find references to the bpfMap pinpoints (`spec.maps`) to configure their eBPF programs.

## Developer

For more architecture details about `bpfman-operator`, refer to
[Developing the bpfman-operator](https://bpfman.io/v0.5.4/developer-guide/develop-operator)

### Bpfman-agent profiling

bpfman-agent process use port `6060` for Golang profiling to be able to get the different profiles

1- Set port-forward rule in a different terminal 
```shell
oc get pods -n bpfman
NAME                               READY   STATUS    RESTARTS   AGE
bpfman-daemon-76v57                3/3     Running   0          14m
bpfman-operator-7f67bc7c57-ww52z   2/2     Running   0          14m

oc -n bpfman port-forward bpfman-daemon-76v57 6060
```

2- Download the required profiles:

`curl -o <profile> http://localhost:6060/debug/pprof/<profile>`

Where <profile> can be:

| profile      | description                                                                   |
|--------------|-------------------------------------------------------------------------------|
| allocs       | A sampling of all memory allocations                                          |
| block        | Stack traces that led to blocking on synchronization primitives               |
| cmdline      | The command line invocation of the current program                            |
| goroutine    | Stack traces of all current goroutines                                        |
| heap         | A sampling of memory allocations of live objects.                             |
|              | You can specify the gc GET parameter to run GC before taking the heap sample. |
| mutex        | Stack traces of holders of contended mutexes                                  |
| profile      | CPU profile.                                                                  |
|              | You can specify the duration in the seconds GET parameter.                    |
| threadcreate | Stack traces that led to the creation of new OS threads                       |
| trace        | A trace of execution of the current program.                                  |
|              | You can specify the duration in the seconds GET parameter.                    |

Example:

```shell
curl "http://localhost:6060/debug/pprof/trace?seconds=20" -o trace
curl "http://localhost:6060/debug/pprof/profile?duration=20" -o cpu
curl "http://localhost:6060/debug/pprof/heap?gc" -o heap
curl "http://localhost:6060/debug/pprof/allocs" -o allocs
curl "http://localhost:6060/debug/pprof/goroutine" -o goroutine
```

3- Use [go tool pprof](https://go.dev/blog/pprof) to dig into the profiles (go tool trace for the trace profile) or use 
  web interface for example `go tool pprof -http=:8080 cpu`
