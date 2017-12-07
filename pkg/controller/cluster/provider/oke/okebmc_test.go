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

package oke

import (
	"bytes"
	"encoding/json"
	"github.com/kubernetes-incubator/cluster-manager/pkg/common"
	"github.com/kubernetes-incubator/cluster-manager/pkg/controller/cluster/options"
	"github.com/kubernetes-incubator/cluster-manager/pkg/controller/cluster/provider/oke/models"
	"github.com/kubernetes-incubator/cluster-manager/pkg/controller/cluster/util"
	navarkospkgcommon "github.com/kubernetes-incubator/navarkos/pkg/common"
	"github.com/pkg/errors"
	"io/ioutil"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/kubernetes/federation/apis/federation/v1beta1"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"testing"
)

func TestNewOke(t *testing.T) {
	options := &options.ClusterControllerOptions{
		DefaultOkeAds: "AD-1,AD-2",
		OkeApiHost:    "api.cluster.us-1.oracledx.com",
	}

	accessKeyIdsAll := []string{OkeBearerToken, OkeAuthGroup, OkeDefaultCloudAuthId}
	accessKeyIdsTwo := []string{OkeBearerToken, OkeAuthGroup}
	accessKeyIdsOne1 := []string{OkeDefaultCloudAuthId}
	accessKeyIdsOne2 := []string{OkeBearerToken}

	tests := []struct {
		accessKeyIds   []string
		expectsSuccess bool
	}{
		{accessKeyIdsAll, true},
		{accessKeyIdsTwo, true},
		{accessKeyIdsOne1, false},
		{accessKeyIdsOne2, false},
	}
	for _, testData := range tests {
		for _, accessKeyId := range accessKeyIdsAll {
			os.Unsetenv(accessKeyId)
		}
		for _, accessKeyId := range testData.accessKeyIds {
			os.Setenv(accessKeyId, accessKeyId)
		}
		oke, err := NewOke(options)
		if testData.expectsSuccess {
			if err != nil {
				t.Errorf("OKE expects no error, but got one: %s!", err)
			}
			if oke == nil {
				t.Errorf("OKE expects a non-nil Kops return value!")
			} else {
				verifyOKEAccessValues(OkeBearerToken, oke.okeBearer, t)
				verifyOKEAccessValues(OkeAuthGroup, oke.okeAuthGroup, t)
				if oke.okeDefaultCloudAuthId != "" {
					verifyOKEAccessValues(OkeDefaultCloudAuthId, oke.okeDefaultCloudAuthId, t)
				}
			}
		} else if err == nil {
			t.Errorf("OKE expects error, but did not get one!")
		}
	}
}

func TestGetProviderName(t *testing.T) {
	oke := &Oke{}
	providerName := oke.GetProviderName()
	if providerName != ProviderName {
		t.Errorf("ProviderName is %s, but expected value is %s!", providerName, ProviderName)
	}
}

func verifyOKEAccessValues(key, setValue string, t *testing.T) {
	passedValue := os.Getenv(key)
	if key == OkeBearerToken {
		passedValue = OkeBearerPrefix + passedValue
	}
	if setValue != passedValue {
		t.Errorf("OKE %s=%s, but expects a value of %s!", key, setValue, passedValue)
	}
}

