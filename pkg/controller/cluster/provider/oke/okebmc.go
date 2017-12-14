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
	"github.com/golang/glog"
	"github.com/kubernetes-incubator/cluster-manager/pkg/common"
	"github.com/kubernetes-incubator/cluster-manager/pkg/controller/cluster/options"
	"github.com/kubernetes-incubator/cluster-manager/pkg/controller/cluster/provider"
	"github.com/kubernetes-incubator/cluster-manager/pkg/controller/cluster/provider/oke/models"
	"github.com/kubernetes-incubator/cluster-manager/pkg/controller/cluster/util"
	navarkospkgcommon "github.com/kubernetes-incubator/navarkos/pkg/common"
	"github.com/pkg/errors"
	"io/ioutil"
	"k8s.io/apimachinery/pkg/runtime"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	federationv1beta1 "k8s.io/kubernetes/federation/apis/federation/v1beta1"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
)

const (
	ProviderName            = "oke"
	OkeBearerToken          = "OKE_BEARER_TOKEN"
	OkeBearerPrefix         = "Bearer "
	OkeAuthGroup            = "OKE_AUTH_GROUP"
	OkeDefaultCloudAuthId   = "OKE_CLOUD_AUTH_ID"
	OkeClusterEndPoint      = "/api/v1/clusters"
	OkeWorkItemEndPoint     = "/api/v1/workItems"
	OkeKubeConfigPath       = "/kubeconfig"
	OkePoolsPath            = "/pools"
	DefaultK8Version        = "v1.7.4"
	DefaultLbType           = "external"
	DefaultWorkersImageName = "Oracle-Linux-7.4"
	DefaultShape            = "VM.Standard1.1"
	DefaultWorkersPerAD     = "1"
)

var _ provider.Provider = Oke{}

// Sample Data:
// '{ "provider": "oke", "nodeZones": "AD-1,AD-2,AD-3", "shape": "VM.Standard1.1", "workersPerAD": "1", "compartment": "CompartmentID" }'
type OkeBmcConfig struct {
	Provider     string   `json:"provider,omitempty"`
	K8Version    string   `json:"k8Version,omitempty"`
	LbType       string   `json:"lbType,omitempty"`
	NodeZones    []string `json:"nodeZones,omitempty"`
	ImageName    string   `json:"imageName,omitempty"`
	Shape        string   `json:"shape,omitempty"`
	WorkersPerAD string   `json:"workersPerAD,omitempty"`
	Compartment  string   `json:"compartment,omitempty"`
	SSHPublicKey string   `json:"sshPublicKey,omitempty"`
}

func (clusterConfig *OkeBmcConfig) unMarshal(cluster federationv1beta1.Cluster) {
	if clusterConfigStr, ok := cluster.Annotations[common.ClusterManagerClusterConfigKey]; ok {
		json.Unmarshal([]byte(clusterConfigStr), &clusterConfig)
	}
}

func (clusterConfig *OkeBmcConfig) setDefaultValues(oke Oke) {
	if clusterConfig.K8Version == "" {
		clusterConfig.K8Version = DefaultK8Version
	}
	if clusterConfig.LbType == "" {
		clusterConfig.LbType = DefaultLbType
	}
	if len(clusterConfig.NodeZones) == 0 {
		clusterConfig.NodeZones = oke.defaultNodeAds
	}
	if clusterConfig.ImageName == "" {
		clusterConfig.ImageName = DefaultWorkersImageName
	}
	if clusterConfig.Shape == "" {
		clusterConfig.Shape = DefaultShape
	}
	if clusterConfig.WorkersPerAD == "" {
		clusterConfig.WorkersPerAD = DefaultWorkersPerAD
	}
}

type httpClientDoInterface func(req *http.Request) (*http.Response, error)

type Oke struct {
	defaultNodeAds        []string
	okeBearer             string
	okeAuthGroup          string
	okeDefaultCloudAuthId string
	apiHost               string
	httpDefaultClientDo   httpClientDoInterface
}

