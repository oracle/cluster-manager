[![Go Report Card](https://goreportcard.com/badge/github.com/oracle/kubernetes-incubator/cluster-manager)](http://goreportcard.com/report/github.com/oracle/kubernetes-incubator/cluster-manager)
# Cluster Manager

Cluster Manager manages the life cycle of Kubernetes clusters from different cloud providers.

## Before You Begin
Cluster Manager supports provisioning, scaling up, scaling down and shutting down of Kubernetes clusters on various
cloud providers. Currently AWS and Oracle Wercker Clusters are supported. Support for more providers will be added in
the near future

The Cluster Manager supports the following operations on clusters:

1. Provisioning: Cluster Manager can create creates a new Kubernetes cluster on a cloud provider.
1. Scale Up: Cluster Manager can scale up nodes on a currently provisioned cluster on any supported cloud provider
1. Scale Down: Cluster Manager can scale down nodes on a currently provisioned cluster on any supported cloud provider
1. Shutdown: Cluster Manager can shutdown a currently provisioned cluster on any supported cloud provider

Cluster Manager may be used in tandem with [Navarkos](https://github.com/oracle/kubernetes-incubator/navarkos)
to help scale out deployments to various cloud providers.

<br>
<img src="overview.png" alt="cluster-manager with Navarkos" width="80%" height="80%"/>
<figcaption>Figure:1 cluster-manager with Navarkos Architecture</figcaption>
<br>

## Deploy
Please refer to [Deployment README.md](deploy/README.md) on how to deploy Cluster Manager and use it provision and
managed clusters

## Build
Contributions to this code are welcome!  The code in this repository can be built and tested using the Makefile.

### Requirements
Cluster Manager build has been currently tested with go version 1.8.x

### Clone Repository
Clone the source to a local repository

```bash
export K8S_INCUBATOR_DIR="$GOPATH/src/github.com/kubernetes-incubator"
mkdir -p $K8S_INCUBATOR_DIR
cd $K8S_INCUBATOR_DIR
git clone https://github.com/oracle/kubernetes-incubator/navarkos
```

### Build Project
Project Makefile supports the following targets

- `make build-local` - cleans up and build Cluster Manager binary.
- `make build-image` - invokes 'build-local' defined above followed by building Cluster Manager docker image.
- `make build-docker` - cleans up and build docker image of Cluster Manager using docker multi-stage build.
- `make push-image` - pushes an already create Cluster Manager docker image to registry of choice. Use ${DOCKER_REGISTRY} environment variable to specify the registry.
- `make build-push-image` - cleans up, build and push Cluster Manager docker image to a registry of choice. Use ${DOCKER_REGISTRY} environment variable to specify the registry,

Please refer to [RELEASE.md](RELEASE.md) about the process of release of this project

## Community, discussion, contribution, and support

Learn how to engage with the Kubernetes community on the [community page](http://kubernetes.io/community/).

You can reach the maintainers of this project at:

- Slack: #sig-multicluster
- Mailing List: https://groups.google.com/forum/#!forum/sig-multicluster

Please refer to [CONTRIBUTING.md](CONTRIBUTING.md) on how to contribute to the project

## Kubernetes Incubator - Pending proposal
This is a [Kubernetes Incubator project](https://github.com/kubernetes/community/blob/master/incubator.md). The project was established 2017-12-08. The incubator team for the project is:
- Sponsor: tbd
- Champion: tbd
- SIG: ~~sig-multicluster~~ tbd

## Code of conduct

Participation in the Kubernetes community is governed by the [Kubernetes Code of Conduct](code-of-conduct.md).

## (Optional) Authors - Kubernetes authors

See also the list of [contributors](https://github.com/your/project/contributors) who participated in this project.

