# Getting Started

These instructions will get you a copy of the project up and running on your local machine for development and testing purposes.


## Prerequisites

- Read [Set up Cluster Federation with Kubefed](https://kubernetes.io/docs/tasks/federation/set-up-cluster-federation-kubefed/) and understand how to set up a Federation.
- This document assumes that you have a [Federation](https://kubernetes.io/docs/tasks/federation/set-up-cluster-federation-kubefed/) set up. 
- An account with desired cloud provider must be set up for clusters to be provisioned.
- Keep the access keys and credentials ready for relevant cloud provider, for example,
    * [Follow instructions to create or manage AWS keys ](http://docs.aws.amazon.com/IAM/latest/UserGuide/id_credentials_access-keys.html)
    * [Follow instructions to set Wercker Cluster parameters](WerckerClustersParameters.md)
- If helm is being used, the project must be cloned so that helm templates are available for the relevant commands.


## Deployment using ClusterManagerTemplate.yaml

1. Create a yaml deployment file using [`ClusterManagerTemplate.yaml`](../examples/ClusterManagerTemplate.yaml). For example,
you can use a bash script to set the required environment variables and generate a new file called `ClusterManager.yaml`.

    ```
    #!/bin/bash
    
    # Set environment variables 
    export FEDERATION_HOST=fedhost
    export FEDERATION_CONTEXT=akube
    export FEDERATION_NAMESPACE=federation-system
    export IMAGE_REPOSITORY=someregistry.io/somewhere
    export IMAGE_VERSION=tagversion
    export DOMAIN=something.net
    export KOPS_STATE_STORE=s3://state-store.something.net
    export AWS_ACCESS_KEY_ID=awsaccesskeyid
    export AWS_SECRET_ACCESS_KEY=awssecretaccesskey
    export OKE_BEARER_TOKEN=werckerclustersbearertoken
    export OKE_AUTH_GROUP=werckerclustersauthgroup
    export OKE_CLOUD_AUTH_ID=werckerclusterscloudauthid
     
    # Convert API access keys to base64 for Kubernetes secret storage.
    export AWS_ACCESS_KEY_ID_BASE64=$(echo -n "$AWS_ACCESS_KEY_ID" | base64)
    export AWS_SECRET_ACCESS_KEY_BASE64=$(echo -n "$AWS_SECRET_ACCESS_KEY" | base64)
    export OKE_BEARER_TOKEN_BASE64=$(echo -n "$OKE_BEARER_TOKEN" | base64)
    export OKE_AUTH_GROUP_BASE64=$(echo -n "$OKE_AUTH_GROUP" | base64)
    export OKE_CLOUD_AUTH_ID_BASE64=$(echo -n "$OKE_CLOUD_AUTH_ID" | base64)
     
    # Generate a new ClusterManager.yaml by using ClusterManagerTemplate.yaml and environment variables.
    eval "cat <<EOF
    $(<ClusterManagerTemplate.yaml)
    EOF" > ClusterManager.yaml  
    ```

1. Use `kubectl` to deploy the generated `ClusterManager.yaml` file.
     
    ``` 
    kubectl --context $FEDERATION_HOST create -f ClusterManager.yaml
    ```

1. Verify if Cluster Manager is installed.
    
    ```
    kubectl --context $FEDERATION_HOST get pods --all-namespaces | grep cluster-manager
    ```

1. (Optional) Uninstall Cluster Manager.
    
    ```
    kubectl --context $FEDERATION_HOST delete -f ClusterManager.yaml
    ```

## Deployment using Helm

1. Install tiller if not installed on the Federation host.
    
    ```
    helm init
    ```

2. Execute helm install with the following values set. Note that the following command makes a reference to helm chart
in the project's [deploy](./) directory (./helm/cluster-manager). Therefore this command should either run from the
deploy directory or may refer to the helm chart in the deploy directory.
    
    ```
    FEDERATION_NAMESPACE=federation-system
    FEDERATION_HOST=fedhost
    FEDERATION_CONTEXT=akube
    helm install ./helm/cluster-manager \
        --name cluster-manager \
        --set awsAccessKeyId="AWSACCESSKEYID" \
        --set awsSecretAccessKey="AWSSECRETACCESSKEY" \
        --set okeBearerToken="WerckerClustersBearerToken" \
        --set okeAuthGroup="WerckerClustersAuthGroup" \
        --set okeCloudAuthId="WerckerClustersCloudAuthId" \
        --set federationEndpoint="https://$FEDERATION_CONTEXT-apiserver" \
        --set federationContext="$FEDERATION_CONTEXT" \
        --set federationNamespace="$FEDERATION_NAMESPACE" \
        --set domain="something.fed.net" \
        --set image.repository="someregistry.io/somewhere" \
        --set image.tag="v1" \
        --set okeApiHost="api.cluster.us-ashburn-1.oracledx.com" \
        --set statestore="s3://clusters-state" \
        --kube-context "$FEDERATION_HOST"
    ```  
    Where, 
    * `FEDERATION_NAMESPACE` - Namespace where federation was installed via kubefed init
    * `FEDERATION_HOST` - Federation host name
    * `FEDERATION_CONTEXT` - Federation context name
    * `awsAccessKeyId` - AWS access key ID
    * `awsSecretAccessKey` - AWS secret access ID
    * `okeBearerToken`- Wercker Clusters bearer token
    * `okeAuthGroup` - Wercker Clusters auth group
    * `okeCloudAuthId` - Wercker Clusters cloud auth ID
    * `federationEndpoint` - Federation API server endpoint
    * `federationContext` - Federation name
    * `federationNamespace` - Similar to FEDERATION_NAMESPACE. Defaults to federation-system if not set.
    * `domain` - AWS domain name
    * `image.repository` - Repository where Cluster Manager is stored
    * `image.tag` - Cluster Manager image tag/version no.
    * `okeApiHost` - Wercker Clusters API host endpoint
    * `statestore` - AWS cluster S3 state store

3. Verify if Cluster Manager is installed.

    ```
    helm --kube-context $FEDERATION_HOST list cluster-manager
    ```

4. (Optional) Uninstall Cluster Manager.

    ```
    helm delete --kube-context $FEDERATION_HOST --purge cluster-manager
    ```

## Examples on how to use Cluster Manager

### Provisioning a Wercker Cluster

1. Deploy a Wercker Cluster to the Federation, use [`ClusterOke.yaml`](../examples/ClusterOke.yaml) from
examples.

    ```
    kubectl --context $FEDERATION_CONTEXT create -f ClusterOke.yaml
    ```
    This command deploys the cluster in an offline state. To customize the Wercker cluster, change the parameters in `cluster-manager.n6s.io/cluster.config` annotation of cluster. Before deploying, you can modify *ClusterOke.yaml* or use `kubectl` annotate command. The supported parameters are

    - K8Version - Kubernetes Version. Valid values are 1.7.4, 1.7.9, or 1.8.0. Default value  is 1.7.4.
    - nodeZones - Available Domains. Valid values are AD-1, AD-2, AD-3. This field can be set to any combination of the set of values.
    - shape - Valid values are VM.Standard1.1, VM.Standard1.2, VM.Standard1.4, VM.Standard1.8, VM.Standard1.16, VM.DenseIO1.4, VM.DenseIO1.8, VM.DenseIO1.16, BM.Standard1.36, BM.HighIO1.36 and BM.DenseIO1.36. 
    - workersPerAD - No. of worker nodes per availability domain.
    - compartment - Wercker Cluster Compartment ID where worker nodes will be instantiated. 

2. Initiate provisioning. **Note:** Do not perform these steps if you are using Navarkos. Navarkos determines on which cluster to perform the operation depending on demand and supply.
    - Update annotation `n6s.io/cluster.lifecycle.state` with value `pending-provision`.  
    - At the end of the provisioning, the value of `n6s.io/cluster.lifecycle.state` will be changed either to `ready` if successful or `failed-up` if failed. 

        ```
        kubectl --context=$FEDERATION_CONTEXT annotate cluster akube-oke n6s.io/cluster.lifecycle.state=pending-provision --overwrite
        ```

### Provisioning an AWS Cluster

1. Deploy an AWS cluster to the Federation using KOPS, use [`ClusterAwsEast2.yaml`](../examples/ClusterAwsEast2.yaml)
from examples.
                                                                                                        
    ```
    kubectl --context $FEDERATION_CONTEXT create -f ClusterAwsEast2.yaml
    ```

    This command deploys the cluster in an offline state. To customize the AWS cluster, change the parameters in
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

1. Initiate provisioning. **Note:** Do not perform this step if you are using Navarkos. Navarkos determines on which cluster to perform the operation depending on demand and supply.
    - Update annotation `n6s.io/cluster.lifecycle.state` with value `pending-provision`.
    - At the end of the provisioning, the value of `n6s.io/cluster.lifecycle.state` will be changed either to `ready` if successful or `failed-up` if failed. 

    ```
    kubectl --context=$FEDERATION_CONTEXT annotate cluster akube-us-east-2 n6s.io/cluster.lifecycle.state=pending-provision --overwrite
    ```

### Scaling up an already provisioned cluster
**Note:** Do not perform these steps if you are using Navarkos. Navarkos determines on which cluster to perform the operation depending on demand and supply.

- If there is more demand, you can scale up an already provisioned cluster to support more load. Update the annotation `n6s.io/cluster.lifecycle.state` with value `pending-up`. 
- You can configure the scale up size using the annotation `cluster-manager.n6s.io/cluster.scale-up-size`, otherwise it uses the default value 1.
- At the end of the provisioning, the value of `n6s.io/cluster.lifecycle.state` will be changed either to `ready` if successful or `failed-up` if failed. 

Here is an example of scaling up a previously provisioned `akube-us-east-2` AWS cluster by 5

```
kubectl --context=$FEDERATION_CONTEXT annotate cluster akube-us-east-2 cluster-manager.n6s.io/cluster.scale-up-size=5 --overwrite && \
kubectl --context=$FEDERATION_CONTEXT annotate cluster akube-us-east-2 n6s.io/cluster.lifecycle.state=pending-up --overwrite
```

### Scaling down an already provisioned cluster
**Note:** Do not perform these steps if you are using Navarkos. Navarkos determines on which cluster to perform the operation depending on demand and supply.

- If the demand is low, then you can scale down an already provisioned cluster. 
    Update the annotation `n6s.io/cluster.lifecycle.state` with value `pending-down`. 
- You can configure scale down size using the annotation `cluster-manager.n6s.io/cluster.scale-down-size`, otherwise it uses the default value 1. 
- At the end of the provisioning, the value of `n6s.io/cluster.lifecycle.state` will be changed either to `ready` if successful or `failed-up` if failed. 

Here is an example of scaling down a previously provisioned `akube-us-east-2` AWS cluster by 5

```
kubectl --context=$FEDERATION_CONTEXT annotate cluster akube-us-east-2 cluster-manager.n6s.io/cluster.scale-down-size=5 --overwrite && \
kubectl --context=$FEDERATION_CONTEXT annotate cluster akube-us-east-2 n6s.io/cluster.lifecycle.state=pending-down --overwrite
```

### Shutting down a provisioned cluster
**Note:** Do not perform these steps if you are using Navarkos. Navarkos determines on which cluster to perform the operation depending on the demand and supply.

- You can shut down a provisioned cluster when it is not in use. Update annotation `n6s.io/cluster.lifecycle.state` with value `pending-shutdown`. 
- At the end of the provisioning, the value of `n6s.io/cluster.lifecycle.state` will be changed to `offline`.

Here is an example of shutting down a previously provisioned `akube-us-east-2` AWS cluster

```
kubectl --context=$FEDERATION_CONTEXT annotate cluster akube-us-east-2 n6s.io/cluster.lifecycle.state=pending-shutdown --overwrite
```

### Checking cluster status

- To check the status of a specific cluster, execute the command and note the value of `n6s.io/cluster.lifecycle.state` annotation.

    ```
    kubectl --context=$FEDERATION_CONTEXT get cluster <Cluster Name> -o yaml
    ``` 
    
- To check the status of all the clusters, execute the command and note the value of `n6s.io/cluster.lifecycle.state` annotation for each of the clusters listed .

    ```
    kubectl --context=$FEDERATION_CONTEXT get clusters -o yaml
    ```    