func NewOke(config *options.ClusterControllerOptions) (*Oke, error) {

	oke := &Oke{
		defaultNodeAds:      strings.Split(config.DefaultOkeAds, ","),
		apiHost:             config.OkeApiHost,
		httpDefaultClientDo: http.DefaultClient.Do,
	}

	if bearer, ok := os.LookupEnv(OkeBearerToken); ok {
		oke.okeBearer = OkeBearerPrefix + bearer
	} else {
		return nil, errors.Errorf("Env var %v is required by OKE provider, OKE clusters will not be provisioned", OkeBearerToken)
	}

	if authGroup, ok := os.LookupEnv(OkeAuthGroup); ok {
		oke.okeAuthGroup = authGroup
	} else {
		return nil, errors.Errorf("Env var %v is required by OKE provider, OKE clusters will not be provisioned", OkeAuthGroup)
	}
	if defaultCloudAuthId, ok := os.LookupEnv(OkeDefaultCloudAuthId); ok {
		oke.okeDefaultCloudAuthId = defaultCloudAuthId
	}

	return oke, nil
}

func (oke Oke) GetProviderName() string {
	return ProviderName
}

func (oke Oke) CreateKubeCluster(newCluster federationv1beta1.Cluster) (*util.ClusterAnnotation, error) {
	glog.V(1).Infof("Got cluster %s to provision in OKE cloud", newCluster.Name)

	clusterConfig := OkeBmcConfig{}
	clusterConfig.unMarshal(newCluster)
	clusterConfig.setDefaultValues(oke)

	var ads []*models.AdDetails
	for _, adName := range clusterConfig.NodeZones {
		ads = append(ads, &models.AdDetails{
			ID:     adName,
			Subnet: "",
		})
	}

	workersPerAdParseInt, err := strconv.ParseInt(clusterConfig.WorkersPerAD, 10, 32)
	workersPerAd := int32(workersPerAdParseInt)
	if err != nil {
		workersPerAdInt64, _ := strconv.ParseInt(DefaultWorkersPerAD, 10, 32)
		workersPerAd = int32(workersPerAdInt64)
	}

	defaultPool := &models.Pool{
		Name:         "DEFAULT",
		Shape:        clusterConfig.Shape,
		WorkersPerAD: workersPerAd,
		ImageName:    clusterConfig.ImageName,
		K8Version:    clusterConfig.K8Version,
		Ads:          ads,
	}
	pools := []*models.Pool{defaultPool}

	createClusterBody := models.ClusterCreateRequest{
		Name:         newCluster.Name,
		CloudType:    "bmc",
		K8Version:    clusterConfig.K8Version,
		LbType:       clusterConfig.LbType,
		CloudAuthID:  oke.okeDefaultCloudAuthId,
		LinuxVersion: "Oracle Linux",
		BmcClusterConfig: &models.BMCClusterConfig{
			Compartment:  clusterConfig.Compartment,
			SSHPublicKey: clusterConfig.SSHPublicKey,
			Pools:        pools,
		},
	}
	nodesCount := int(workersPerAd) * len(ads)

	body, err := json.Marshal(createClusterBody)
	if err != nil {
		return nil, err
	}

	bodyBytes, err := oke.sendRequestWithPayload(http.MethodPost, OkeClusterEndPoint, body)
	if err != nil {
		return nil, err
	}

	cwiResp := &models.ClusterWorkItemResponse{}
	err = cwiResp.UnmarshalBinary(bodyBytes)

	var clusterAnnotation *util.ClusterAnnotation
	if err != nil {
		glog.Errorf("Unable to unmarshal cluster %s work item response '%s' with error '%v'", newCluster.Name, string(bodyBytes), err)
	} else if cwiResp.ClusterID == "" {
		err = errors.Errorf("ClusterId is not specified for %s", newCluster.Name)
		glog.Errorf(err.Error())
	} else if cwiResp.WorkItemID == "" {
		err = errors.Errorf("WorkItemID is not specified for %s", newCluster.Name)
		glog.Errorf(err.Error())
	} else {
		glog.V(1).Infof("Cluster %s with ClusterId %s and WorkItemId %s is currently provisioning in OKE cloud",
			newCluster.Name, cwiResp.ClusterID, cwiResp.WorkItemID)
		clusterAnnotation = &util.ClusterAnnotation{}
		clusterAnnotation.SetAnnotation(common.ClusterManagerClusterOKEIdKey, cwiResp.ClusterID)
		clusterAnnotation.SetAnnotation(common.ClusterManagerClusterOKEWorkItemKey, cwiResp.WorkItemID)
		clusterAnnotation.SetAnnotation(navarkospkgcommon.NavarkosClusterNodeCountKey, strconv.Itoa(nodesCount))
	}

	return clusterAnnotation, err
}

