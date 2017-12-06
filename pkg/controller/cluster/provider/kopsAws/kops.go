/*
Copyright 2017 The Kubernetes Authors.

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

// aws-kops is the implementation of pkg/clusterprovider interface for AWS kops

package kopsAws

import (
	"encoding/json"
	"fmt"
	"github.com/golang/glog"
	"github.com/kubernetes-incubator/cluster-manager/pkg/common"
	"github.com/kubernetes-incubator/cluster-manager/pkg/controller/cluster/options"
	"github.com/kubernetes-incubator/cluster-manager/pkg/controller/cluster/provider"
	"github.com/kubernetes-incubator/cluster-manager/pkg/controller/cluster/util"
	navarkospkgcommon "github.com/kubernetes-incubator/navarkos/pkg/common"
	"github.com/pkg/errors"
	"k8s.io/client-go/kubernetes"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/kops"
	apiskops "k8s.io/kops/pkg/apis/kops"
	"k8s.io/kops/pkg/client/simple"
	"k8s.io/kops/pkg/kubeconfig"
	"k8s.io/kops/pkg/resources"
	"k8s.io/kops/pkg/validation"
	"k8s.io/kops/upup/pkg/fi"
	"k8s.io/kops/upup/pkg/fi/cloudup"
	"k8s.io/kops/util/pkg/vfs"
	federationv1beta1 "k8s.io/kubernetes/federation/apis/federation/v1beta1"
	"os"
	"strconv"
	"strings"
)

const (
	ProviderName           = "aws-kops"
	DefaultMasterSize      = "t2.medium"
	DefaultNodeSize        = "t2.small"
	DefaultNumberOfMasters = "1"
	DefaultNumberOfNodes   = "1"
	InstanceGroupMaster    = "master"
	InstanceGroupNodes     = "nodes"
	AWSAccessKeyId         = "AWS_ACCESS_KEY_ID"
	AWSSecretAccessKeyId   = "AWS_SECRET_ACCESS_KEY"
	CalicoNetwork          = "calico"
)

var _ provider.Provider = Kops{}

// Sample Data:
// '{ "provider": "aws", "region": "us-east-2", "masterZones": ["us-east-2a"], "nodeZones": ["us-east-2a"], "masterSize": "t2.small", "nodeSize": "t2.micro", "numberOfMasters": "1", "numberOfNodes": "1" }'
type AWSKopsConfig struct {
	Provider         string   `json:"provider,omitempty"`
	Region           string   `json:"region,omitempty"`
	MasterZones      []string `json:"masterZones,omitempty"`
	NodeZones        []string `json:"nodeZones,omitempty"`
	MasterSize       string   `json:"masterSize,omitempty"`
	NodeSize         string   `json:"nodeSize,omitempty"`
	NumberOfMasters  string   `json:"numberOfMasters,omitempty"`
	NumberOfNodes    string   `json:"numberOfNodes,omitempty"`
	MaxNumberOfNodes string   `json:"maxNumberOfNodes,omitempty"`
	MinNumberOfNodes string   `json:"minNumberOfNodes,omitempty"`
	NetworkProvider  string   `json:"networkProvider,omitempty"`
	NodeNetwork      string   `json:"nodeNetwork,omitempty"`
	PodNetwork       string   `json:"podNetwork,omitempty"`
	SshKey           string   `json:"sshkey,omitempty"`
	MaxPods          string   `json:"maxPods,omitempty"`
}

func (clusterConfig *AWSKopsConfig) unMarshal(cluster federationv1beta1.Cluster) {
	if clusterConfigStr, ok := cluster.Annotations[common.ClusterManagerClusterConfigKey]; ok {
		json.Unmarshal([]byte(clusterConfigStr), &clusterConfig)
	}
}

func (clusterConfig *AWSKopsConfig) setDefaultValues(k Kops) {
	if len(clusterConfig.MasterZones) == 0 {
		clusterConfig.MasterZones = k.defaultMasterZones
	}
	if len(clusterConfig.NodeZones) == 0 {
		clusterConfig.NodeZones = k.defaultNodeZones
	}
	if clusterConfig.MasterSize == "" {
		clusterConfig.MasterSize = DefaultMasterSize
	}
	if clusterConfig.NodeSize == "" {
		clusterConfig.NodeSize = DefaultNodeSize
	}
	if clusterConfig.NumberOfMasters == "" {
		clusterConfig.NumberOfMasters = DefaultNumberOfMasters
	}
	if clusterConfig.NumberOfNodes == "" {
		clusterConfig.NumberOfNodes = DefaultNumberOfNodes
	}
	if clusterConfig.MaxPods == "" {
		clusterConfig.MaxPods = "0"
	}
}

// This will wrap low level blocking API calls so they can easily be faked/mocked in a unit test
type toolSetInterface interface {
	getClientset() simple.Clientset
	createCluster(cluster *apiskops.Cluster) error
	getCluster(clusterName string) (*apiskops.Cluster, error)
	cloudUpPerformAssignments(cluster *apiskops.Cluster) error
	createInstanceGroup(cluster *apiskops.Cluster, ig *apiskops.InstanceGroup) error
	getInstanceGroupForNodes(cluster *apiskops.Cluster) (*apiskops.InstanceGroup, error)
	updateInstanceGroup(cluster *apiskops.Cluster, ig *apiskops.InstanceGroup) error
	applyCmdRun(applyCmd *cloudup.ApplyClusterCmd) error
	addSSHPublicKey(keyStore fi.CAStore, sshKey string) error
	getRegistryKeyStore(cluster *apiskops.Cluster) (fi.CAStore, error)
	getRegistrySecretStore(cluster *apiskops.Cluster) (fi.SecretStore, error)
	buildKubeConfig(cluster *apiskops.Cluster, keyStore fi.CAStore, secretStore fi.SecretStore) (*kubeconfig.KubeconfigBuilder, error)
	generateKubeConfig(confBuilder *kubeconfig.KubeconfigBuilder) (*clientcmdapi.Config, error)
	getInstanceGroupList(cluster *apiskops.Cluster) (*apiskops.InstanceGroupList, error)
	validateCluster(clusterName string, list *apiskops.InstanceGroupList, clientSet *kubernetes.Clientset) (*validation.ValidationCluster, error)
	awsupFindRegion(cluster *apiskops.Cluster) (string, error)
	awsupNewAWSCloud(region string, tags map[string]string) (fi.Cloud, error)
	listResources(d *resources.ClusterResources) (map[string]*resources.ResourceTracker, error)
	registryConfigBase(cluster *apiskops.Cluster) (vfs.Path, error)
	registryDeleteAllClusterState(configBase vfs.Path) error
	deleteResources(d *resources.ClusterResources, clusterResources map[string]*resources.ResourceTracker) error
	newConfigBuilder() *kubeconfig.KubeconfigBuilder
	deleteKubeConfig(kConfig *kubeconfig.KubeconfigBuilder) error
}

func init() {
	glog.V(1).Infof("Init for %q provider %q", ProviderName)
	kops.Version = "1.7.0"
}

func osLookup(keyId string) error {
	if value, ok := os.LookupEnv(keyId); !ok {
		return errors.Errorf("%s env is not set, kops provider will be disabled", keyId)
	} else {
		glog.Infof("Using %s=%v", keyId, value)
		return nil
	}
}

func NewAwsKops(config *options.ClusterControllerOptions) (*Kops, error) {
	// Check if AWS Access Keys had been set
	accessKeyIds := []string{AWSAccessKeyId, AWSSecretAccessKeyId}
	for _, accessKeyId := range accessKeyIds {
		if err := osLookup(accessKeyId); err != nil {
			return nil, err
		}
	}

	ksp, err := vfs.Context.BuildVfsPath(config.KopsStateStore)

	if err != nil {
		glog.Errorf("error parsing registry path %v", err)
		return nil, err
	}

	k := &Kops{
		kopsStatePath:      ksp,
		defaultNodeZones:   strings.Split(config.DefaultNodeZones, ","),
		defaultMasterZones: strings.Split(config.DefaultMasterZones, ","),
		domain:             config.Domain,
		toolSet:            newToolSet(ksp),
	}
	return k, nil
}

type Kops struct {
	kopsStatePath      vfs.Path
	defaultNodeZones   []string
	defaultMasterZones []string
	domain             string
	toolSet            toolSetInterface
}

func (k Kops) getClusterName(fedCluster federationv1beta1.Cluster) string {
	return strings.Join([]string{fedCluster.Name, k.domain}, ".")
}

//returns nil as providerId and error
func (k Kops) CreateKubeCluster(newCluster federationv1beta1.Cluster) (*util.ClusterAnnotation, error) {
	glog.V(1).Infof("Submitted Cluster %q for provisioning on %q", newCluster.Name, ProviderName)

	clusterAnnotation, err := k.up(newCluster)

	if err != nil {
		return nil, err
	}

	glog.V(1).Infof("Done with Up, doing Apply at %q ", k.kopsStatePath.Path())

	err = k.apply(newCluster)
	if err != nil {
		return nil, err
	}

	glog.V(1).Infof("Done with Apply at %q ", k.kopsStatePath.Path())

	return clusterAnnotation, nil
}

func (k Kops) GetProviderName() string {
	return ProviderName
}

func (k Kops) ExportKubeConfig(fedCluster federationv1beta1.Cluster) (*clientcmdapi.Config, error) {

	confBuilder, err := k.ExportKubeConfigBuilder(fedCluster.Name)
	if err != nil {
		return nil, err
	}

	return k.toolSet.generateKubeConfig(confBuilder)
}

func (k Kops) ExportKubeConfigBuilder(newClusterName string) (*kubeconfig.KubeconfigBuilder, error) {

	clusterName := strings.Join([]string{newClusterName, k.domain}, ".")
	cluster, err := k.toolSet.getCluster(clusterName)

	if err != nil {
		return nil, err
	}

	keyStore, err := k.toolSet.getRegistryKeyStore(cluster)
	if err != nil {
		return nil, err
	}
	secretStore, err := k.toolSet.getRegistrySecretStore(cluster)
	if err != nil {
		return nil, err
	}

	conf, err := k.toolSet.buildKubeConfig(cluster, keyStore, secretStore)
	if err != nil {
		return nil, err
	}

	return conf, nil
}

func (k Kops) instantiateInstanceGroup(cluster *apiskops.Cluster, icType string, zones []string, size string, numberOfInstances string) error {
	instances, err := strconv.ParseInt(numberOfInstances, 10, 32)
	if err != nil {
		instances = 1
	}
	instanceCount := int32(instances)

	ig := &apiskops.InstanceGroup{}
	ig.ObjectMeta.Name = icType
	var instanceGroupRole apiskops.InstanceGroupRole
	switch icType {
	case InstanceGroupMaster:
		instanceGroupRole = apiskops.InstanceGroupRoleMaster
	case InstanceGroupNodes:
		instanceGroupRole = apiskops.InstanceGroupRoleNode
	default:
		return errors.Errorf("Invalid InstanceGroup type: %s", icType)
	}
	ig.Spec = apiskops.InstanceGroupSpec{
		Role:        instanceGroupRole,
		Subnets:     zones,
		MinSize:     fi.Int32(instanceCount),
		MaxSize:     fi.Int32(instanceCount),
		MachineType: size,
	}

	return k.toolSet.createInstanceGroup(cluster, ig)
}

func (k Kops) up(newCluster federationv1beta1.Cluster) (*util.ClusterAnnotation, error) {

	glog.V(1).Infof("Creating Cluster Config at %q ", k.kopsStatePath.Path())

	clusterName := k.getClusterName(newCluster)
	clusterExists, err := k.toolSet.getCluster(clusterName)

	if err != nil {
		glog.Errorf("Error querying Cluster %s Config at %s", clusterName, k.kopsStatePath.Path())
		return nil, err
	}

	if clusterExists != nil {

		glog.V(1).Infof("Cluster Config for %s already exists at %s", clusterName, k.kopsStatePath.Path())

		return nil, nil
	}

	cluster := &apiskops.Cluster{}
	cluster.ObjectMeta.Name = clusterName
	cluster.Spec = apiskops.ClusterSpec{
		Channel:             "stable",
		CloudProvider:       "aws",
		ConfigBase:          k.kopsStatePath.Join(cluster.ObjectMeta.Name).Path(),
		SSHAccess:           []string{"0.0.0.0/0"},
		KubernetesAPIAccess: []string{"0.0.0.0/0"},
	}

	clusterConfig := AWSKopsConfig{}
	clusterConfig.unMarshal(newCluster)
	clusterConfig.setDefaultValues(k)

	k.setClusterSpecNetworking(clusterConfig, cluster)

	if clusterConfig.NodeNetwork != "" {
		cluster.Spec.NetworkCIDR = clusterConfig.NodeNetwork
	}

	if clusterConfig.PodNetwork != "" {
		cluster.Spec.NonMasqueradeCIDR = clusterConfig.PodNetwork
	}

	cluster.Spec.Topology = &apiskops.TopologySpec{}
	cluster.Spec.Topology.Masters = apiskops.TopologyPublic
	cluster.Spec.Topology.Nodes = apiskops.TopologyPublic

	mp64, _ := strconv.ParseInt(clusterConfig.MaxPods, 10, 32)
	maxPods := int32(mp64)

	k.setClusterMaxPods(maxPods, cluster)

	cluster.Spec.Topology.DNS = &apiskops.DNSSpec{}
	cluster.Spec.Topology.DNS.Type = apiskops.DNSTypePublic

	cluster.Spec.Authorization = &apiskops.AuthorizationSpec{}
	cluster.Spec.Authorization.AlwaysAllow = &apiskops.AlwaysAllowAuthorizationSpec{}

	for _, z := range clusterConfig.NodeZones {
		cluster.Spec.Subnets = append(cluster.Spec.Subnets, apiskops.ClusterSubnetSpec{
			Name: z,
			Zone: z,
			Type: apiskops.SubnetTypePublic,
		})
	}

	for _, etcdClusterName := range cloudup.EtcdClusters {
		etcdCluster := &apiskops.EtcdClusterSpec{
			Name: etcdClusterName,
		}
		for _, masterZone := range clusterConfig.MasterZones {
			etcdMember := &apiskops.EtcdMemberSpec{
				Name:          masterZone,
				InstanceGroup: fi.String(InstanceGroupMaster),
			}
			etcdCluster.Members = append(etcdCluster.Members, etcdMember)
		}
		cluster.Spec.EtcdClusters = append(cluster.Spec.EtcdClusters, etcdCluster)
	}

	if err := k.toolSet.cloudUpPerformAssignments(cluster); err != nil {
		return nil, err
	}

	if err = k.toolSet.createCluster(cluster); err != nil {
		return nil, err
	}

	err = k.instantiateInstanceGroup(cluster, InstanceGroupMaster, clusterConfig.MasterZones, clusterConfig.MasterSize, clusterConfig.NumberOfMasters)
	if err != nil {
		return nil, err
	}
	err = k.instantiateInstanceGroup(cluster, InstanceGroupNodes, clusterConfig.NodeZones, clusterConfig.NodeSize, clusterConfig.NumberOfNodes)
	if err != nil {
		return nil, err
	}

	if clusterConfig.SshKey != "" {
		keyStore, err := k.toolSet.getRegistryKeyStore(cluster)
		if err != nil {
			return nil, err
		}
		glog.Infof("using sshkey: %s", clusterConfig.SshKey)
		err = k.toolSet.addSSHPublicKey(keyStore, clusterConfig.SshKey)
		if err != nil {
			return nil, fmt.Errorf("error addding SSH public key: %v", err)
		}
	}
	clusterAnnotation := &util.ClusterAnnotation{}
	clusterAnnotation.SetAnnotation(navarkospkgcommon.NavarkosClusterNodeCountKey, clusterConfig.NumberOfNodes)

	return clusterAnnotation, nil
}

func (k Kops) setClusterSpecNetworking(clusterConfig AWSKopsConfig, cluster *apiskops.Cluster) {
	if cluster.Spec.Networking == nil {
		cluster.Spec.Networking = &apiskops.NetworkingSpec{}
	}
	if clusterConfig.NetworkProvider != "" {
		if clusterConfig.NetworkProvider == CalicoNetwork {
			cluster.Spec.Networking.Calico = &apiskops.CalicoNetworkingSpec{}
			cluster.Spec.Networking.Calico.CrossSubnet = true
		} else {
			glog.Errorf("Network provider: %q not impl yet - defaulting to Kubenet ", clusterConfig.NetworkProvider)
			cluster.Spec.Networking.Kubenet = &apiskops.KubenetNetworkingSpec{}
		}
	} else {
		cluster.Spec.Networking.Kubenet = &apiskops.KubenetNetworkingSpec{}
	}
}

func (k Kops) setClusterMaxPods(maxPods int32, cluster *apiskops.Cluster) {
	if maxPods > 0 {
		cluster.Spec.Kubelet = &apiskops.KubeletConfigSpec{}
		cluster.Spec.Kubelet.MaxPods = &maxPods
	}
}

func (k Kops) apply(fedCluster federationv1beta1.Cluster) error {

	glog.V(1).Infof("Submitting Cluster %q for provisioning on %q", fedCluster.Name, ProviderName)

	clusterName := k.getClusterName(fedCluster)
	cluster, err := k.toolSet.getCluster(clusterName)

	if err == nil && cluster != nil {
		glog.V(1).Infof("Got cluster def %v", cluster)
	} else if err != nil {
		glog.Errorf("Error reading cluster config from the store - %q", err)
		return err
	}

	if cluster == nil {
		return errors.Errorf("Could not read config for cluster %v from the registry %v", clusterName, k.kopsStatePath.Path())
	}

	applyCmd := &cloudup.ApplyClusterCmd{
		Cluster:         cluster,
		Clientset:       k.toolSet.getClientset(),
		TargetName:      cloudup.TargetDirect,
		DryRun:          false,
		MaxTaskDuration: cloudup.DefaultMaxTaskDuration,
	}
	err = k.toolSet.applyCmdRun(applyCmd)
	if err != nil {
		return err
	}

	return nil
}

func (k Kops) DeleteKubeCluster(existingCluster federationv1beta1.Cluster) error {

	glog.V(1).Infof("Got request to delete Cluster %q  on %q", existingCluster.Name, ProviderName)

	clusterName := k.getClusterName(existingCluster)
	cluster, err := k.toolSet.getCluster(clusterName)

	if err != nil {
		glog.Errorf("Cluster %s encountered error querying Cluster Config at %q", clusterName, k.kopsStatePath.Path())
		return err
	}

	if cluster == nil {
		glog.V(1).Infof("Cluster Config for %q does not exist at %q ", clusterName, k.kopsStatePath.Path())
		return nil
	}

	glog.V(4).Infof("Delete kube cluster found cluster %q, named %q ", cluster, existingCluster.Name)

	var region string
	region, err = k.toolSet.awsupFindRegion(cluster)
	if err != nil {
		return err
	}

	tags := map[string]string{"KubernetesCluster": clusterName}
	cloud, err := k.toolSet.awsupNewAWSCloud(region, tags)
	if err != nil {
		glog.Errorf("error initializing AWS client: %v", err)
		return err
	}

	configBase, err := k.toolSet.registryConfigBase(cluster)
	if err != nil {
		glog.Errorf("config base error : %v", err)
		return err
	}

	glog.V(4).Infof("Delete kube cluster config base %q ", configBase)

	d := &resources.ClusterResources{}
	d.ClusterName = clusterName
	d.Cloud = cloud

	allResources, err := k.toolSet.listResources(d)
	if err != nil {
		glog.Errorf("list resources error : %v", err)
		return err
	}

	clusterResources := k.extractUnsharedClusterResources(allResources)
	if len(clusterResources) > 0 {
		glog.V(1).Infof("Deleting unshared resources\n")

		err = k.toolSet.deleteResources(d, clusterResources)
		if err != nil {
			glog.Errorf("delete resource error : %v", err)
			return err
		}

		err = k.toolSet.registryDeleteAllClusterState(configBase)
		if err != nil {
			glog.Errorf("Delete all kopsCluster state error : %v", err)
			return err
		}
	}

	kConfig := k.toolSet.newConfigBuilder()
	kConfig.Context = clusterName
	err = k.toolSet.deleteKubeConfig(kConfig)
	if err != nil {
		glog.Warningf("Error removing kube config: %v", err)
	}

	glog.V(1).Infof("\nDeleted kopsCluster: %q\n", clusterName)
	return nil
}

func (k Kops) extractUnsharedClusterResources(allResources map[string]*resources.ResourceTracker) map[string]*resources.ResourceTracker {
	clusterResources := make(map[string]*resources.ResourceTracker)
	for k, resource := range allResources {
		if resource.Shared {
			continue
		}
		clusterResources[k] = resource
	}
	return clusterResources
}

/**
 * Scale Cluster Nodes. A positive scaleSize means it is a scale up and a negative value means it is a scale down.
 * The function returns the target node count and error value.
 */
