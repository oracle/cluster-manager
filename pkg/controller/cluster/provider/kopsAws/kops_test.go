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

package kopsAws

import (
	"encoding/json"
	"github.com/kubernetes-incubator/cluster-manager/pkg/common"
	testutils "github.com/kubernetes-incubator/cluster-manager/pkg/common/test"
	"github.com/kubernetes-incubator/cluster-manager/pkg/controller/cluster/options"
	"github.com/kubernetes-incubator/cluster-manager/pkg/controller/cluster/util"
	navarkospkgcommon "github.com/kubernetes-incubator/navarkos/pkg/common"
	"k8s.io/client-go/kubernetes"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	apiskops "k8s.io/kops/pkg/apis/kops"
	"k8s.io/kops/pkg/client/simple"
	"k8s.io/kops/pkg/client/simple/vfsclientset"
	"k8s.io/kops/pkg/kubeconfig"
	"k8s.io/kops/pkg/resources"
	"k8s.io/kops/pkg/validation"
	"k8s.io/kops/upup/pkg/fi"
	"k8s.io/kops/upup/pkg/fi/cloudup"
	"k8s.io/kops/upup/pkg/fi/cloudup/awsup"
	"k8s.io/kops/upup/pkg/fi/secrets"
	"k8s.io/kops/util/pkg/vfs"
	"k8s.io/kubernetes/federation/apis/federation/v1beta1"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

type fakeKopsToolSet struct {
	clientset               simple.Clientset
	k                       *Kops
	kopsCluster             *apiskops.Cluster                     // simulates returning cluster value in getCluster() or createCluster()
	clusterConfig           *AWSKopsConfig                        // simulates the clusterConfig value
	nodeCount               int32                                 // simulates current nodeCount in scaleKubeCluster. If value is less than 0, will simulate returning nil instanceGroup value in getInstanceGroupForNodes()
	scaleSize               int                                   // Simulates scaleSize in scaleKubeCluster
	scaleUp                 bool                                  // Simulates scaleUp if true or scaleDown in scaleKubeCluster
	instanceGroupList       *apiskops.InstanceGroupList           // Simulates returning a value for getInstanceGroupList() called in ValidateKubeCluster()
	resourceTrackerMap      map[string]*resources.ResourceTracker // Simulates retuning a value for listResources() in DeleKubeCluster()
	executionSequenceList   []string                              // Use this to list the order of sequence of the kopsToolSet api calls
	sequenceIndex           *int                                  // This will be used to identify the current index of the executed function in the sequence list. Set this to 0 initially.
	functionWithError       string                                // Use this to identify the function that would return an error
	functionWithErrorNumber int                                   // Use this to identify the instance # of the function that will return the error if there are multiple of them in the sequence
	functionWithErrorCount  int                                   // This will be internally used to count the number of times a function is called to be used by functionWithErrorNumber
	t                       *testing.T
}

func (fkts *fakeKopsToolSet) getClientset() simple.Clientset {
	return fkts.clientset
}

func (fkts *fakeKopsToolSet) createCluster(cluster *apiskops.Cluster) error {
	testutils.CheckExecutionSequence(fkts.executionSequenceList, testutils.CallerfunctionName(), fkts.sequenceIndex, fkts.t)
	if err := testutils.CheckIfFunctionHasError(fkts.functionWithError, testutils.CallerfunctionName(), fkts.functionWithErrorNumber, &fkts.functionWithErrorCount, fkts.t); err != nil {
		return err
	}
	if fkts.kopsCluster == nil {
		fkts.kopsCluster = cluster
	}
	return nil
}

func (fkts *fakeKopsToolSet) getCluster(clusterName string) (*apiskops.Cluster, error) {
	testutils.CheckExecutionSequence(fkts.executionSequenceList, testutils.CallerfunctionName(), fkts.sequenceIndex, fkts.t)
	if err := testutils.CheckIfFunctionHasError(fkts.functionWithError, testutils.CallerfunctionName(), fkts.functionWithErrorNumber, &fkts.functionWithErrorCount, fkts.t); err != nil {
		return nil, err
	}
	if fkts.kopsCluster != nil && clusterName != "" {
		fkts.kopsCluster.Name = clusterName
	}
	return fkts.kopsCluster, nil
}

func (fkts *fakeKopsToolSet) cloudUpPerformAssignments(cluster *apiskops.Cluster) error {
	testutils.CheckExecutionSequence(fkts.executionSequenceList, testutils.CallerfunctionName(), fkts.sequenceIndex, fkts.t)
	if err := testutils.CheckIfFunctionHasError(fkts.functionWithError, testutils.CallerfunctionName(), fkts.functionWithErrorNumber, &fkts.functionWithErrorCount, fkts.t); err != nil {
		return err
	}
	return nil
}

func (fkts *fakeKopsToolSet) createInstanceGroup(cluster *apiskops.Cluster, ig *apiskops.InstanceGroup) error {
	testutils.CheckExecutionSequence(fkts.executionSequenceList, testutils.CallerfunctionName(), fkts.sequenceIndex, fkts.t)
	if err := testutils.CheckIfFunctionHasError(fkts.functionWithError, testutils.CallerfunctionName(), fkts.functionWithErrorNumber, &fkts.functionWithErrorCount, fkts.t); err != nil {
		return err
	}
	if fkts.clusterConfig != nil {
		var configZones, defaultZones []string
		var configSize, defaultSize string
		var configNumberOfInstances, defaultNumberOfInstances string

		switch ig.Spec.Role {
		case apiskops.InstanceGroupRoleMaster:
			configZones = fkts.clusterConfig.MasterZones
			defaultZones = fkts.k.defaultMasterZones
			configSize = fkts.clusterConfig.MasterSize
			defaultSize = DefaultMasterSize
			configNumberOfInstances = fkts.clusterConfig.NumberOfMasters
			defaultNumberOfInstances = DefaultNumberOfMasters
		case apiskops.InstanceGroupRoleNode:
			configZones = fkts.clusterConfig.NodeZones
			defaultZones = fkts.k.defaultNodeZones
			configSize = fkts.clusterConfig.NodeSize
			defaultSize = DefaultNodeSize
			configNumberOfInstances = fkts.clusterConfig.NumberOfNodes
			defaultNumberOfInstances = DefaultNumberOfNodes
		default:
			fkts.t.Errorf("Unknown instance type: %s!", ig.Spec.Role)
		}
		if len(configZones) == 0 {
			if !reflect.DeepEqual(ig.Spec.Subnets, defaultZones) {
				fkts.t.Errorf("Zones for %s is %v but expects the default of %v!", ig.Spec.Role, ig.Spec.Subnets, defaultZones)
			}
		} else if !reflect.DeepEqual(ig.Spec.Subnets, configZones) {
			fkts.t.Errorf("Zones for %s is %v but expects %v!", ig.Spec.Role, ig.Spec.Subnets, configZones)
		}
		if configSize == "" {
			if ig.Spec.MachineType != defaultSize {
				fkts.t.Errorf("Size for %s is %s but expects default of %s!", ig.Spec.Role, ig.Spec.MachineType, defaultSize)
			}
		} else if ig.Spec.MachineType != configSize {
			fkts.t.Errorf("Size for %s is %s but expects %s!", ig.Spec.Role, ig.Spec.MachineType, configSize)
		}
		numberOfInstances := strconv.Itoa(int(*ig.Spec.MaxSize))
		if configNumberOfInstances == "" {
			if numberOfInstances != defaultNumberOfInstances {
				fkts.t.Errorf("Number of Instances for %s is %s but expects default of %s!", ig.Spec.Role, numberOfInstances, defaultNumberOfInstances)
			}
		} else if numberOfInstances != configNumberOfInstances {
			if _, err := strconv.ParseInt(configNumberOfInstances, 10, 32); err != nil {
				// Because the configNumberOfInstances is not numeric, it will be set to "1"
				configNumberOfInstances = "1"
				if numberOfInstances != configNumberOfInstances {
					fkts.t.Errorf("Number of Instances for %s is %s but expects %s!", ig.Spec.Role, numberOfInstances, configNumberOfInstances)
				}
			} else {
				fkts.t.Errorf("Number of Instances for %s is %s but expects %s!", ig.Spec.Role, numberOfInstances, configNumberOfInstances)
			}
		}
	}
	return nil
}

func (fkts *fakeKopsToolSet) getInstanceGroupForNodes(cluster *apiskops.Cluster) (*apiskops.InstanceGroup, error) {
	testutils.CheckExecutionSequence(fkts.executionSequenceList, testutils.CallerfunctionName(), fkts.sequenceIndex, fkts.t)
	if err := testutils.CheckIfFunctionHasError(fkts.functionWithError, testutils.CallerfunctionName(), fkts.functionWithErrorNumber, &fkts.functionWithErrorCount, fkts.t); err != nil {
		return nil, err
	}
	ig := &apiskops.InstanceGroup{}
	ig.Spec.MinSize = &fkts.nodeCount
	// If provided nodeCount is < 0, simulate returning a nil InstanceGroup value
	if fkts.nodeCount >= 0 {
		ig.Spec.MaxSize = ig.Spec.MinSize
	} else {
		ig = nil
	}
	return ig, nil
}

func (fkts *fakeKopsToolSet) updateInstanceGroup(cluster *apiskops.Cluster, ig *apiskops.InstanceGroup) error {
	testutils.CheckExecutionSequence(fkts.executionSequenceList, testutils.CallerfunctionName(), fkts.sequenceIndex, fkts.t)
	if err := testutils.CheckIfFunctionHasError(fkts.functionWithError, testutils.CallerfunctionName(), fkts.functionWithErrorNumber, &fkts.functionWithErrorCount, fkts.t); err != nil {
		return err
	}
	var expectedNodeCount int
	if fkts.scaleUp {
		expectedNodeCount = int(fkts.nodeCount) + fkts.scaleSize
	} else {
		expectedNodeCount = int(fkts.nodeCount) - fkts.scaleSize
	}
	if expectedNodeCount <= 1 {
		expectedNodeCount = 1
	}
	if *ig.Spec.MinSize != *ig.Spec.MaxSize {
		fkts.t.Errorf("Min(%d) and Max(%d) node counts are not equal!", *ig.Spec.MaxSize, *ig.Spec.MaxSize)
	} else if int(*ig.Spec.MaxSize) != expectedNodeCount {
		fkts.t.Errorf("Expected node count is %d while min/max are currently at %d/%d respectively!", expectedNodeCount, *ig.Spec.MaxSize, *ig.Spec.MaxSize)
	}
	return nil
}

func (fkts *fakeKopsToolSet) applyCmdRun(applyCmd *cloudup.ApplyClusterCmd) error {
	testutils.CheckExecutionSequence(fkts.executionSequenceList, testutils.CallerfunctionName(), fkts.sequenceIndex, fkts.t)
	if err := testutils.CheckIfFunctionHasError(fkts.functionWithError, testutils.CallerfunctionName(), fkts.functionWithErrorNumber, &fkts.functionWithErrorCount, fkts.t); err != nil {
		return err
	}
	return nil
}

func (fkts *fakeKopsToolSet) addSSHPublicKey(keyStore fi.CAStore, sshKey string) error {
	testutils.CheckExecutionSequence(fkts.executionSequenceList, testutils.CallerfunctionName(), fkts.sequenceIndex, fkts.t)
	if err := testutils.CheckIfFunctionHasError(fkts.functionWithError, testutils.CallerfunctionName(), fkts.functionWithErrorNumber, &fkts.functionWithErrorCount, fkts.t); err != nil {
		return err
	}
	if fkts.clusterConfig != nil {
		if fkts.clusterConfig.SshKey != "" && sshKey != fkts.clusterConfig.SshKey {
			fkts.t.Errorf("sshKey is %s but expects %s!", sshKey, fkts.clusterConfig.SshKey)
		}
	}
	return nil
}

func (fkts *fakeKopsToolSet) getRegistryKeyStore(cluster *apiskops.Cluster) (fi.CAStore, error) {
	testutils.CheckExecutionSequence(fkts.executionSequenceList, testutils.CallerfunctionName(), fkts.sequenceIndex, fkts.t)
	if err := testutils.CheckIfFunctionHasError(fkts.functionWithError, testutils.CallerfunctionName(), fkts.functionWithErrorNumber, &fkts.functionWithErrorCount, fkts.t); err != nil {
		return nil, err
	}
	return &fi.VFSCAStore{}, nil

}

func (fkts *fakeKopsToolSet) getRegistrySecretStore(cluster *apiskops.Cluster) (fi.SecretStore, error) {
	testutils.CheckExecutionSequence(fkts.executionSequenceList, testutils.CallerfunctionName(), fkts.sequenceIndex, fkts.t)
	if err := testutils.CheckIfFunctionHasError(fkts.functionWithError, testutils.CallerfunctionName(), fkts.functionWithErrorNumber, &fkts.functionWithErrorCount, fkts.t); err != nil {
		return nil, err
	}
	return &secrets.VFSSecretStore{}, nil
}

func (fkts *fakeKopsToolSet) buildKubeConfig(cluster *apiskops.Cluster, keyStore fi.CAStore, secretStore fi.SecretStore) (*kubeconfig.KubeconfigBuilder, error) {
	testutils.CheckExecutionSequence(fkts.executionSequenceList, testutils.CallerfunctionName(), fkts.sequenceIndex, fkts.t)
	if err := testutils.CheckIfFunctionHasError(fkts.functionWithError, testutils.CallerfunctionName(), fkts.functionWithErrorNumber, &fkts.functionWithErrorCount, fkts.t); err != nil {
		return nil, err
	}
	return &kubeconfig.KubeconfigBuilder{}, nil
}

func (fkts *fakeKopsToolSet) generateKubeConfig(confBuilder *kubeconfig.KubeconfigBuilder) (*clientcmdapi.Config, error) {
	testutils.CheckExecutionSequence(fkts.executionSequenceList, testutils.CallerfunctionName(), fkts.sequenceIndex, fkts.t)
	if err := testutils.CheckIfFunctionHasError(fkts.functionWithError, testutils.CallerfunctionName(), fkts.functionWithErrorNumber, &fkts.functionWithErrorCount, fkts.t); err != nil {
		return nil, err
	}
	return &clientcmdapi.Config{}, nil
}

func (fkts *fakeKopsToolSet) getInstanceGroupList(cluster *apiskops.Cluster) (*apiskops.InstanceGroupList, error) {
	testutils.CheckExecutionSequence(fkts.executionSequenceList, testutils.CallerfunctionName(), fkts.sequenceIndex, fkts.t)
	if err := testutils.CheckIfFunctionHasError(fkts.functionWithError, testutils.CallerfunctionName(), fkts.functionWithErrorNumber, &fkts.functionWithErrorCount, fkts.t); err != nil {
		return nil, err
	}
	return fkts.instanceGroupList, nil
}

func (fkts *fakeKopsToolSet) validateCluster(clusterName string, list *apiskops.InstanceGroupList, clientSet *kubernetes.Clientset) (*validation.ValidationCluster, error) {
	testutils.CheckExecutionSequence(fkts.executionSequenceList, testutils.CallerfunctionName(), fkts.sequenceIndex, fkts.t)
	if err := testutils.CheckIfFunctionHasError(fkts.functionWithError, testutils.CallerfunctionName(), fkts.functionWithErrorNumber, &fkts.functionWithErrorCount, fkts.t); err != nil {
		return nil, err
	}
	if !reflect.DeepEqual(list, fkts.instanceGroupList) {
		fkts.t.Errorf("InstanceGroupList (%v) is invalid. Expected value but should be (%v)", list, fkts.instanceGroupList)
	}
	return &validation.ValidationCluster{}, nil
}

func (fkts *fakeKopsToolSet) awsupFindRegion(cluster *apiskops.Cluster) (string, error) {
	testutils.CheckExecutionSequence(fkts.executionSequenceList, testutils.CallerfunctionName(), fkts.sequenceIndex, fkts.t)
	if err := testutils.CheckIfFunctionHasError(fkts.functionWithError, testutils.CallerfunctionName(), fkts.functionWithErrorNumber, &fkts.functionWithErrorCount, fkts.t); err != nil {
		return "", err
	}
	return "us-west", nil
}

func (fkts *fakeKopsToolSet) awsupNewAWSCloud(region string, tags map[string]string) (fi.Cloud, error) {
	testutils.CheckExecutionSequence(fkts.executionSequenceList, testutils.CallerfunctionName(), fkts.sequenceIndex, fkts.t)
	if err := testutils.CheckIfFunctionHasError(fkts.functionWithError, testutils.CallerfunctionName(), fkts.functionWithErrorNumber, &fkts.functionWithErrorCount, fkts.t); err != nil {
		return nil, err
	}
	return &awsup.MockAWSCloud{}, nil
}

func (fkts *fakeKopsToolSet) listResources(d *resources.ClusterResources) (map[string]*resources.ResourceTracker, error) {
	testutils.CheckExecutionSequence(fkts.executionSequenceList, testutils.CallerfunctionName(), fkts.sequenceIndex, fkts.t)
	if err := testutils.CheckIfFunctionHasError(fkts.functionWithError, testutils.CallerfunctionName(), fkts.functionWithErrorNumber, &fkts.functionWithErrorCount, fkts.t); err != nil {
		return nil, err
	}
	// Simulate resource.ClusterResources returning some resources
	resourceTrackerMap := make(map[string]*resources.ResourceTracker)
	if fkts.resourceTrackerMap != nil {
		resourceTrackerMap = fkts.resourceTrackerMap
	}
	return resourceTrackerMap, nil
}

func (fkts *fakeKopsToolSet) registryConfigBase(cluster *apiskops.Cluster) (vfs.Path, error) {
	testutils.CheckExecutionSequence(fkts.executionSequenceList, testutils.CallerfunctionName(), fkts.sequenceIndex, fkts.t)
	if err := testutils.CheckIfFunctionHasError(fkts.functionWithError, testutils.CallerfunctionName(), fkts.functionWithErrorNumber, &fkts.functionWithErrorCount, fkts.t); err != nil {
		return nil, err
	}
	return &vfs.S3Path{}, nil
}

func (fkts *fakeKopsToolSet) registryDeleteAllClusterState(configBase vfs.Path) error {
	testutils.CheckExecutionSequence(fkts.executionSequenceList, testutils.CallerfunctionName(), fkts.sequenceIndex, fkts.t)
	if err := testutils.CheckIfFunctionHasError(fkts.functionWithError, testutils.CallerfunctionName(), fkts.functionWithErrorNumber, &fkts.functionWithErrorCount, fkts.t); err != nil {
		return err
	}
	return nil
}

func (fkts *fakeKopsToolSet) deleteResources(d *resources.ClusterResources, clusterResources map[string]*resources.ResourceTracker) error {
	testutils.CheckExecutionSequence(fkts.executionSequenceList, testutils.CallerfunctionName(), fkts.sequenceIndex, fkts.t)
	if err := testutils.CheckIfFunctionHasError(fkts.functionWithError, testutils.CallerfunctionName(), fkts.functionWithErrorNumber, &fkts.functionWithErrorCount, fkts.t); err != nil {
		return err
	}
	if fkts.resourceTrackerMap != nil {
		unsharedClusterResources := fkts.k.extractUnsharedClusterResources(fkts.resourceTrackerMap)
		if !reflect.DeepEqual(clusterResources, unsharedClusterResources) {
			fkts.t.Errorf("Received clusterResources is (%v), but expected value but should be (%v)", clusterResources, unsharedClusterResources)
		}
	}
	return nil
}

func (fkts *fakeKopsToolSet) newConfigBuilder() *kubeconfig.KubeconfigBuilder {
	testutils.CheckExecutionSequence(fkts.executionSequenceList, testutils.CallerfunctionName(), fkts.sequenceIndex, fkts.t)
	return &kubeconfig.KubeconfigBuilder{}
}

func (fkts *fakeKopsToolSet) deleteKubeConfig(kConfig *kubeconfig.KubeconfigBuilder) error {
	testutils.CheckExecutionSequence(fkts.executionSequenceList, testutils.CallerfunctionName(), fkts.sequenceIndex, fkts.t)
	if err := testutils.CheckIfFunctionHasError(fkts.functionWithError, testutils.CallerfunctionName(), fkts.functionWithErrorNumber, &fkts.functionWithErrorCount, fkts.t); err != nil {
		return err
	}
	return nil
}

func TestNewAwsKops(t *testing.T) {
	accessKeyIdOnly := []string{AWSAccessKeyId}
	accessSecretKeyIdOnly := []string{AWSSecretAccessKeyId}
	bothAccessKeyIds := []string{AWSAccessKeyId, AWSSecretAccessKeyId}
	tests := []struct {
		accessKeyIds   []string
		kopsStateStore string
		expectsSuccess bool
	}{
		{accessKeyIdOnly, "s3://clusterstate/", false},
		{accessSecretKeyIdOnly, "s3://clusterstate/", false},
		{bothAccessKeyIds, "s3://clusterstate/", true},
		{bothAccessKeyIds, "http://InvalidClusterStateFormat", false},
	}

	domain := "domain.net"
	// kopsStateStore := "s3://clusterstate/"
	defaultNodeZonesStr := "us-east-1a, us-east-2a"
	defaultNodeZones := strings.Split(defaultNodeZonesStr, ",")
	defaultMasterZonesStr := "us-west-1a, us-west-2a"
	defaultMasterZones := strings.Split(defaultMasterZonesStr, ",")
	for _, test := range tests {
		os.Unsetenv(AWSAccessKeyId)
		os.Unsetenv(AWSSecretAccessKeyId)
		for _, accessKeyId := range test.accessKeyIds {
			os.Setenv(accessKeyId, accessKeyId)
		}
		k, err := NewAwsKops(&options.ClusterControllerOptions{
			Domain:             domain,
			DefaultMasterZones: defaultMasterZonesStr,
			DefaultNodeZones:   defaultNodeZonesStr,
			KopsStateStore:     test.kopsStateStore,
		})
		if test.expectsSuccess {
			if err != nil {
				t.Errorf("NewAwsKops expects no error, but got one: %s!", err)
			}
			if k == nil {
				t.Errorf("NewAwsKops expects a non-nil Kops return value!")
			} else {
				if k.domain != domain {
					t.Errorf("NewAwsKops domain is %s, but expects a value of %s!", k.domain, domain)
				}
				if k.kopsStatePath.Path() != test.kopsStateStore {
					t.Errorf("NewAwsKops state path is %s, but expects a value of %s!", k.kopsStatePath.Path(), test.kopsStateStore)
				}
				if !reflect.DeepEqual(k.defaultMasterZones, defaultMasterZones) {
					t.Errorf("NewAwsKops defaultMasterZones is %v, but expects a value of %v!", k.defaultMasterZones, defaultMasterZones)
				}
				if !reflect.DeepEqual(k.defaultNodeZones, defaultNodeZones) {
					t.Errorf("NewAwsKops defaultMasterZones is %v, but expects a value of %v!", k.defaultNodeZones, defaultNodeZones)
				}
			}
		} else if err == nil {
			t.Errorf("NewAwsKops expects error, but did not get one!")
		}
	}
}

func TestGetProviderName(t *testing.T) {
	k := &Kops{}
	providerName := k.GetProviderName()
	if providerName != ProviderName {
		t.Errorf("ProviderName is %s, but expected value is %s!", providerName, ProviderName)
	}
}

/*
	Tests following scenarios:
	1. Cluster config parameters are set to custom values
    2. Cluster config parameters are not set so will get default values
	3. A previously created kopsCluster already exists when create operation was executed

	Checks the following:
	1. Correct execution sequence of kopsToolset functions
	2. The functions are getting the correct values (e.g IG values) whether they were set using custom values or unset and assumed the default values
*/
func TestCreateKubeCluster(t *testing.T) {
	successfulOperationSequenceList := []string{
		// up()
		testutils.GetFunctionName((*fakeKopsToolSet).getCluster),
		testutils.GetFunctionName((*fakeKopsToolSet).cloudUpPerformAssignments),
		testutils.GetFunctionName((*fakeKopsToolSet).createCluster),
		testutils.GetFunctionName((*fakeKopsToolSet).createInstanceGroup), // for Master
		testutils.GetFunctionName((*fakeKopsToolSet).createInstanceGroup), // for Nodes
		testutils.GetFunctionName((*fakeKopsToolSet).getRegistryKeyStore),
		testutils.GetFunctionName((*fakeKopsToolSet).addSSHPublicKey),
		// apply()
		testutils.GetFunctionName((*fakeKopsToolSet).getCluster),
		testutils.GetFunctionName((*fakeKopsToolSet).applyCmdRun),
	}
	successfulOperationForDefaultValuesSequenceList := []string{
		// up()
		testutils.GetFunctionName((*fakeKopsToolSet).getCluster),
		testutils.GetFunctionName((*fakeKopsToolSet).cloudUpPerformAssignments),
		testutils.GetFunctionName((*fakeKopsToolSet).createCluster),
		testutils.GetFunctionName((*fakeKopsToolSet).createInstanceGroup), // for Master
		testutils.GetFunctionName((*fakeKopsToolSet).createInstanceGroup), // for Nodes
		// apply()
		testutils.GetFunctionName((*fakeKopsToolSet).getCluster),
		testutils.GetFunctionName((*fakeKopsToolSet).applyCmdRun),
	}
	clusterAlreadyExistSequenceList := []string{
		// up()
		testutils.GetFunctionName((*fakeKopsToolSet).getCluster),
		// apply()
		testutils.GetFunctionName((*fakeKopsToolSet).getCluster),
		testutils.GetFunctionName((*fakeKopsToolSet).applyCmdRun),
	}

	newCluster := v1beta1.Cluster{}
	newCluster.Name = "testcluster"
	customValuesClusterConfig := &AWSKopsConfig{
		Region:          "us-west-1",
		MasterZones:     []string{"us-west-1a", "us-west-1b"},
		NodeZones:       []string{"us-west-1b", "us-west-1c"},
		MasterSize:      "t2.small",
		NodeSize:        "t2.micro",
		NumberOfMasters: "2",
		NumberOfNodes:   "3",
		NodeNetwork:     "1.1.1.1",
		PodNetwork:      "2.2.2.2.2",
		MaxPods:         "1000",
		SshKey:          "AAB4MD7hfdhjdwuyuhu!djisishfdfHHSuu test@something.com",
	}
	defaultValuesClusterConfig := &AWSKopsConfig{
		Region: "us-west-1",
	}

	ksp, _ := vfs.Context.BuildVfsPath("s3://clusterstate")
	k := &Kops{
		kopsStatePath:      ksp,
		defaultNodeZones:   strings.Split("us-west-1a, us-east-1a", ","),
		defaultMasterZones: strings.Split("us-west-1a, us-east-1a", ","),
		domain:             "k8sfed.net",
	}

	tests := []struct {
		kopsCluster           *apiskops.Cluster
		clusterConfig         *AWSKopsConfig
		executionSequenceList []string
	}{
		{nil, customValuesClusterConfig, successfulOperationSequenceList},                  // Test by setting custom values for kopsCluster config
		{nil, defaultValuesClusterConfig, successfulOperationForDefaultValuesSequenceList}, // Test default values by not setting kopsCluster config values
		{&apiskops.Cluster{}, defaultValuesClusterConfig, clusterAlreadyExistSequenceList}, // Test when Cluster already exists
	}

	for _, testData := range tests {
		executionIndex := 0
		k.toolSet = &fakeKopsToolSet{
			clientset:             vfsclientset.NewVFSClientset(ksp),
			k:                     k,
			kopsCluster:           testData.kopsCluster,
			clusterConfig:         testData.clusterConfig,
			executionSequenceList: testData.executionSequenceList,
			sequenceIndex:         &executionIndex,
			t:                     t,
		}

		clusterConfigAnnotation, _ := json.Marshal(testData.clusterConfig)
		newCluster.Annotations = make(map[string]string)
		newCluster.Annotations[common.ClusterManagerClusterConfigKey] = string(clusterConfigAnnotation)

		clusterAnnotation, err := k.CreateKubeCluster(newCluster)
		if err != nil {
			t.Errorf("CreateKubeCluster failed with error: %v!", err)
		}
		testutils.VerifyExecutionSequenceCompleted(testData.executionSequenceList, executionIndex, t)
		if clusterAnnotation != nil {
			numberOfNodes := testData.clusterConfig.NumberOfNodes
			if numberOfNodes == "" {
				numberOfNodes = DefaultNumberOfNodes
			}
			expectedClusterAnnotation := &util.ClusterAnnotation{}
			expectedClusterAnnotation.SetAnnotation(navarkospkgcommon.NavarkosClusterNodeCountKey, numberOfNodes)
			if !reflect.DeepEqual(clusterAnnotation, expectedClusterAnnotation) {
				t.Errorf("CreateKubeCluster clusterAnnotation return value is %v, but the expected result should be %v!", clusterAnnotation, expectedClusterAnnotation)
			}
		}

		/*
			if providerId != "" {
				t.Errorf("CreateKubeCluster does not implement providerID so should be empty, but instead has this value: %s!", providerId)
			}
		*/
	}
}

// Simulate kopsToolSet functions used by CreateKubeCluster, returning errors
func TestCreateKubeClusterErrors(t *testing.T) {
	customValuesClusterConfig := &AWSKopsConfig{
		Region:          "us-west-1",
		MasterZones:     []string{"us-west-1a", "us-west-1b"},
		NodeZones:       []string{"us-west-1b", "us-west-1c"},
		MasterSize:      "t2.small",
		NodeSize:        "t2.micro",
		NumberOfMasters: "2",
		NumberOfNodes:   "3",
		SshKey:          "AAB4MD7hfdhjdwuyuhu!djisishfdfHHSuu test@something.com",
	}

	ksp, _ := vfs.Context.BuildVfsPath("s3://clusterstate")
	k := &Kops{
		kopsStatePath:      ksp,
		defaultNodeZones:   strings.Split("us-west-1a, us-east-1a", ","),
		defaultMasterZones: strings.Split("us-west-1a, us-east-1a", ","),
		domain:             "k8sfed.net",
	}

	tests := []struct {
		functionWithError       string
		functionWithErrorNumber int
	}{
		{testutils.GetFunctionName((*fakeKopsToolSet).getCluster), 1},
		{testutils.GetFunctionName((*fakeKopsToolSet).cloudUpPerformAssignments), 1},
		{testutils.GetFunctionName((*fakeKopsToolSet).createCluster), 1},
		{testutils.GetFunctionName((*fakeKopsToolSet).createInstanceGroup), 1},
		{testutils.GetFunctionName((*fakeKopsToolSet).createInstanceGroup), 2},
		{testutils.GetFunctionName((*fakeKopsToolSet).getRegistryKeyStore), 1},
		{testutils.GetFunctionName((*fakeKopsToolSet).addSSHPublicKey), 1},
		{testutils.GetFunctionName((*fakeKopsToolSet).getCluster), 2},
		{testutils.GetFunctionName((*fakeKopsToolSet).applyCmdRun), 1},
	}

	for i, testData := range tests {
		k.toolSet = &fakeKopsToolSet{
			clientset:               vfsclientset.NewVFSClientset(ksp),
			k:                       k,
			t:                       t,
			kopsCluster:             nil,
			clusterConfig:           customValuesClusterConfig,
			functionWithError:       testData.functionWithError,
			functionWithErrorNumber: testData.functionWithErrorNumber,
		}

		newCluster := v1beta1.Cluster{}
		newCluster.Name = "testcluster-" + strconv.Itoa(i)
		clusterConfigAnnotation, _ := json.Marshal(customValuesClusterConfig)
		newCluster.Annotations = make(map[string]string)
		newCluster.Annotations[common.ClusterManagerClusterConfigKey] = string(clusterConfigAnnotation)

		_, err := k.CreateKubeCluster(newCluster)
		if err == nil {
			t.Errorf("CreateKubeCluster is expected to have error in %s", testData.functionWithError)
		}
	}
}

func TestApplyNoCluster(t *testing.T) {
	newCluster := v1beta1.Cluster{}
	newCluster.Name = "testcluster"
	ksp, _ := vfs.Context.BuildVfsPath("s3://clusterstate")
	k := &Kops{
		kopsStatePath: ksp,
	}
	k.toolSet = &fakeKopsToolSet{
		clientset:   vfsclientset.NewVFSClientset(ksp),
		k:           k,
		kopsCluster: nil,
		t:           t,
	}
	err := k.apply(newCluster)
	if err == nil {
		t.Errorf("apply is expected to have error in")
	}

}

func TestInstantiateInstanceGroupWithInvalidICType(t *testing.T) {
	k := &Kops{
		toolSet: &fakeKopsToolSet{
			t: t,
		},
	}
	if err := k.instantiateInstanceGroup(&apiskops.Cluster{}, "InvalidInstanceGroup", []string{}, "us-west-1b", "3"); err == nil {
		t.Errorf("instantiateInstanceGroup is expected to have an error!")
	}
}

func TestInstantiateInstanceGroupWithInvalidNumberOfInstances(t *testing.T) {
	clusterConfig := &AWSKopsConfig{
		Region:          "us-west-1",
		MasterZones:     []string{"us-west-1a", "us-west-1b"},
		NodeZones:       []string{"us-west-1b", "us-west-1c"},
		MasterSize:      "t2.small",
		NodeSize:        "t2.micro",
		NumberOfMasters: "2",
		NumberOfNodes:   "A",
		SshKey:          "AAB4MD7hfdhjdwuyuhu!djisishfdfHHSuu test@something.com",
	}
	k := &Kops{}
	k.toolSet = &fakeKopsToolSet{
		k:             k,
		t:             t,
		clusterConfig: clusterConfig,
	}

	if err := k.instantiateInstanceGroup(&apiskops.Cluster{}, InstanceGroupNodes, clusterConfig.NodeZones, clusterConfig.NodeSize, clusterConfig.NumberOfNodes); err != nil {
		t.Errorf("instantiateInstanceGroup failed with error: %v!", err)
	}
}

func TestSetClusterSpecNetworking(t *testing.T) {
	tests := []struct {
		networkProvider string
	}{
		{CalicoNetwork},
		{"3rdPartyNetwork"},
		{""},
	}

	for _, testData := range tests {
		clusterConfig := &AWSKopsConfig{
			NetworkProvider: testData.networkProvider,
		}
		k := &Kops{}

		kopsCluster := &apiskops.Cluster{}
		k.setClusterSpecNetworking(*clusterConfig, kopsCluster)

		if testData.networkProvider == CalicoNetwork {
			if kopsCluster.Spec.Networking.Calico == nil || !kopsCluster.Spec.Networking.Calico.CrossSubnet {
				t.Errorf("setClusterSpecNetworking failed because the CalicoNetwork was not set up!")
			}
		} else if kopsCluster.Spec.Networking.Kubenet == nil {
			t.Errorf("setClusterSpecNetworking failed because the KubeNetworking was not set up!")

		}
	}
}

func TestSetClusterMaxPods(t *testing.T) {
	tests := []struct {
		maxPods int32
	}{
		{-1},
		{0},
		{5},
	}

	for _, testData := range tests {
		k := &Kops{}

		kopsCluster := &apiskops.Cluster{}
		k.setClusterMaxPods(testData.maxPods, kopsCluster)

		if testData.maxPods > 0 {
			if kopsCluster.Spec.Kubelet == nil || kopsCluster.Spec.Kubelet.MaxPods == nil || *kopsCluster.Spec.Kubelet.MaxPods != testData.maxPods {
				t.Errorf("setClusterMaxPods failed because Kubelet was not set up!")
			}
		} else if kopsCluster.Spec.Kubelet != nil {
			t.Errorf("setClusterSpecNetworking failed because Kubelet is suppose to be nil!")

		}
	}
}

func TestExportKubeConfig(t *testing.T) {
	successfulOperationSequenceList := []string{
		testutils.GetFunctionName((*fakeKopsToolSet).getCluster),
		testutils.GetFunctionName((*fakeKopsToolSet).getRegistryKeyStore),
		testutils.GetFunctionName((*fakeKopsToolSet).getRegistrySecretStore),
		testutils.GetFunctionName((*fakeKopsToolSet).buildKubeConfig),
		testutils.GetFunctionName((*fakeKopsToolSet).generateKubeConfig),
	}
	ksp, _ := vfs.Context.BuildVfsPath("s3://clusterstate")
	k := &Kops{
		kopsStatePath: ksp,
		domain:        "k8sfed.net",
	}
	executionIndex := 0
	k.toolSet = &fakeKopsToolSet{
		clientset:             vfsclientset.NewVFSClientset(ksp),
		executionSequenceList: successfulOperationSequenceList,
		sequenceIndex:         &executionIndex,
		k:                     k,
		t:                     t,
	}

	fedCluster := v1beta1.Cluster{}
	fedCluster.Name = "testcluster"

	kubeConf, err := k.ExportKubeConfig(fedCluster)
	if err != nil {
		t.Errorf("ExportKubeConfig failed with error: %v - %v!", err, kubeConf)
	}
	testutils.VerifyExecutionSequenceCompleted(successfulOperationSequenceList, executionIndex, t)
}

// Simulate kopsToolSet functions used by ExportKubeConfig, returning errors
func TestExportKubeConfigErrors(t *testing.T) {
	ksp, _ := vfs.Context.BuildVfsPath("s3://clusterstate")
	k := &Kops{
		kopsStatePath: ksp,
		domain:        "k8sfed.net",
	}
	tests := []struct {
		functionWithError       string
		functionWithErrorNumber int
	}{
		{testutils.GetFunctionName((*fakeKopsToolSet).getCluster), 1},
		{testutils.GetFunctionName((*fakeKopsToolSet).getRegistryKeyStore), 1},
		{testutils.GetFunctionName((*fakeKopsToolSet).getRegistrySecretStore), 1},
		{testutils.GetFunctionName((*fakeKopsToolSet).buildKubeConfig), 1},
		{testutils.GetFunctionName((*fakeKopsToolSet).generateKubeConfig), 1},
	}

	for i, testData := range tests {
		k.toolSet = &fakeKopsToolSet{
			k:                       k,
			t:                       t,
			clientset:               vfsclientset.NewVFSClientset(ksp),
			functionWithError:       testData.functionWithError,
			functionWithErrorNumber: testData.functionWithErrorNumber,
		}

		fedCluster := v1beta1.Cluster{}
		fedCluster.Name = "testcluster-" + strconv.Itoa(i)

		_, err := k.ExportKubeConfig(fedCluster)
		if err == nil {
			t.Errorf("ExportKubeConfig is expected to have error in %s", testData.functionWithError)
		}
	}
}

func TestDeleteKubeCluster(t *testing.T) {
	emptyResourceTrackerMap := make(map[string]*resources.ResourceTracker)

	resourceTrackerMapWithAllUnsharedResources := make(map[string]*resources.ResourceTracker)
	resourceTrackerMapWithAllUnsharedResources["Resource-1"] = &resources.ResourceTracker{Shared: false}
	resourceTrackerMapWithAllUnsharedResources["Resource-2"] = &resources.ResourceTracker{Shared: false}

	resourceTrackerMapWithAllSharedResources := make(map[string]*resources.ResourceTracker)
	resourceTrackerMapWithAllSharedResources["Resource-1"] = &resources.ResourceTracker{Shared: true}
	resourceTrackerMapWithAllSharedResources["Resource-2"] = &resources.ResourceTracker{Shared: true}

	resourceTrackerMapWithSharedAndUnsharedResources := make(map[string]*resources.ResourceTracker)
	resourceTrackerMapWithSharedAndUnsharedResources["Resource-1"] = &resources.ResourceTracker{Shared: false}
	resourceTrackerMapWithSharedAndUnsharedResources["Resource-2"] = &resources.ResourceTracker{Shared: true}

	// This happens when there is a shared resources to delete
	completeOperationSequenceList := []string{
		testutils.GetFunctionName((*fakeKopsToolSet).getCluster),
		testutils.GetFunctionName((*fakeKopsToolSet).awsupFindRegion),
		testutils.GetFunctionName((*fakeKopsToolSet).awsupNewAWSCloud),
		testutils.GetFunctionName((*fakeKopsToolSet).registryConfigBase),
		testutils.GetFunctionName((*fakeKopsToolSet).listResources),
		testutils.GetFunctionName((*fakeKopsToolSet).deleteResources),
		testutils.GetFunctionName((*fakeKopsToolSet).registryDeleteAllClusterState),
		testutils.GetFunctionName((*fakeKopsToolSet).newConfigBuilder),
		testutils.GetFunctionName((*fakeKopsToolSet).deleteKubeConfig),
	}

	noClusterSharedResourcesOperationSequenceList := []string{
		testutils.GetFunctionName((*fakeKopsToolSet).getCluster),
		testutils.GetFunctionName((*fakeKopsToolSet).awsupFindRegion),
		testutils.GetFunctionName((*fakeKopsToolSet).awsupNewAWSCloud),
		testutils.GetFunctionName((*fakeKopsToolSet).registryConfigBase),
		testutils.GetFunctionName((*fakeKopsToolSet).listResources),
		testutils.GetFunctionName((*fakeKopsToolSet).newConfigBuilder),
		testutils.GetFunctionName((*fakeKopsToolSet).deleteKubeConfig),
	}

	clusterDoesNotExistSequenceList := []string{
		testutils.GetFunctionName((*fakeKopsToolSet).getCluster),
	}

	ksp, _ := vfs.Context.BuildVfsPath("s3://clusterstate")
	k := &Kops{
		kopsStatePath: ksp,
		domain:        "k8sfed.net",
	}

	kopsCluster := &apiskops.Cluster{}
	tests := []struct {
		kopsCluster           *apiskops.Cluster
		resourceTrackerMap    map[string]*resources.ResourceTracker
		executionSequenceList []string
	}{
		{kopsCluster, emptyResourceTrackerMap, noClusterSharedResourcesOperationSequenceList},
		{kopsCluster, resourceTrackerMapWithAllUnsharedResources, completeOperationSequenceList},
		{kopsCluster, resourceTrackerMapWithAllSharedResources, noClusterSharedResourcesOperationSequenceList},
		{kopsCluster, resourceTrackerMapWithSharedAndUnsharedResources, completeOperationSequenceList},
		{nil, emptyResourceTrackerMap, clusterDoesNotExistSequenceList},
	}

	for i, testData := range tests {
		executionIndex := 0
		k.toolSet = &fakeKopsToolSet{
			clientset:             vfsclientset.NewVFSClientset(ksp),
			executionSequenceList: testData.executionSequenceList,
			sequenceIndex:         &executionIndex,
			kopsCluster:           testData.kopsCluster,
			resourceTrackerMap:    testData.resourceTrackerMap,
			k:                     k,
			t:                     t,
		}

		fedCluster := v1beta1.Cluster{}
		fedCluster.Name = "testcluster-" + strconv.Itoa(i)

		err := k.DeleteKubeCluster(fedCluster)
		if err != nil {
			t.Errorf("DeleteKubeCluster failed with error: %v!", err)
		}
		testutils.VerifyExecutionSequenceCompleted(testData.executionSequenceList, executionIndex, t)
	}
}

// Simulate kopsToolSet functions used by DeleteKubeCluster, returning errors
func TestDeleteKubeClusterErrors(t *testing.T) {
	ksp, _ := vfs.Context.BuildVfsPath("s3://clusterstate")
	k := &Kops{
		kopsStatePath: ksp,
		domain:        "k8sfed.net",
	}
	tests := []struct {
		functionWithError       string
		functionWithErrorNumber int
		expectsError            bool
	}{
		{testutils.GetFunctionName((*fakeKopsToolSet).getCluster), 1, true},
		{testutils.GetFunctionName((*fakeKopsToolSet).awsupFindRegion), 1, true},
		{testutils.GetFunctionName((*fakeKopsToolSet).awsupNewAWSCloud), 1, true},
		{testutils.GetFunctionName((*fakeKopsToolSet).registryConfigBase), 1, true},
		{testutils.GetFunctionName((*fakeKopsToolSet).listResources), 1, true},
		{testutils.GetFunctionName((*fakeKopsToolSet).deleteResources), 1, true},
		{testutils.GetFunctionName((*fakeKopsToolSet).registryDeleteAllClusterState), 1, true},
		{testutils.GetFunctionName((*fakeKopsToolSet).deleteKubeConfig), 1, false},
	}

	resourceTrackerMap := make(map[string]*resources.ResourceTracker)
	resourceTrackerMap["Resource-1"] = &resources.ResourceTracker{}
	resourceTrackerMap["Resource-2"] = &resources.ResourceTracker{}
	for i, testData := range tests {
		// Simulate resource.ClusterResources returining some resources

		k.toolSet = &fakeKopsToolSet{
			clientset:               vfsclientset.NewVFSClientset(ksp),
			kopsCluster:             &apiskops.Cluster{},
			resourceTrackerMap:      resourceTrackerMap,
			k:                       k,
			t:                       t,
			functionWithError:       testData.functionWithError,
			functionWithErrorNumber: testData.functionWithErrorNumber,
		}

		fedCluster := v1beta1.Cluster{}
		fedCluster.Name = "testcluster-" + strconv.Itoa(i)

		err := k.DeleteKubeCluster(fedCluster)
		if testData.expectsError {
			if err == nil {
				t.Errorf("DeleteKubeCluster is expected to have error in %s", testData.functionWithError)
			}
		} else if err != nil {
			t.Errorf("DeleteKubeCluster is expected to have no error in %s, but got one: %v", testData.functionWithError, err)
		}
	}
}

/*
	Checks the following:
	1. Correct execution sequence of kopsToolset functions for:
       a. normal scenarios
	   b. when it is unable to scale down because the node count is already at a minimum.
    2. InstanceGroups MaxNode and MinNode are set correctly.
	3. Resulting clusterAnnotation contains the correct nodeCount from either a scale up or scale down.
	4. Resulting clusterAnnotation contains status of ready if nodeCount at the value of 1 cannot be scaled down anymore.

*/
func TestScaleKubeCluster(t *testing.T) {
	successfulOperationSequenceList := []string{
		// scaling routines
		testutils.GetFunctionName((*fakeKopsToolSet).getCluster),
		testutils.GetFunctionName((*fakeKopsToolSet).getInstanceGroupForNodes),
		testutils.GetFunctionName((*fakeKopsToolSet).updateInstanceGroup),
		// apply()
		testutils.GetFunctionName((*fakeKopsToolSet).getCluster),
		testutils.GetFunctionName((*fakeKopsToolSet).applyCmdRun),
	}
	unableToScaleDownNodeCountSequenceList := []string{
		testutils.GetFunctionName((*fakeKopsToolSet).getCluster),
		testutils.GetFunctionName((*fakeKopsToolSet).getInstanceGroupForNodes),
	}
	instanceGroupIsNilSequenceList := unableToScaleDownNodeCountSequenceList
	clusterDoesNotExistSequenceList := []string{
		testutils.GetFunctionName((*fakeKopsToolSet).getCluster),
	}

	tests := []struct {
		scaleUp               bool
		originalNodeCount     int32
		scaleSize             int
		kopsCluster           *apiskops.Cluster
		executionSequenceList []string
	}{
		{true, 3, 2, &apiskops.Cluster{}, successfulOperationSequenceList},
		{true, 2, 1, &apiskops.Cluster{}, successfulOperationSequenceList},
		{false, 2, 2, &apiskops.Cluster{}, successfulOperationSequenceList},
		{false, 3, 4, &apiskops.Cluster{}, successfulOperationSequenceList},
		{false, 1, 2, &apiskops.Cluster{}, unableToScaleDownNodeCountSequenceList},
		{true, -1, 1, &apiskops.Cluster{}, instanceGroupIsNilSequenceList},
		{true, 3, 2, nil, clusterDoesNotExistSequenceList},
	}

	ksp, _ := vfs.Context.BuildVfsPath("s3://clusterstate")
	k := &Kops{
		kopsStatePath:      ksp,
		defaultNodeZones:   strings.Split("us-west-1a, us-east-1a", ","),
		defaultMasterZones: strings.Split("us-west-1a, us-east-1a", ","),
		domain:             "k8sfed.net",
	}
	for _, testData := range tests {
		executionIndex := 0
		k.toolSet = &fakeKopsToolSet{
			clientset:             vfsclientset.NewVFSClientset(ksp),
			nodeCount:             testData.originalNodeCount,
			kopsCluster:           testData.kopsCluster,
			scaleSize:             testData.scaleSize,
			scaleUp:               testData.scaleUp,
			executionSequenceList: testData.executionSequenceList,
			sequenceIndex:         &executionIndex,
			k:                     k,
			t:                     t,
		}
		var nodeCount, expectedNodeCount int
		var err error

		fedCluster := v1beta1.Cluster{}
		fedCluster.Name = "testcluster"

		scaleSize := testData.scaleSize
		if !testData.scaleUp {
			scaleSize = 0 - scaleSize
		}

		expectedNodeCount = int(testData.originalNodeCount) + scaleSize
		clusterAnnotation, err := k.ScaleKubeCluster(fedCluster, scaleSize)

		testutils.VerifyExecutionSequenceCompleted(testData.executionSequenceList, executionIndex, t)
		// This should return a nil instanceGroup which should result to an error
		if testData.originalNodeCount < 0 {
			if err == nil {
				t.Errorf("ScaleKubeCluster expects an error because instanceGroup is nil!")
			}
		} else {
			if err != nil {
				t.Errorf("ScaleKubeCluster failed with error: %v!", err)
			}
			if testData.kopsCluster != nil {
				if expectedNodeCount < 1 && testData.originalNodeCount > 1 {
					expectedNodeCount = 1
				}
				expectedClusterAnnotation := &util.ClusterAnnotation{}
				if expectedNodeCount < 1 {
					if testData.originalNodeCount == 1 {
						expectedClusterAnnotation.SetAnnotation(navarkospkgcommon.NavarkosClusterStateKey, navarkospkgcommon.NavarkosClusterStateReady)
					}
				} else {
					expectedClusterAnnotation.SetAnnotation(navarkospkgcommon.NavarkosClusterNodeCountKey, strconv.Itoa(expectedNodeCount))
				}
				if !reflect.DeepEqual(clusterAnnotation, expectedClusterAnnotation) {
					t.Errorf("ScaleKubeCluster clusterAnnotation return value is %v, but the expected result should be %v!", clusterAnnotation, expectedClusterAnnotation)
				}
			} else {
				if nodeCount != 0 {
					t.Errorf("Resulting Node Count of %d is suppose to be 0!", nodeCount)
				}
			}
		}
	}
}

func TestScaleKubeClusterErrors(t *testing.T) {
	tests := []struct {
		functionWithError       string
		functionWithErrorNumber int
	}{
		{testutils.GetFunctionName((*fakeKopsToolSet).getCluster), 1},
		{testutils.GetFunctionName((*fakeKopsToolSet).getInstanceGroupForNodes), 1},
		{testutils.GetFunctionName((*fakeKopsToolSet).updateInstanceGroup), 1},
		{testutils.GetFunctionName((*fakeKopsToolSet).getCluster), 2},
		{testutils.GetFunctionName((*fakeKopsToolSet).applyCmdRun), 1},
	}

	ksp, _ := vfs.Context.BuildVfsPath("s3://clusterstate")
	k := &Kops{
		kopsStatePath: ksp,
		domain:        "k8sfed.net",
	}
	for i, testData := range tests {
		k.toolSet = &fakeKopsToolSet{
			clientset:               vfsclientset.NewVFSClientset(ksp),
			kopsCluster:             &apiskops.Cluster{},
			k:                       k,
			t:                       t,
			functionWithError:       testData.functionWithError,
			functionWithErrorNumber: testData.functionWithErrorNumber,
		}

		fedCluster := v1beta1.Cluster{}
		fedCluster.Name = "testcluster-" + strconv.Itoa(i)

		_, err := k.ScaleKubeCluster(fedCluster, 1)
		if err == nil {
			t.Errorf("ScaleKubeCluster is expected to have error in %s", testData.functionWithError)
		}
	}
}

/*
	Checks the following:
	1. Correct execution sequence of kopsToolset functions.
    2. InstanceGroupList retrieved from kopsCluster is correctly passed on to validateCluster().
*/
func TestValidateKubeCluster(t *testing.T) {
	successfulOperationSequenceList := []string{
		// ValidateKubeCluster
		testutils.GetFunctionName((*fakeKopsToolSet).getCluster),
		testutils.GetFunctionName((*fakeKopsToolSet).getInstanceGroupList),
		// ExportKubeConfigBuilder
		testutils.GetFunctionName((*fakeKopsToolSet).getCluster),
		testutils.GetFunctionName((*fakeKopsToolSet).getRegistryKeyStore),
		testutils.GetFunctionName((*fakeKopsToolSet).getRegistrySecretStore),
		testutils.GetFunctionName((*fakeKopsToolSet).buildKubeConfig),
		// ValidateKubeCluster
		testutils.GetFunctionName((*fakeKopsToolSet).validateCluster),
	}
	clusterDoesNotExistSequenceList := []string{
		// ValidateKubeCluster
		testutils.GetFunctionName((*fakeKopsToolSet).getCluster),
	}

	ksp, _ := vfs.Context.BuildVfsPath("s3://clusterstate")
	k := &Kops{
		kopsStatePath: ksp,
		domain:        "k8sfed.net",
	}

	// Simulate instanceGroupList retrieval from kopsCluster
	var machineType string
	instanceGroupItems := make([]apiskops.InstanceGroup, 2)
	for i := 0; i < len(instanceGroupItems); i++ {
		ig := apiskops.InstanceGroup{}
		var instanceGroupRole apiskops.InstanceGroupRole
		if i == 0 {
			ig.ObjectMeta.Name = InstanceGroupMaster
			instanceGroupRole = apiskops.InstanceGroupRoleMaster
			machineType = DefaultMasterSize
		} else {
			ig.ObjectMeta.Name = InstanceGroupNodes
			instanceGroupRole = apiskops.InstanceGroupRoleNode
			machineType = DefaultNodeSize
		}
		var nodeCount int32
		nodeCount = int32(i + 1)
		ig.Spec = apiskops.InstanceGroupSpec{
			Role:        instanceGroupRole,
			Subnets:     []string{string(i + 1)},
			MinSize:     &nodeCount,
			MaxSize:     &nodeCount,
			MachineType: machineType,
		}
		ig.Name = ig.ObjectMeta.Name
		ig.Kind = ig.ObjectMeta.Name
		instanceGroupItems[i] = ig
	}

	tests := []struct {
		kopsCluster           *apiskops.Cluster
		executionSequenceList []string
	}{
		{&apiskops.Cluster{}, successfulOperationSequenceList},
		{nil, clusterDoesNotExistSequenceList},
	}

	instanceGroupList := &apiskops.InstanceGroupList{Items: instanceGroupItems}
	for i, testData := range tests {
		executionIndex := 0
		k.toolSet = &fakeKopsToolSet{
			clientset:             vfsclientset.NewVFSClientset(ksp),
			instanceGroupList:     instanceGroupList,
			executionSequenceList: testData.executionSequenceList,
			sequenceIndex:         &executionIndex,
			k:                     k,
			t:                     t,
			kopsCluster:           testData.kopsCluster,
		}

		fedCluster := v1beta1.Cluster{}
		fedCluster.Name = "testcluster-" + strconv.Itoa(i)

		_, err := k.ValidateKubeCluster(fedCluster)
		if testData.kopsCluster == nil {
			if err == nil {
				t.Errorf("ValidateKubeCluster expects an error!")
			}

		} else if err != nil {
			t.Errorf("ValidateKubeCluster failed with error: %v!", err)
		}
		testutils.VerifyExecutionSequenceCompleted(testData.executionSequenceList, executionIndex, t)
	}
}

// Simulate kopsToolSet functions used by ValidateKubeCluster, returning errors
func TestValidateKubeClusterErrors(t *testing.T) {
	ksp, _ := vfs.Context.BuildVfsPath("s3://clusterstate")
	k := &Kops{
		kopsStatePath: ksp,
		domain:        "k8sfed.net",
	}

	// Simulate instanceGroupList retrieval from kopsCluster
	var machineType string
	instanceGroupItems := make([]apiskops.InstanceGroup, 2)
	for i := 0; i < len(instanceGroupItems); i++ {
		ig := apiskops.InstanceGroup{}
		var instanceGroupRole apiskops.InstanceGroupRole
		if i == 0 {
			ig.ObjectMeta.Name = InstanceGroupMaster
			instanceGroupRole = apiskops.InstanceGroupRoleMaster
			machineType = DefaultMasterSize
		} else {
			ig.ObjectMeta.Name = InstanceGroupNodes
			instanceGroupRole = apiskops.InstanceGroupRoleNode
			machineType = DefaultNodeSize
		}
		var nodeCount int32
		nodeCount = int32(i + 1)
		ig.Spec = apiskops.InstanceGroupSpec{
			Role:        instanceGroupRole,
			Subnets:     []string{string(i + 1)},
			MinSize:     &nodeCount,
			MaxSize:     &nodeCount,
			MachineType: machineType,
		}
		ig.Name = ig.ObjectMeta.Name
		ig.Kind = ig.ObjectMeta.Name
		instanceGroupItems[i] = ig
	}

	tests := []struct {
		functionWithError       string
		functionWithErrorNumber int
	}{
		{testutils.GetFunctionName((*fakeKopsToolSet).getCluster), 1},
		{testutils.GetFunctionName((*fakeKopsToolSet).getInstanceGroupList), 1},
		{testutils.GetFunctionName((*fakeKopsToolSet).getCluster), 2},
		{testutils.GetFunctionName((*fakeKopsToolSet).getRegistryKeyStore), 1},
		{testutils.GetFunctionName((*fakeKopsToolSet).getRegistrySecretStore), 1},
		{testutils.GetFunctionName((*fakeKopsToolSet).buildKubeConfig), 1},
		{testutils.GetFunctionName((*fakeKopsToolSet).validateCluster), 1},
	}

	instanceGroupList := &apiskops.InstanceGroupList{Items: instanceGroupItems}
	for i, testData := range tests {
		k.toolSet = &fakeKopsToolSet{
			clientset:               vfsclientset.NewVFSClientset(ksp),
			instanceGroupList:       instanceGroupList,
			k:                       k,
			t:                       t,
			kopsCluster:             &apiskops.Cluster{},
			functionWithError:       testData.functionWithError,
			functionWithErrorNumber: testData.functionWithErrorNumber,
		}

		fedCluster := v1beta1.Cluster{}
		fedCluster.Name = "testcluster-" + strconv.Itoa(i)

		_, err := k.ValidateKubeCluster(fedCluster)
		if err == nil {
			t.Errorf("ValidateKubeCluster is expected to have error in %s", testData.functionWithError)
		}
	}
}