type OkeKubeConfig struct {
	Kind           string                   `json:"kind,omitempty"`
	APIVersion     string                   `json:"apiVersion,omitempty"`
	Preferences    clientcmdapi.Preferences `json:"preferences"`
	Clusters       []OkeCluster             `json:"clusters"`
	Users          []OkeUser                `json:"users"`
	Contexts       []OkeContext             `json:"contexts"`
	CurrentContext string                   `json:"current-context"`
	Extensions     []runtime.Object         `json:"extensions,omitempty"`
}

type OkeCluster struct {
	Name    string                `json:"name"`
	Cluster *clientcmdapi.Cluster `json:"cluster"`
}

type OkeContext struct {
	Name    string                `json:"name"`
	Context *clientcmdapi.Context `json:"context"`
}

type OkeUser struct {
	Name string                 `json:"name"`
	User *clientcmdapi.AuthInfo `json:"user"`
}

func (oke Oke) ExportKubeConfig(fedCluster federationv1beta1.Cluster) (*clientcmdapi.Config, error) {
	if clusterId, ok := fedCluster.Annotations[common.ClusterManagerClusterOKEIdKey]; ok {
		path := OkeClusterEndPoint + "/" + clusterId + OkeKubeConfigPath
		bodyBytes, err := oke.sendRequest(http.MethodGet, path)
		bodyString := string(bodyBytes)
		if err != nil {
			glog.Errorf("ExportKubeConfig failed with error '%v' and response body of %s", err, bodyString)
			return nil, err
		}

		var config OkeKubeConfig
		err = json.Unmarshal([]byte(bodyString), &config)

		if err != nil {
			glog.Errorf("Unable to unmarshal cluster %s KubeConfig response '%s' with error '%v'", fedCluster.Name, bodyString, err)
			return nil, err
		}

		return shimKubeConf(&config), nil

	} else {
		return nil, errors.Errorf("ClusterId is not specified for %v", fedCluster.Name)

	}
}

func shimKubeConf(okeCfg *OkeKubeConfig) *clientcmdapi.Config {
	conf := clientcmdapi.Config{}
	conf.Kind = okeCfg.Kind
	conf.APIVersion = okeCfg.APIVersion
	conf.Preferences = okeCfg.Preferences
	conf.CurrentContext = okeCfg.CurrentContext
	conf.Contexts = make(map[string]*clientcmdapi.Context)
	conf.Clusters = make(map[string]*clientcmdapi.Cluster)
	conf.AuthInfos = make(map[string]*clientcmdapi.AuthInfo)

	for _, okeContext := range okeCfg.Contexts {
		conf.Contexts[okeContext.Name] = okeContext.Context
	}

	for _, okeCluster := range okeCfg.Clusters {
		conf.Clusters[okeCluster.Name] = okeCluster.Cluster
	}

	for _, okeUser := range okeCfg.Users {
		conf.AuthInfos[okeUser.Name] = okeUser.User
	}

	return &conf
}

func (oke Oke) DeleteKubeCluster(existingCluster federationv1beta1.Cluster) error {
	if clusterId, ok := existingCluster.Annotations[common.ClusterManagerClusterOKEIdKey]; ok {
		glog.V(1).Infof("Got request to delete Cluster %s with ClusterId %s on %s", existingCluster.Name, clusterId, ProviderName)

		path := OkeClusterEndPoint + "/" + clusterId
		bodyBytes, err := oke.sendRequest(http.MethodDelete, path)
		if err != nil {
			glog.Errorf("DeleteKubeCluster failed with error '%v' and response body of %s", err, string(bodyBytes))
		}
		return nil

	} else {
		err := errors.Errorf("Attempt to delete but ClusterId is not specified for %v", existingCluster.Name)
		glog.Errorf(err.Error())
		return err
	}
}

/**
 * Scale Cluster Nodes. A positive scaleSize means it is a scale up and a negative value means it is a scale down.
 * The function returns the target node count and error value.
 */
