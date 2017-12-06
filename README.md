# Cluster Manager

Cluster Manager manages the life cycle of Kubernetes clusters from different cloud providers.

## Before You Begin
Cluster Manager supports provisioning, scaling up, scaling down and shutting down of Kubernetes clusters on various
cloud providers. The cluster lifecycle status and cloud provider specific configuration are stored as annotations into 
the Kubernetes Cluster object in the Federation. Cluster Manager uses these annotations to decide which lifecycle
operation to perform and how to set up the cluster. Currently AWS and Wercker Clusters are supported. Support for more
providers will be added in the near future

Cluster Manager may be used with [Navarkos](https://github.com/oracle/kubernetes-incubator/navarkos)
to help scale out deployments to various cloud providers. When Cluster Manager is used with Navarkos, Navarkos 
determines on which cluster to perform the operation depending on the demand and supply.

<br>
<img src="overview.png" alt="cluster-manager with Navarkos" width="80%" height="80%"/>
<figcaption>Figure:1 cluster-manager with Navarkos Architecture</figcaption>
<br>

The Cluster Manager supports the following operations on clusters:

1. Provisioning: Cluster Manager can create creates a new Kubernetes cluster on a cloud provider.
1. Scale Up: Cluster Manager can scale up nodes on a currently provisioned cluster on any supported cloud provider
1. Scale Down: Cluster Manager can scale down nodes on a currently provisioned cluster on any supported cloud provider
1. Shutdown: Cluster Manager can shutdown a currently provisioned cluster on any supported cloud provider


## Deploy
Follow the [deploy instructions](deploy/README.md) to deploy Cluster Manager.

## Build

#### Requirements

Cluster Manager build has been currently tested with go version 1.8.x

#### Clone Repository

Clone the source to a local repository

```
export K8S_INCUBATOR_DIR="$GOPATH/src/github.com/kubernetes-incubator"
mkdir -p $K8S_INCUBATOR_DIR
cd $K8S_INCUBATOR_DIR
git clone https://github.com/oracle/cluster-manager
```

#### Build Project

Project Makefile supports the following targets

- `make build-local` - cleans up and build Cluster Manager binary.
- `make build-image` - invokes 'build-local' by building Cluster Manager docker image.
- `make build-docker` - cleans up and build docker image of Cluster Manager using docker multi-stage build.
- `make push-image` - pushes an already created Cluster Manager docker image to registry of choice. Use `${DOCKER_REGISTRY}` environment variable to specify the registry.
- `make build-push-image` - cleans up, build and push Cluster Manager docker image to a registry of choice. Use `${DOCKER_REGISTRY}` environment variable to specify the registry.

## Community, discussion, contribution, and support

Learn how to engage with the Kubernetes community on the [community page](http://kubernetes.io/community/).

You can reach the maintainers of this project at:

- Slack: tbd
- Mailing List: tbd
- [OWNERS](OWNERS) file has the project leads listed.

Contributions to this code are welcome!

Refer to [CONTRIBUTING.md](CONTRIBUTING.md) and [LICENSE](LICENSE) on how to contribute to the project.
Refer to [RELEASE.md](RELEASE.md) about the process of release of this project.

## Kubernetes Incubator - Pending proposal
This is a [Kubernetes Incubator project](https://github.com/kubernetes/community/blob/master/incubator.md). The project was established 2017-12-08. The incubator team for the project is:
- Sponsor: tbd
- Champion: tbd
- SIG: tbd

## Code of conduct

Participation in the Kubernetes community is governed by the [Kubernetes Code of Conduct](code-of-conduct.md).