/*
	Tests the following:
	1. Cluster config annotation parameters are passed on properly to the request
	2. Correct http method ('POST') and URL path ("/api/v1/clusters/")
    3. Custom values for cluster config are set properly
	4. Will succeed when clusterId in the response is not empty, and will fail otherwise
    5. Will fail if the clusterId cannot be unmarshalled from the response
*/
func TestCreateKubeCluster(t *testing.T) {
	newCluster := v1beta1.Cluster{}
	newCluster.Name = "testcluster"

	// '{ "provider": "oke", "nodeZones": "AD-1,AD-2,AD-3", "shape": "VM.Standard1.1", "workersPerAD": "1", "compartment": "TestCompartmentID" }'
	provider := "oke"
	customClusterConfig := &OkeBmcConfig{
		Provider:     provider,
		K8Version:    "v2.00",
		LbType:       "TestLoadBalancer",
		NodeZones:    []string{"AD-1", "AD-2", "AD-3"},
		ImageName:    "TestLinuxImage",
		Shape:        "VM.Standard1.1",
		WorkersPerAD: "3",
		Compartment:  "TestCompartmentID",
		SSHPublicKey: "AAB4MD7hfdhjdwuyuhu!djisishfdfHHSuu test@something.com",
	}
	defaultNodeAds := []string{"TestAD1", "TestAD2"}

	emptyClusterConfig := &OkeBmcConfig{
		Provider: provider,
	}

	expectedclusterId := "TestOkeClusterId"
	expectedWorkItemId := "TestOkeWorkItemId"

	cwiResp := &models.ClusterWorkItemResponse{
		WorkItemID: expectedWorkItemId,
		ClusterID:  expectedclusterId,
	}

	cwiRespByte, _ := cwiResp.MarshalBinary()
	successResponse := string(cwiRespByte)

	cwiRespNoClusterID := &models.ClusterWorkItemResponse{
		WorkItemID: expectedWorkItemId,
	}
	cwiRespByte, _ = cwiRespNoClusterID.MarshalBinary()
	emptyClusterIDResponse := string(cwiRespByte)

	cwiRespNoWorkItemID := &models.ClusterWorkItemResponse{
		ClusterID: expectedclusterId,
	}
	cwiRespByte, _ = cwiRespNoWorkItemID.MarshalBinary()
	emptyWorkItemIDResponse := string(cwiRespByte)

	cannotBeUnmarshalledResponse := "Unmarshal error in response!"
	failedResponse := "The test has failed!"

	var testClusterConfig *OkeBmcConfig
	var testResponse string
	oke := &Oke{
		apiHost:        "okeApiHost",
		defaultNodeAds: defaultNodeAds,
		httpDefaultClientDo: func(req *http.Request) (*http.Response, error) {
			validateHTTPRequest(req, http.MethodPost, OkeClusterEndPoint, t)

			if testResponse == failedResponse {
				return nil, errors.Errorf("An error occurred!")
			}

			clusterCreateRequest := models.ClusterCreateRequest{}
			// Validate that the cluster config values are passed on correctly
			bodyBytes, _ := ioutil.ReadAll(req.Body)
			json.Unmarshal([]byte(bodyBytes), &clusterCreateRequest)

			bmcClusterConfig := *clusterCreateRequest.BmcClusterConfig
			pools := bmcClusterConfig.Pools

			var retrievedNodeZones []string
			retrievedNodeZones = make([]string, len(pools[0].Ads))
			for i, ads := range pools[0].Ads {
				retrievedNodeZones[i] = ads.ID
			}
			retrievedClusterConfig := &OkeBmcConfig{
				Provider:     provider,
				K8Version:    clusterCreateRequest.K8Version,
				LbType:       clusterCreateRequest.LbType,
				NodeZones:    retrievedNodeZones,
				ImageName:    pools[0].ImageName,
				Shape:        pools[0].Shape,
				WorkersPerAD: strconv.Itoa(int(pools[0].WorkersPerAD)),
				Compartment:  bmcClusterConfig.Compartment,
				SSHPublicKey: bmcClusterConfig.SSHPublicKey,
			}
			if retrievedClusterConfig.K8Version != pools[0].K8Version {
				t.Errorf("Pool's K8Version is %s, but expects %s!", pools[0].K8Version, retrievedClusterConfig.K8Version)
			}
			if _, err := strconv.Atoi(testClusterConfig.WorkersPerAD); err != nil {
				testClusterConfig.WorkersPerAD = DefaultWorkersPerAD
			}
			if !reflect.DeepEqual(retrievedClusterConfig, testClusterConfig) {
				t.Errorf("Cluster config is %v, but expects %v!", retrievedClusterConfig, testClusterConfig)
			}
			return &http.Response{Body: ioutil.NopCloser(bytes.NewReader([]byte(testResponse)))}, nil
		},
	}

	tests := []struct {
		clusterConfig  *OkeBmcConfig
		workersPerAd   string
		response       string
		expectsSuccess bool
	}{
		{customClusterConfig, "", successResponse, true},
		{emptyClusterConfig, "", successResponse, true},
		{customClusterConfig, "", cannotBeUnmarshalledResponse, false},
		{customClusterConfig, "", emptyClusterIDResponse, false},
		{customClusterConfig, "", emptyWorkItemIDResponse, false},
		{customClusterConfig, "A", successResponse, true},
		{customClusterConfig, "", failedResponse, false},
	}

	for _, testData := range tests {
		testData.clusterConfig.setDefaultValues(*oke)
		if testData.workersPerAd != "" {
			testData.clusterConfig.WorkersPerAD = testData.workersPerAd
		}
		testClusterConfig = testData.clusterConfig
		testResponse = testData.response
		clusterConfigAnnotation, err := json.Marshal(testData.clusterConfig)
		if err != nil {
			t.Errorf("Unable to unmarshal cluster config annotation with error: %v", err)
			continue
		}
		newCluster.Annotations = make(map[string]string)
		newCluster.Annotations[common.ClusterManagerClusterConfigKey] = string(clusterConfigAnnotation)

		clusterAnnotation, err := oke.CreateKubeCluster(newCluster)
		if testData.expectsSuccess {
			if err != nil {
				t.Errorf("CreateKubeCluster failed with error: %v!", err)
			} else {
				workersPerADStr := testData.clusterConfig.WorkersPerAD
				if workersPerADStr == "" {
					workersPerADStr = DefaultWorkersPerAD
				}
				workersPerAD, _ := strconv.Atoi(workersPerADStr)
				adLen := len(testData.clusterConfig.NodeZones)
				if adLen == 0 {
					adLen = len(defaultNodeAds)
				}

				expectedClusterAnnotation := &util.ClusterAnnotation{}
				expectedClusterAnnotation.SetAnnotation(common.ClusterManagerClusterOKEIdKey, cwiResp.ClusterID)
				expectedClusterAnnotation.SetAnnotation(common.ClusterManagerClusterOKEWorkItemKey, cwiResp.WorkItemID)
				expectedClusterAnnotation.SetAnnotation(navarkospkgcommon.NavarkosClusterNodeCountKey, strconv.Itoa(workersPerAD*adLen))
				if !reflect.DeepEqual(clusterAnnotation, expectedClusterAnnotation) {
					t.Errorf("CreateKubeCluster return value is %v, but the expected result should be %v!", clusterAnnotation, expectedClusterAnnotation)
				}
			}
		} else {
			if err == nil {
				t.Errorf("CreateKubeCluster did not return an error when it should have")
			}
		}

	}
}