func (oke Oke) ScaleKubeCluster(existingCluster federationv1beta1.Cluster, scaleSize int) (*util.ClusterAnnotation, error) {
	if clusterId, ok := existingCluster.Annotations[common.ClusterManagerClusterOKEIdKey]; ok {
		glog.V(1).Infof("Submitted cluster %s with clusterId of %s for scaling with size %d on %s",
			existingCluster.Name, clusterId, scaleSize, ProviderName)

		path := OkeClusterEndPoint + "/" + clusterId + OkePoolsPath
		bodyBytes, err := oke.sendRequest(http.MethodGet, path)
		if err != nil {
			return nil, err
		}

		glog.V(4).Info("Receiving pool data of '%s'", string(bodyBytes))

		clusterPoolsResponse := models.ClusterPoolsResponse{}
		err = clusterPoolsResponse.UnmarshalBinary(bodyBytes)
		if err != nil {
			return nil, err
		}

		workersPerAd := clusterPoolsResponse.Pools[0].WorkersPerAD

		if workersPerAd == 1 && scaleSize < 0 {
			glog.V(1).Infof("Cannot scale down Cluster %s as the Workers per Availability Domain size is already 1.", existingCluster.Name)
			clusterAnnotation := &util.ClusterAnnotation{}
			// Set back to ready status as there is nothing more to do
			clusterAnnotation.SetAnnotation(navarkospkgcommon.NavarkosClusterStateKey, navarkospkgcommon.NavarkosClusterStateReady)
			return clusterAnnotation, nil
		}

		workersPerAd += int32(scaleSize)
		if workersPerAd < 1 {
			workersPerAd = 1
		}

		clusterPools := models.ClusterPools{}
		clusterPools.ClusterID = clusterId
		clusterPools.Pools = clusterPoolsResponse.Pools
		clusterPools.Pools[0].WorkersPerAD = workersPerAd

		b, err := clusterPools.MarshalBinary()
		if err != nil {
			return nil, err
		}

		bodyBytes, err = oke.sendRequestWithPayload(http.MethodPut, path, b)
		if err != nil {
			return nil, err
		}

		cwiResp := &models.ClusterWorkItemResponse{}
		err = cwiResp.UnmarshalBinary(bodyBytes)
		if err != nil {
			glog.Errorf("Unable to unmarshal cluster %s WorkItem response '%s' with error '%v'", existingCluster.Name, string(bodyBytes), err)
			return nil, err
		} else if cwiResp.ClusterID == clusterId {
			nodesCount := int(workersPerAd) * len(clusterPoolsResponse.Pools[0].Ads)

			glog.V(1).Infof("Successfully scaled Cluster %s with ClusterId %s and size of %d resulting to new nodeCount: %d and workItemId: %s",
				existingCluster.Name, clusterId, scaleSize, nodesCount, cwiResp.WorkItemID)
			var clusterAnnotation *util.ClusterAnnotation
			if cwiResp.WorkItemID != "" {
				clusterAnnotation = &util.ClusterAnnotation{}
				clusterAnnotation.SetAnnotation(common.ClusterManagerClusterOKEWorkItemKey, cwiResp.WorkItemID)
				clusterAnnotation.SetAnnotation(navarkospkgcommon.NavarkosClusterNodeCountKey, strconv.Itoa(nodesCount))
			}
			return clusterAnnotation, nil
		} else {
			return nil, errors.Errorf("Unable to scale Cluster %s because original ClusterId %s is not the same as the ClusterId %s in the response!",
				existingCluster.Name, clusterId, cwiResp.ClusterID)
		}
	} else {
		err := errors.Errorf("Attempt to scale with with size of %d but ClusterId is not specified for %v", scaleSize, existingCluster.Name)
		glog.Errorf(err.Error())
		return nil, err
	}
}