func (k Kops) ScaleKubeCluster(existingCluster federationv1beta1.Cluster, scaleSize int) (*util.ClusterAnnotation, error) {

	glog.V(1).Infof("Submitted cluster: %q for scaling with size %d on %q", existingCluster.Name, scaleSize, ProviderName)

	clusterName := k.getClusterName(existingCluster)
	cluster, err := k.toolSet.getCluster(clusterName)

	if err != nil {
		glog.Errorf("Cluster %s encountered error querying Cluster Config at %q", clusterName, k.kopsStatePath.Path())
		return nil, err
	}
	if cluster == nil {
		glog.V(1).Infof("Cluster Config for %q does not exist at %q ", clusterName, k.kopsStatePath.Path())
		return nil, err
	}

	ig, err := k.toolSet.getInstanceGroupForNodes(cluster)
	if err != nil {
		glog.Errorf("Unable to retrieve instanceGroup for cluster '%s' with error '%v", clusterName, err)
		return nil, err
	}
	if ig == nil {
		return nil, errors.Errorf("Instance group for nodes returned a nil value for cluster: %s ", clusterName)
	}

	var nodeCount = fi.Int32Value(ig.Spec.MaxSize)
	if nodeCount == 1 && scaleSize < 0 {
		glog.V(1).Infof("Cannot scale down cluster %s as the node size is already 1.", clusterName)
		clusterAnnotation := &util.ClusterAnnotation{}
		// Set back to ready status as there is nothing more to do
		clusterAnnotation.SetAnnotation(navarkospkgcommon.NavarkosClusterStateKey, navarkospkgcommon.NavarkosClusterStateReady)
		return clusterAnnotation, nil
	}

	nodeCount += int32(scaleSize)
	if nodeCount < 1 {
		nodeCount = 1
	}
	ig.Spec.MinSize = fi.Int32(nodeCount)
	ig.Spec.MaxSize = fi.Int32(nodeCount)
	err = k.toolSet.updateInstanceGroup(cluster, ig)
	if err != nil {
		glog.Errorf("Unable to update instanceGroup for cluster: %q", clusterName)
		return nil, err
	}

	glog.V(1).Infof("Scaled cluster %s  with size %d resulting to new nodeCount: %d", clusterName, scaleSize, nodeCount)

	err = k.apply(existingCluster)
	var clusterAnnotation *util.ClusterAnnotation
	if err != nil {
		glog.Errorf("Unable to apply changes for cluster: %q", clusterName)
	} else {
		clusterAnnotation = &util.ClusterAnnotation{}
		clusterAnnotation.SetAnnotation(navarkospkgcommon.NavarkosClusterNodeCountKey, strconv.Itoa(int(nodeCount)))
		glog.V(1).Infof("Done with Apply at %q ", k.kopsStatePath.Path())
	}

	return clusterAnnotation, err
}