/*
	Tests following scenarios:
	1. Cluster has a defined cluster id annotation
    2. Cluster does not have a cluster id annotation
    3. A simulated OkeKubeConfig is returned in the response

	Checks the following:
	1. Correct http method ('DELETE') and URL path ("/api/v1/clusters/" + clusterId + /kubeconfig)
    2. Should not return an error when cluster id annotation exists
	3. Should return an error when there is no cluster id annotation
	4. Should return the correct KubeConfig based on the passed OkeKubeConfig
*/
func TestExportKubeConfig(t *testing.T) {
	expectedOkeKubeConfig := &OkeKubeConfig{
		Kind:       "TestKind",
		APIVersion: "testver1.0",
		Clusters: []OkeCluster{
			{
				Name: "TestOkeCluster",
				Cluster: &clientcmdapi.Cluster{
					LocationOfOrigin: "TesOkeClusterLocation",
					Server:           "TestOkeClusterServer1",
				},
			},
		},
		Users: []OkeUser{
			{
				Name: "TestOkeUser",
				User: &clientcmdapi.AuthInfo{
					LocationOfOrigin:  "TestOkeUserLocation",
					ClientCertificate: "TestOkeUserCertificate",
				},
			},
		},
		Contexts: []OkeContext{
			{
				Name: "TestOkeContext",
				Context: &clientcmdapi.Context{
					LocationOfOrigin: "TestOkeContextLocation",
					Cluster:          "TestOkeContextCluster",
				},
			},
		},
		CurrentContext: "TestCurrentContext",
	}
	okeKubeConfigByte, _ := json.Marshal(expectedOkeKubeConfig)

	invalidKubeConfigByte := []byte("InvalidKubeConfigByte")

	clusterWithId := v1beta1.Cluster{}
	clusterWithId.Name = "testclusterwithid"
	clusterWithId.Annotations = make(map[string]string)
	clusterId := "TestOkeclusterId"
	clusterWithId.Annotations[common.ClusterManagerClusterOKEIdKey] = clusterId

	clusterWithoutId := v1beta1.Cluster{}
	clusterWithoutId.Name = "testclusterwithoutid"

	var responseWithError bool

	var testClusterId string
	var kubeConfigByte []byte
	oke := &Oke{
		apiHost: "okeApiHost",
		httpDefaultClientDo: func(req *http.Request) (*http.Response, error) {
			validateHTTPRequest(req, http.MethodGet, OkeClusterEndPoint+"/"+testClusterId+OkeKubeConfigPath, t)
			var err error
			response := &http.Response{Body: ioutil.NopCloser(bytes.NewReader(kubeConfigByte))}
			if !responseWithError {
			} else {
				err = errors.Errorf("An error occurred!")
			}
			return response, err
		},
	}
	tests := []struct {
		fedCluster        v1beta1.Cluster
		kubeConfigByte    []byte
		responseWithError bool
		expectsSuccess    bool
	}{
		{clusterWithId, okeKubeConfigByte, false, true},
		{clusterWithoutId, okeKubeConfigByte, false, false},
		{clusterWithoutId, okeKubeConfigByte, false, false},
		{clusterWithId, invalidKubeConfigByte, false, false},
		{clusterWithId, invalidKubeConfigByte, true, false},
	}

	for _, testData := range tests {
		testClusterId = testData.fedCluster.Annotations[common.ClusterManagerClusterOKEIdKey]
		kubeConfigByte = testData.kubeConfigByte
		responseWithError = testData.responseWithError

		kubeConfig, err := oke.ExportKubeConfig(testData.fedCluster)

		if testData.expectsSuccess {
			if err != nil {
				t.Errorf("ValidateKubeCluster failed with error: %v!", err)
			} else {
				shimExpectedKubeConf := shimKubeConf(expectedOkeKubeConfig)
				if !reflect.DeepEqual(kubeConfig, shimExpectedKubeConf) {
					t.Errorf("Kube config is %v, but expects %v!", kubeConfig, shimExpectedKubeConf)
				}
			}
		} else if !testData.expectsSuccess && err == nil {
			t.Errorf("ValidateKubeCluster expects an error its response!")
		}
	}
}