func (oke Oke) ValidateKubeCluster(fedCluster federationv1beta1.Cluster) (*util.ClusterAnnotation, error) {
	var clusterAnnotation *util.ClusterAnnotation
	if workItemId, ok := fedCluster.Annotations[common.ClusterManagerClusterOKEWorkItemKey]; ok {
		glog.V(1).Infof("Checking Cluster %s with WorkItem of %s on %s has completed",
			fedCluster.Name, workItemId, ProviderName)
		path := OkeWorkItemEndPoint + "/" + workItemId
		bodyBytes, err := oke.sendRequest(http.MethodGet, path)
		if err != nil {
			return nil, err
		}
		glog.V(4).Info("Receiving work item detail response of '%s'", string(bodyBytes))

		workItemDetails := models.WorkItem{}
		err = workItemDetails.UnmarshalBinary(bodyBytes)
		if err != nil {
			glog.Errorf("Unable to unmarshal cluster %s workItemDetails response '%s' with error '%v'", fedCluster.Name, string(bodyBytes), err)
			return nil, err
		}
		if workItemDetails.StatusString == models.WorkItemStatusStringComplete {
			clusterAnnotation = &util.ClusterAnnotation{}
			// Delete WorkItemID as it has already completed
			clusterAnnotation.DeleteAnnotation(common.ClusterManagerClusterOKEWorkItemKey)
		} else {
			err = errors.Errorf("Cluster %s with WorkItem %s has not completed processing. Current status is %s.",
				fedCluster.Name, workItemId, workItemDetails.StatusString)
			glog.Errorf(err.Error())
			return nil, err
		}
	}
	if clusterId, ok := fedCluster.Annotations[common.ClusterManagerClusterOKEIdKey]; ok {
		glog.V(1).Infof("Validating Cluster %s with ClusterId of %s on %s",
			fedCluster.Name, clusterId, ProviderName)

		path := OkeClusterEndPoint + "/" + clusterId
		bodyBytes, err := oke.sendRequest(http.MethodGet, path)
		if err != nil {
			return clusterAnnotation, err
		}
		glog.V(4).Info("Receiving cluster details response data of '%s'", string(bodyBytes))

		clusterDetails := models.ClusterDetails{}
		err = clusterDetails.UnmarshalBinary(bodyBytes)
		if err != nil {
			glog.Errorf("Unable to unmarshal cluster %s ClusterDetails response '%s' with error '%v'", fedCluster.Name, string(bodyBytes), err)
			return clusterAnnotation, err
		}

		if clusterDetails.K8.MastersState == models.ClusterMastersStateRUNNING && clusterDetails.K8.NodesState == models.ClusterNodesStateRUNNING {
			glog.V(1).Infof("Validate successful for ClusterId %s", clusterId)
			err = nil
		} else {
			err = errors.Errorf("Cluster %s with ClusterId %s has not completed processing. Master has %s state, while Nodes has %s state.",
				fedCluster.Name, clusterId, clusterDetails.K8.MastersState, clusterDetails.K8.NodesState)
			glog.Errorf(err.Error())
		}
		return clusterAnnotation, err
	} else {
		err := errors.Errorf("Attempt to validate cluster %s, but ClusterID is not specified", fedCluster.Name)
		glog.Errorf(err.Error())
		return clusterAnnotation, err
	}
}

func (oke Oke) buildBaseRequest(method string, path string) *http.Request {
	request := &http.Request{
		Header: make(http.Header),
		Method: method,
		URL: &url.URL{
			Host:   oke.apiHost,
			Path:   path,
			Scheme: "https",
		},
	}

	request.Header.Set("Accept", "application/json")
	request.Header.Set("AuthorizationGroup", oke.okeAuthGroup)
	request.Header.Set("Authorization", oke.okeBearer)

	return request
}

func (oke Oke) buildRequestWithBody(method string, path string, body *bytes.Buffer) *http.Request {
	request := oke.buildBaseRequest(method, path)
	request.Body = ioutil.NopCloser(body)
	request.ContentLength = int64(body.Len())

	return request
}

func (oke Oke) sendRequest(method string, path string) ([]byte, error) {
	request := oke.buildBaseRequest(method, path)
	bodyBytes, err := oke.transmitRequest(request)
	return bodyBytes, err
}

func (oke Oke) sendRequestWithPayload(method string, path string, body []byte) ([]byte, error) {
	request := oke.buildRequestWithBody(method, path, bytes.NewBuffer(body))
	bodyBytes, err := oke.transmitRequest(request)
	return bodyBytes, err
}

func (oke Oke) transmitRequest(request *http.Request) ([]byte, error) {
	resp, err := oke.httpDefaultClientDo(request)
	if err != nil {
		return nil, err
	}

	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return bodyBytes, nil
}