func (k Kops) ValidateKubeCluster(fedCluster federationv1beta1.Cluster) (*util.ClusterAnnotation, error) {

	glog.V(1).Infof("Validating Cluster %q for provisioning on %q", fedCluster.Name, ProviderName)

	clusterName := k.getClusterName(fedCluster)
	cluster, err := k.toolSet.getCluster(clusterName)

	if err == nil && cluster != nil {
		glog.V(10).Infof("Got cluster def %v", cluster)
	} else if err != nil {
		glog.Errorf("Error reading cluster config from the store - %q", err)
		return nil, err
	}

	if cluster == nil {
		return nil, errors.Errorf("Could not read config for cluster %v from the registry %v", clusterName, k.kopsStatePath.Path())
	}

	list, err := k.toolSet.getInstanceGroupList(cluster)
	if err != nil {
		glog.Errorf("Error getting IGs for cluster  %q:  %v", cluster.Name, err)
		return nil, err
	} else {
		glog.V(3).Infof("Got IGs for cluster %q - %v", cluster.Name, list.Items)
	}

	confBuilder, err := k.ExportKubeConfigBuilder(fedCluster.Name)
	if err != nil {
		glog.Errorf("Unable to export kube config for %q: %v\n", cluster.Name, err)
		return nil, err
	}

	clientConfig, err := confBuilder.BuildRestConfig()
	if err != nil {
		glog.Errorf("Cannot build rest config for %q: %v\n", cluster.Name, err)
		return nil, err
	}

	k8sClient, err := kubernetes.NewForConfig(clientConfig)
	if err != nil {
		glog.Errorf("Cannot build kube api client for %q: %v\n", cluster.Name, err)
		return nil, err
	}

	glog.V(3).Infof("About to run validation on Cluster %q", cluster.Name)

	validationCluster, validationFailed := k.toolSet.validateCluster(cluster.ObjectMeta.Name, list, k8sClient)

	if validationFailed != nil {
		glog.V(3).Infof("Cluster %q validation failed: %v", fedCluster.Name, validationFailed)
		glog.V(10).Infof("Cluster %q validation data: %v", fedCluster.Name, validationCluster)
	} else {
		glog.V(3).Infof("Cluster %q is ready", fedCluster.Name)
	}

	return nil, validationFailed
}