/*
	Tests following scenarios:
	1. Cluster has a defined cluster id annotation
    2. Cluster does not have a cluster id annotation

	Checks the following:
	1. Correct http method ('DELETE') and URL path ("/api/v1/clusters/" + clusterId)
    2. Should not return an error when cluster id annotation exists
	3. Should return an error when there is no cluster id annotation
*/
func TestDeleteKubeCluster(t *testing.T) {
	clusterWithId := v1beta1.Cluster{}
	clusterWithId.Name = "testclusterwithid"
	clusterWithId.Annotations = make(map[string]string)
	clusterId := "TestOkeclusterId"
	clusterWithId.Annotations[common.ClusterManagerClusterOKEIdKey] = clusterId

	clusterWithoutId := v1beta1.Cluster{}
	clusterWithoutId.Name = "testclusterwithoutid"

	responseWithError := false

	var testClusterId string
	oke := &Oke{
		apiHost: "okeApiHost",
		httpDefaultClientDo: func(req *http.Request) (*http.Response, error) {
			validateHTTPRequest(req, http.MethodDelete, OkeClusterEndPoint+"/"+testClusterId, t)
			if !responseWithError {
				return &http.Response{Body: ioutil.NopCloser(bytes.NewReader([]byte("DeleteResponse")))}, nil
			} else {
				return nil, errors.Errorf("An error occurred!")
			}
		},
	}
	tests := []struct {
		fedCluster        v1beta1.Cluster
		responseWithError bool
		expectsSuccess    bool
	}{
		{clusterWithId, false, true},
		{clusterWithoutId, false, false},
		{clusterWithId, true, true},
	}

	for _, testData := range tests {
		testClusterId = testData.fedCluster.Annotations[common.ClusterManagerClusterOKEIdKey]
		responseWithError = testData.responseWithError
		err := oke.DeleteKubeCluster(testData.fedCluster)

		if testData.expectsSuccess && err != nil {
			t.Errorf("DeleteKubeCluster failed with error: %v!", err)
		} else if !testData.expectsSuccess && err == nil {
			t.Errorf("DeleteKubeCluster expects an error in its response!")
		}
	}
}

