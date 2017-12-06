//+build !test

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

// This is the production implementation of all toolsets required by kops.go. This code block will by default be a part
// of the build as long as "-tags test" is not added in go test/build. Otherwise if "-tags test" is added, this code
// will be excluded, so it will also be excluded in the code coverage. This implementation is a way to separate the
// production code which calls 3rd party APIs, from unit test implementation which will mock the behavior of those
// 3rd party API calls. Because they are 3rd party APIs, they don't need to be part of the code coverage.

package kopsAws

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	apiskops "k8s.io/kops/pkg/apis/kops"
	"k8s.io/kops/pkg/apis/kops/registry"
	"k8s.io/kops/pkg/client/simple"
	"k8s.io/kops/pkg/client/simple/vfsclientset"
	"k8s.io/kops/pkg/kubeconfig"
	"k8s.io/kops/pkg/resources"
	"k8s.io/kops/pkg/validation"
	"k8s.io/kops/upup/pkg/fi"
	"k8s.io/kops/upup/pkg/fi/cloudup"
	"k8s.io/kops/upup/pkg/fi/cloudup/awsup"
	"k8s.io/kops/util/pkg/vfs"
)

var _ toolSetInterface = &kopsToolSet{}

type kopsToolSet struct {
	clientset simple.Clientset
}

// kopsToolSet constructor
func newToolSet(kopsStatePath vfs.Path) toolSetInterface {
	return &kopsToolSet{clientset: vfsclientset.NewVFSClientset(kopsStatePath)}
}

func (kts *kopsToolSet) getClientset() simple.Clientset {
	return kts.clientset
}

func (kts *kopsToolSet) createCluster(cluster *apiskops.Cluster) error {
	_, err := kts.clientset.ClustersFor(cluster).Create(cluster)
	return err
}

func (kts *kopsToolSet) getCluster(clusterName string) (*apiskops.Cluster, error) {
	return kts.clientset.GetCluster(clusterName)
}

func (kts *kopsToolSet) cloudUpPerformAssignments(cluster *apiskops.Cluster) error {
	return cloudup.PerformAssignments(cluster)
}

func (kts *kopsToolSet) createInstanceGroup(cluster *apiskops.Cluster, ig *apiskops.InstanceGroup) error {
	_, err := kts.clientset.InstanceGroupsFor(cluster).Create(ig)
	return err
}

func (kts *kopsToolSet) getInstanceGroupForNodes(cluster *apiskops.Cluster) (*apiskops.InstanceGroup, error) {
	return kts.clientset.InstanceGroupsFor(cluster).Get("nodes", metav1.GetOptions{})
}

func (kts *kopsToolSet) updateInstanceGroup(cluster *apiskops.Cluster, ig *apiskops.InstanceGroup) error {
	_, err := kts.clientset.InstanceGroupsFor(cluster).Update(ig)
	return err
}

func (kts *kopsToolSet) addSSHPublicKey(keyStore fi.CAStore, sshKey string) error {
	return keyStore.AddSSHPublicKey(fi.SecretNameSSHPrimary, []byte(sshKey))
}

func (kts *kopsToolSet) applyCmdRun(applyCmd *cloudup.ApplyClusterCmd) error {
	return applyCmd.Run()
}

func (kts *kopsToolSet) getRegistryKeyStore(cluster *apiskops.Cluster) (fi.CAStore, error) {
	return registry.KeyStore(cluster)
}

func (kts *kopsToolSet) getRegistrySecretStore(cluster *apiskops.Cluster) (fi.SecretStore, error) {
	return registry.SecretStore(cluster)
}

func (kts *kopsToolSet) buildKubeConfig(cluster *apiskops.Cluster, keyStore fi.CAStore, secretStore fi.SecretStore) (*kubeconfig.KubeconfigBuilder, error) {
	return kubeconfig.BuildKubecfg(cluster, keyStore, secretStore, &cloudDiscoveryStatusStore{&statusDiscoveryToolSet{}})
}

func (kts *kopsToolSet) generateKubeConfig(confBuilder *kubeconfig.KubeconfigBuilder) (*clientcmdapi.Config, error) {
	return confBuilder.GenerateKubecfg()
}

func (kts *kopsToolSet) getInstanceGroupList(cluster *apiskops.Cluster) (*apiskops.InstanceGroupList, error) {
	return kts.clientset.InstanceGroupsFor(cluster).List(metav1.ListOptions{})
}

func (kts *kopsToolSet) validateCluster(clusterName string, list *apiskops.InstanceGroupList, clientSet *kubernetes.Clientset) (*validation.ValidationCluster, error) {
	return validation.ValidateCluster(clusterName, list, clientSet)
}

func (kts *kopsToolSet) awsupFindRegion(cluster *apiskops.Cluster) (string, error) {
	return awsup.FindRegion(cluster)
}

func (kts *kopsToolSet) awsupNewAWSCloud(region string, tags map[string]string) (fi.Cloud, error) {
	return awsup.NewAWSCloud(region, tags)
}

func (kts *kopsToolSet) listResources(d *resources.ClusterResources) (map[string]*resources.ResourceTracker, error) {
	return d.ListResources()
}

func (kts *kopsToolSet) registryConfigBase(cluster *apiskops.Cluster) (vfs.Path, error) {
	return registry.ConfigBase(cluster)
}

func (kts *kopsToolSet) registryDeleteAllClusterState(configBase vfs.Path) error {
	return registry.DeleteAllClusterState(configBase)
}

func (kts *kopsToolSet) deleteResources(d *resources.ClusterResources, clusterResources map[string]*resources.ResourceTracker) error {
	return d.DeleteResources(clusterResources)
}

func (kts *kopsToolSet) newConfigBuilder() *kubeconfig.KubeconfigBuilder {
	return kubeconfig.NewKubeconfigBuilder()
}

func (kts *kopsToolSet) deleteKubeConfig(kConfig *kubeconfig.KubeconfigBuilder) error {
	return kConfig.DeleteKubeConfig()
}
