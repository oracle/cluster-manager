# Deploying Cluster Manager

## Prerequisites
* [Federation control pane](https://kubernetes.io/docs/tasks/federation/set-up-cluster-federation-kubefed/) must be setup
to deploy Cluster Manager. The federation host acts as a repository for cluster objects are are deployed to it, that
Cluster Manager then acts on. Instructions on setting up the federation control
can be found [here](https://kubernetes.io/docs/tasks/federation/set-up-cluster-federation-kubefed/)
* An account with desired cloud provider must be setup for clusters to be provisioned
* Access keys and credentials for relevant cloud provider, depending on which provider is being chosen for the following
examples
    * Instructions on getting details for AWS can be found 
    [here](http://docs.aws.amazon.com/IAM/latest/UserGuide/id_credentials_access-keys.html)
    * Instructions on getting details for Wercker Clusters can be found
    [here](WerckerClustersParameters.md)
* If helm is being used, the project must be cloned so that helm templates are available for the relevant commands

## Deployment using ClusterManager.yaml

Use kubectl to create a deployment using [`ClusterManager.yaml`](../examples/ClusterManager.yaml). Set the required
environment variables and proceed with the deployment:

```bash
export FEDERATION_HOST=fedhost
export FEDERATION_CONTEXT=akube
export FEDERATION_NAMESPACE=federation-system
export CLUSTER_MANAGER_IMAGE=/docker.io/somewhere/cluster-manager:v1
export AWS_ACCESS_KEY_ID=awsaccesskeyid
export AWS_SECRET_ACCESS_KEY=awssecretaccesskey
export OKE_BEARER_TOKEN=werckerclustersbearertoken
export OKE_AUTH_GROUP=werckerclustersauthgroup
export OKE_CLOUD_AUTH_ID=werckerclusterscloudauthid
export DOMAIN=something.net
export KOPS_STATE_STORE=state-store.something.net
 
kubectl --context $FEDERATION_HOST create -f ClusterManager.yaml
```

Deployment may be verified using the following command
```bash
kubectl --context $FEDERATION_HOST get pods --all-namespaces | grep cluster-manager
```

Cluster Manager may be undeployed using the following command
```bash
kubectl --context $FEDERATION_HOST delete -f ClusterManager.yaml
```

## Deployment using Helm

Install tiller if not yet installed in the federation host.
```bash
helm init
```

Execute helm install with values set. Note that the following command makes a reference to helm chart in project
(./helm/cluster-manager). Therefore this command should either run from project directory or may refer to helm chart
in project directory
```bash
FEDERATION_NAMESPACE=federation-system
FEDERATION_HOST=fedhost
helm install ./helm/cluster-manager \
    --name cluster-manager \
    --set awsAccessKeyId="AWSACCESSKEYID" \
    --set awsSecretAccessKey="AWSSECRETACCESSKEY" \
    --set okeBearerToken="WerckerClustersBearerToken" \
    --set okeAuthGroup="WerckerClustersAuthGroup" \
    --set okeCloudAuthId="WerckerClustersCloudAuthId" \
    --set federationEndpoint="https://akube-apiserver" \
    --set federationContext="akube" \
    --set federationNamespace="$FEDERATION_NAMESPACE" \
    --set domain="something.fed.net" \
    --set image.repository="docker.io/somewhere/" \
    --set image.tag="v1" \
    --set okeApiHost="api.cluster.us-ashburn-1.oracledx.com" \
    --set statestore="s3://clusters-state" \
    --namespace "$FEDERATION_NAMESPACE" \
    --kube-context "$FEDERATION_HOST"
```  

* FEDERATION_NAMESPACE - Namespace where federation was installed via kubefed init
* FEDERATION_HOST - Federation host name
* awsAccessKeyId - AWS access key ID
* awsSecretAccessKey - AWS secret access ID
* okeBearerToken- Wercker Clusters bearer token
* okeAuthGroup - Wercker Clusters auth group
* okeCloudAuthId - Wercker Clusters cloud auth ID
* federationEndpoint - Federation API server endpoint
* federationContext - Federation name
* federationNamespace - Similar to FEDERATION_NAMESPACE. Defaults to federation-system if not set.
* domain - AWS domain name
* image.repository - Repository where Cluster Manager is stored
* image.tag - Cluster Manager image tag/version no.
* okeApiHost - Wercker Clusters API host endpoint
* statestore - AWS cluster S3 state store

Deployment may be verified by running the following command:

```bash
helm --kube-context $FEDERATION_HOST list cluster-manager
```

To undeploy Cluster Manager, the following command can be used:

```bash
helm delete --context $FEDERATION_HOST --purge cluster-manager
```

## Provisioning an Oracle Wercker Cluster

To deploy an Oracle Wercker Cluster to the federation, use [`ClusterOke.yaml`](../examples/ClusterOke.yaml) from
examples
```bash
kubectl --context $FEDERATION_HOST create -f ClusterOke.yaml
```
The above command places the cluster in an offline state. To customize the Wercker cluster, change the parameters in
`cluster-manager.n6s.io/cluster.config` annotation of cluster. This may either be done by modifying ClusterOke.yaml
before deployment or using kubectl annotate command. The supported parameters are

- K8Version - Kubernetes Version. Valid values are 1.7.4, 1.7.9, or 1.8.0. Default value  is 1.7.4.
- nodeZones - Available Domains. Valid values are AD-1, AD-2, AD-3. This field can be set to any combination of the set of values.
- shape - Valid values are VM.Standard1.1, VM.Standard1.2, VM.Standard1.4, VM.Standard1.8, VM.Standard1.16, VM.DenseIO1.4, VM.DenseIO1.8, VM.DenseIO1.16, BM.Standard1.36, BM.HighIO1.36 and BM.DenseIO1.36. 
- workersPerAD - No. of worker nodes per availability domain.
- compartment - Wercker Cluster Compartment ID where worker nodes will be instantiated. 

To initiate provisioning update annotation
`n6s.io/cluster.lifecycle.state` with value `pending-provision`.  At the end of provisioning the value of
`n6s.io/cluster.lifecycle.state` is changed to `ready` if successful or `failed-provision` if failed.

```bash
kubectl --context=$FEDERATION_HOST annotate cluster akube-oke n6s.io/cluster.lifecycle.state=pending-provision --overwrite
```

## Provisioning an AWS Cluster
To deploy an AWS specific cluster to the federation using KOPS, use [`ClusterAwsEast2.yaml`](../examples/ClusterAwsEast2.yaml)
from examples
                                                                                                        
```bash
kubectl --context $FEDERATION_HOST create -f ClusterAwsEast2.yaml
```

The above command places the cluster in an offline state. To customize the AWS cluster, change the parameters in
`cluster-manager.n6s.io/cluster.config`. For valid values that are going to be used on some of the fields in
`cluster-manager.n6s.io/cluster.config` which is used to customize the cluster, please refer to the 
[AWS Documentation](https://aws.amazon.com/documentation):
- region - Region where  the cluster will be instantiated.
- masterZones - List of zones where master nodes will be instantiated.
- nodeZones - List of zones where worker nodes will be instantiated. 
- masterSize - Master node(s) instance size 
- nodeSize - Worker node(s) instance size. 
- numberOfMasters - No. of master nodes to instantiate.
- numberOfNodes - No. of worker nodes to instantiate. 
- sshkey - SSH public access key to the cluster nodes.

To initiate provisioning update annotation
`n6s.io/cluster.lifecycle.state` with value `pending-provision`.  At the end of provisioning the value of
`n6s.io/cluster.lifecycle.state` is changed to `ready` if successful or `failed-provision` if failed.

```bash
kubectl --context=$FEDERATION_HOST annotate cluster akube-us-east-2 n6s.io/cluster.lifecycle.state=pending-provision --overwrite
```

## Scaling up an already provisioned cluster

An already provisioned cluster may be scaled up for supporting more load. This can be achieved by updating annotation
`n6s.io/cluster.lifecycle.state` with value `pending-up`. The size for requested scale up may be configured using the
annotation `cluster-manager.n6s.io/cluster.scale-up-size`, otherwise the default of 1 is used. At the end of this
operation the value of `n6s.io/cluster.lifecycle.state` is changed to `ready` if successful or `failed-up` if failed.
Here is an example of scaling up a previously provisioned `akube-us-east-2` AWS cluster by 5

```bash
kubectl --context=$FEDERATION_HOST annotate cluster akube-us-east-2 cluster-manager.n6s.io/cluster.scale-up-size=5 --overwrite && \
    kubectl --context=$FEDERATION_HOST annotate cluster akube-us-east-2 n6s.io/cluster.lifecycle.state=pending-up --overwrite
```

## Scaling down an already provisioned cluster

An already provisioned cluster may be scaled down when the load is low. This can be achieved by updating annotation
`n6s.io/cluster.lifecycle.state` with value `pending-down`. The size for requested scale down may be configured using
the annotation `cluster-manager.n6s.io/cluster.scale-down-size`, otherwise the default of 1 is used. At the end of this
operation the value of `n6s.io/cluster.lifecycle.state` is changed to `ready` if successful or `failed-down` if failed.

Here is an example of scaling down a previously provisioned `akube-us-east-2` AWS cluster by 5
```bash
kubectl --context=$FEDERATION_HOST annotate cluster akube-us-east-2 cluster-manager.n6s.io/cluster.scale-down-size=5 --overwrite && \
    kubectl --context=$FEDERATION_HOST annotate cluster akube-us-east-2 n6s.io/cluster.lifecycle.state=pending-down --overwrite
```

## Shutting down a provisioned cluster

A provisioned cluster may be shutdown when it is not needed anymore. This is done by updating annotation
`n6s.io/cluster.lifecycle.state` with value `pending-shutdown`. At the end of this operation the value of
`n6s.io/cluster.lifecycle.state` is changed to `offline`.

Here is an example of shutting down a previously provisioned `akube-us-east-2` AWS cluster
```bash
kubectl --context=$FEDERATION_HOST annotate cluster akube-us-east-2 n6s.io/cluster.lifecycle.state=pending-shutdown --overwrite
```