/*
	Checks the following:
	1. Correct http method (1st request is a 'GET' and 2nd is a 'PUT') and URL path ("/api/v1/clusters/" + clusterId + "/pools")
    2. Should not return an error when cluster id annotation exists
	3. Should return an error when there is no cluster id annotation
	4. WorkerPerAD in the Pool is set correctly.
	5. Resulting nodeCount from either a scale up or scale down are correct.
*/
func TestScaleKubeCluster(t *testing.T) {
	clusterWithId := v1beta1.Cluster{}
	clusterWithId.Name = "testclusterwithid"
	clusterWithId.Annotations = make(map[string]string)
	clusterId := "TestOkeclusterId"
	clusterWithId.Annotations[common.ClusterManagerClusterOKEIdKey] = clusterId
	workItemId := "TestWorkItemID"

	clusterWithoutId := v1beta1.Cluster{}
	clusterWithoutId.Name = "testclusterwithoutid"

	ad1 := &models.AdDetails{ID: "AD-1"}
	ad2 := &models.AdDetails{ID: "AD-2"}
	ad3 := &models.AdDetails{ID: "AD-3"}
	testAds := []*models.AdDetails{ad1, ad2, ad3}
	testAdsLength := len(testAds)

	var testExpectedNodeCount int
	var testClusterId string
	var testWorkersPerAD int32
	var sameClusterId bool
	requestOccurrence := 0
	requestOccurrenceWithError := -1
	requestOccurrenceForCannotBeUnmarshalledResponse := -1
	oke := &Oke{
		apiHost: "okeApiHost",
		httpDefaultClientDo: func(req *http.Request) (*http.Response, error) {
			var bodyBytes []byte
			var err error

			// Process forced error scenarios
			switch requestOccurrence {
			case requestOccurrenceWithError:
				return nil, errors.Errorf("An error occurred!")
			case requestOccurrenceForCannotBeUnmarshalledResponse:
				bodyBytes = []byte("Unmarshal error in response!")
				return &http.Response{Body: ioutil.NopCloser(bytes.NewReader(bodyBytes))}, nil
			}

			switch requestOccurrence {
			case 0:
				validateHTTPRequest(req, http.MethodGet, OkeClusterEndPoint+"/"+testClusterId+OkePoolsPath, t)

				clusterPoolsResponse := models.ClusterPoolsResponse{}
				pools := []*models.Pool{{
					Name:         "DEFAULT",
					Shape:        DefaultShape,
					WorkersPerAD: testWorkersPerAD,
					ImageName:    "Oracle-Linux-7.4",
					K8Version:    "v1.7.0",
					Ads:          testAds,
				}}
				clusterPoolsResponse.ResourceOwnerID = "TestResourceOwnerId"
				clusterPoolsResponse.Pools = pools
				bodyBytes, _ = clusterPoolsResponse.MarshalBinary()
			case 1:
				validateHTTPRequest(req, http.MethodPut, OkeClusterEndPoint+"/"+testClusterId+OkePoolsPath, t)

				bodyBytes, _ = ioutil.ReadAll(req.Body)
				clusterPools := models.ClusterPools{}
				err = clusterPools.UnmarshalBinary(bodyBytes)
				if err != nil {
					t.Errorf("Unable to Unmarshal '%s' into clusterPools", string(bodyBytes))
				}
				wAD := int(clusterPools.Pools[0].WorkersPerAD)
				nodesCount := wAD * len(clusterPools.Pools[0].Ads)
				if nodesCount != testExpectedNodeCount {
					t.Errorf("Updated pool's nodeCount of %d is not equal to expected node count of %d!", nodesCount, testExpectedNodeCount)
				}

				cwiResp := &models.ClusterWorkItemResponse{}
				if sameClusterId {
					cwiResp.ClusterID = clusterId
				} else {
					cwiResp.ClusterID = "DifferentClusterId"
				}
				cwiResp.WorkItemID = workItemId
				bodyBytes, err = cwiResp.MarshalBinary()
				if err != nil {
					t.Errorf("Unable to marshal response for ClusterID '%s", clusterId)
				}

			default:
				t.Errorf("Invalid number of request occurrences (%d)!", requestOccurrence)
			}
			requestOccurrence += 1

			return &http.Response{Body: ioutil.NopCloser(bytes.NewReader(bodyBytes))}, nil
		},
	}
	tests := []struct {
		scaleUp                                          bool
		originalWorkersPerAD                             int32
		scaleSize                                        int
		fedCluster                                       v1beta1.Cluster
		sameClusterId                                    bool
		requestOccurrenceWithError                       int
		requestOccurrenceForCannotBeUnmarshalledResponse int
		expectsSuccess                                   bool
	}{
		{true, 3, 2, clusterWithId, true, -1, -1, true},
		{true, 2, 1, clusterWithId, true, -1, -1, true},
		{false, 2, 2, clusterWithId, true, -1, -1, true},
		{false, 3, 4, clusterWithId, true, -1, -1, true},
		{false, 1, 2, clusterWithId, true, -1, -1, true},
		{true, 1, 1, clusterWithoutId, true, -1, -1, false},
		{false, 2, 1, clusterWithoutId, true, -1, -1, false},
		{true, 3, 2, clusterWithId, false, -1, -1, false},
		{true, 3, 2, clusterWithId, true, 0, -1, false},
		{true, 3, 2, clusterWithId, true, 1, -1, false},
		{true, 3, 2, clusterWithId, true, -1, 0, false},
		{true, 3, 2, clusterWithId, true, -1, 1, false},
	}

	// var err error
	for _, testData := range tests {
		requestOccurrence = 0
		testClusterId = testData.fedCluster.Annotations[common.ClusterManagerClusterOKEIdKey]
		testWorkersPerAD = testData.originalWorkersPerAD
		sameClusterId = testData.sameClusterId

		scaleSize := testData.scaleSize
		if testData.scaleUp {
			testExpectedNodeCount = (int(testData.originalWorkersPerAD) + testData.scaleSize) * testAdsLength
		} else {
			wADToProcess := int(testData.originalWorkersPerAD) - testData.scaleSize
			if wADToProcess < 1 {
				wADToProcess = 1
			}
			testExpectedNodeCount = (wADToProcess) * testAdsLength
			scaleSize = 0 - scaleSize
		}

		requestOccurrenceWithError = testData.requestOccurrenceWithError
		requestOccurrenceForCannotBeUnmarshalledResponse = testData.requestOccurrenceForCannotBeUnmarshalledResponse

		clusterAnnotation, err := oke.ScaleKubeCluster(testData.fedCluster, scaleSize)
		expectedClusterAnnotation := &util.ClusterAnnotation{}

		if testData.expectsSuccess {
			if scaleSize < 0 && testData.originalWorkersPerAD == 1 {
				expectedClusterAnnotation.SetAnnotation(navarkospkgcommon.NavarkosClusterStateKey, navarkospkgcommon.NavarkosClusterStateReady)
			} else {
				expectedClusterAnnotation.SetAnnotation(common.ClusterManagerClusterOKEWorkItemKey, workItemId)
				expectedClusterAnnotation.SetAnnotation(navarkospkgcommon.NavarkosClusterNodeCountKey, strconv.Itoa(testExpectedNodeCount))
			}
			if err != nil {
				t.Errorf("ScaleKubeKubeCluster failed with error: %v!", err)
			} else {
				if !reflect.DeepEqual(clusterAnnotation, expectedClusterAnnotation) {
					t.Errorf("ScaleKubeCluster clusterAnnotation return value is %v, but the expected result should be %v!", clusterAnnotation, expectedClusterAnnotation)
				}
			}
		} else if !testData.expectsSuccess && err == nil {
			t.Errorf("ScaleKubeKubeCluster expects an error because clusterId does not exist!")
		}
	}
}

/*
	Tests following scenarios:
	1. Cluster Masters State and Nodes State are both RUNNING. This should result to to no error.
    2. Cluster Masters State and Nodes State are both RUNNING but no ClusterId exists. This should result to an error.
	3. Cluster Masters State is in PENDINGSTATE and Nodes State is RUNNING. This should result to an error.
	4. Cluster Masters State is RUNNING and Nodes State is PROVISIONING. This should result to an error.

	Checks the following:
	1. Correct http method ('GET') and URL path ("/api/v1/clusters/" + clusterId)
    2. Should not return an error when cluster id annotation exists
	3. Should return an error when there is no cluster id annotation
*/
func TestValidateKubeCluster(t *testing.T) {
	clusterWithIdAndWorkItem := v1beta1.Cluster{}
	clusterWithIdAndWorkItem.Name = "testclusterwithid"
	clusterWithIdAndWorkItem.Annotations = make(map[string]string)
	clusterId := "TestOkeclusterId"
	clusterWithIdAndWorkItem.Annotations[common.ClusterManagerClusterOKEIdKey] = clusterId
	workItemId := "TestWorkItemID"
	clusterWithIdAndWorkItem.Annotations[common.ClusterManagerClusterOKEWorkItemKey] = workItemId

	clusterWithoutId := v1beta1.Cluster{}
	clusterWithoutId.Name = "testclusterwithoutid"

	clusterWithIdAndNoWorkItem := v1beta1.Cluster{}
	clusterWithIdAndNoWorkItem.Name = "testclusterwithid"
	clusterWithIdAndNoWorkItem.Annotations = make(map[string]string)
	clusterWithIdAndNoWorkItem.Annotations[common.ClusterManagerClusterOKEIdKey] = clusterId

	expectDeleteWorkItemIDClusterAnnotation := &util.ClusterAnnotation{}
	expectDeleteWorkItemIDClusterAnnotation.DeleteAnnotation(common.ClusterManagerClusterOKEWorkItemKey)

	requestOccurrence := 0
	requestOccurrenceWithError := -1
	requestOccurrenceForCannotBeUnmarshalledResponse := -1

	var testMastersState, testNodesState, testClusterId, testWorkItemState string
	var testCluster v1beta1.Cluster
	oke := &Oke{
		apiHost: "okeApiHost",
		httpDefaultClientDo: func(req *http.Request) (*http.Response, error) {
			var bodyBytes []byte
			var err error

			testClusterId = testCluster.Annotations[common.ClusterManagerClusterOKEIdKey]
			// Go to the next request processor if there is no work item id
			if _, ok := testCluster.Annotations[common.ClusterManagerClusterOKEWorkItemKey]; !ok {
				requestOccurrence++
			}

			// Process forced error scenarios
			switch requestOccurrence {
			case requestOccurrenceWithError:
				return nil, errors.Errorf("An error occurred!")
			case requestOccurrenceForCannotBeUnmarshalledResponse:
				bodyBytes = []byte("Unmarshal error in response!")
				return &http.Response{Body: ioutil.NopCloser(bytes.NewReader(bodyBytes))}, nil
			}

			switch requestOccurrence {
			case 0:
				validateHTTPRequest(req, http.MethodGet, OkeWorkItemEndPoint+"/"+workItemId, t)
				workItemDetails := models.WorkItem{}
				workItemDetails.ID = workItemId
				workItemDetails.StatusString = testWorkItemState // models.WorkItemStatusStringComplete
				var status int32 = 2
				workItemDetails.Status = &status
				workItemDetails.ResourceOwnerID = "TestResourceOwnerID"
				bodyBytes, err = workItemDetails.MarshalBinary()
				if err != nil {
					t.Errorf("Unable to marshal response for ClusterID '%s", clusterId)
				}

			case 1:
				validateHTTPRequest(req, http.MethodGet, OkeClusterEndPoint+"/"+testClusterId, t)

				clusterDetails := models.ClusterDetails{}
				clusterDetails.K8 = &models.Cluster{}
				clusterDetails.K8.ResourceOwnerID = "TestResourceOwnerID"
				clusterDetails.K8.MastersState = testMastersState
				clusterDetails.K8.NodesState = testNodesState
				bodyBytes, err = clusterDetails.MarshalBinary()
				if err != nil {
					t.Errorf("Unable to marshal response for ClusterID '%s", clusterId)
				}

			default:
				t.Errorf("Invalid number of request occurrences (%d)!", requestOccurrence)
			}
			requestOccurrence += 1

			return &http.Response{Body: ioutil.NopCloser(bytes.NewReader(bodyBytes))}, nil
		},
	}
	tests := []struct {
		fedCluster                                       v1beta1.Cluster
		requestOccurrenceWithError                       int
		requestOccurrenceForCannotBeUnmarshalledResponse int
		mastersState                                     string
		nodesState                                       string
		workItemState                                    string
		expectsSuccess                                   bool
	}{
		{clusterWithIdAndWorkItem, -1, -1, models.ClusterMastersStateRUNNING, models.ClusterNodesStateRUNNING, models.WorkItemStatusStringComplete, true},
		{clusterWithIdAndNoWorkItem, -1, -1, models.ClusterMastersStateRUNNING, models.ClusterNodesStateRUNNING, models.WorkItemStatusStringComplete, true},
		{clusterWithoutId, -1, -1, models.ClusterMastersStateRUNNING, models.ClusterNodesStateRUNNING, models.WorkItemStatusStringComplete, false},
		{clusterWithIdAndWorkItem, -1, 1, models.ClusterMastersStatePENDINGSTATE, models.ClusterNodesStateRUNNING, models.WorkItemStatusStringComplete, false},
		{clusterWithIdAndNoWorkItem, -1, -1, models.ClusterMastersStatePENDINGSTATE, models.ClusterNodesStateRUNNING, models.WorkItemStatusStringComplete, false},
		{clusterWithoutId, -1, -1, models.ClusterMastersStateRUNNING, models.ClusterNodesStatePROVISIONING, models.WorkItemStatusStringComplete, false},
		{clusterWithIdAndWorkItem, 0, -1, models.ClusterMastersStateRUNNING, models.ClusterNodesStateRUNNING, models.WorkItemStatusStringComplete, false},
		{clusterWithIdAndWorkItem, 1, -1, models.ClusterMastersStateRUNNING, models.ClusterNodesStateRUNNING, models.WorkItemStatusStringComplete, false},
		{clusterWithIdAndWorkItem, -1, 0, models.ClusterMastersStateRUNNING, models.ClusterNodesStateRUNNING, models.WorkItemStatusStringComplete, false},
		{clusterWithIdAndWorkItem, -1, 1, models.ClusterMastersStateRUNNING, models.ClusterNodesStateRUNNING, models.WorkItemStatusStringComplete, false},
		{clusterWithIdAndWorkItem, -1, -1, models.ClusterMastersStateRUNNING, models.ClusterNodesStateRUNNING, models.WorkItemStatusStringRunning, false},
	}

	for _, testData := range tests {
		requestOccurrence = 0
		testMastersState = testData.mastersState
		testNodesState = testData.nodesState
		testWorkItemState = testData.workItemState
		// testClusterId = test.fedCluster.Annotations[common.ClusterManagerClusterOKEIdKey]
		testCluster = testData.fedCluster

		requestOccurrenceWithError = testData.requestOccurrenceWithError
		requestOccurrenceForCannotBeUnmarshalledResponse = testData.requestOccurrenceForCannotBeUnmarshalledResponse

		clusterAnnotation, err := oke.ValidateKubeCluster(testData.fedCluster)

		if testData.expectsSuccess {
			if err != nil {
				t.Errorf("ValidateKubeCluster failed with error: %v!", err)
			} else {
				// If there is a WorkItemID in the cluster, expect clusterAnnotation to contain DeleteAnnotation for the WorkItem
				// Otherwise if empty, expect the clusterAnnotation to be nil
				if _, ok := testCluster.Annotations[common.ClusterManagerClusterOKEWorkItemKey]; ok {
					if !reflect.DeepEqual(clusterAnnotation, expectDeleteWorkItemIDClusterAnnotation) {
						t.Errorf("ValidateKubeCluster return value is %v, but the expected result should be %v!", clusterAnnotation, expectDeleteWorkItemIDClusterAnnotation)
					}
				} else {
					if clusterAnnotation != nil {
						t.Errorf("ValidateKubeCluster expects returned clusterAnnotation to be nil!")
					}
				}
			}
		} else if !testData.expectsSuccess && err == nil {
			t.Errorf("ValidateKubeCluster expects an error its response!")
		}
	}
}

type FailingReader struct {
}

func (r *FailingReader) Read(b []byte) (n int, err error) {
	return 0, errors.Errorf("Read error occurred!")
}

func TestTransmitRequestErrorConditions(t *testing.T) {
	var requestScenario int
	ReturnsError := 0
	BodyCausesError := 1

	oke := &Oke{
		apiHost: "okeApiHost",
		httpDefaultClientDo: func(req *http.Request) (*http.Response, error) {
			var err error
			var response *http.Response
			switch requestScenario {
			case ReturnsError: // return error
				response = nil
				err = errors.Errorf("An error occurred!")
			case BodyCausesError: // return http.response with bad body
				response = &http.Response{Body: ioutil.NopCloser(&FailingReader{})}
				err = nil
			}
			return response, err
		},
	}
	tests := []struct {
		requestScenario int
	}{
		{0},
		{1},
	}

	for _, testData := range tests {
		requestScenario = testData.requestScenario
		_, err := oke.transmitRequest(nil)
		if err == nil {
			t.Errorf("Result for scenario %d expects an error!", testData.requestScenario)
		}
	}
}

func validateHTTPRequest(req *http.Request, expectedURLMethod string, expectedURLPath string, t *testing.T) {
	if req.Method != expectedURLMethod {
		t.Errorf("Request method is %s but should be '%s'", req.Method, expectedURLMethod)
	}
	if req.URL.Path != expectedURLPath {
		t.Errorf("Request URL path is %s but should be '%s'", req.URL.Path, expectedURLPath)
	}
}
