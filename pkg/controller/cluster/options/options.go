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

package options

import "github.com/spf13/pflag"

type ClusterControllerOptions struct {
	Domain             string
	DefaultMasterZones string
	DefaultNodeZones   string
	KopsStateStore     string
	DefaultOkeAds      string
	OkeApiHost         string
	FkubeApiServer     string
	FkubeName          string
	FkubeNamespace     string
}

func NewCCO() *ClusterControllerOptions {
	cco := ClusterControllerOptions{
		Domain:             "",
		DefaultMasterZones: "us-east-1a",
		DefaultNodeZones:   "us-east-1a",
		KopsStateStore:     "",
		DefaultOkeAds:      "AD-1,AD-2,AD-3",
		OkeApiHost:         "api.cluster.oracledx.com",
		FkubeApiServer:     "",
		FkubeName:          "federation",
		FkubeNamespace:     "federation-system",
	}
	return &cco
}

func (cco *ClusterControllerOptions) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&cco.Domain, "domain", cco.Domain, "Domain to be used in aws")
	fs.StringVar(&cco.DefaultMasterZones, "masterZones", cco.DefaultMasterZones, "Default master zones")
	fs.StringVar(&cco.DefaultNodeZones, "nodeZones", cco.DefaultNodeZones, "Default node zones")
	fs.StringVar(&cco.KopsStateStore, "statestore", cco.KopsStateStore, "Kops State Store (like s3 path)")
	fs.StringVar(&cco.DefaultOkeAds, "okeAds", cco.DefaultOkeAds, "Default OKE Availability Domains")
	fs.StringVar(&cco.OkeApiHost, "okeApiHost", cco.OkeApiHost, "OKE API host")
	fs.StringVar(&cco.FkubeApiServer, "fkubeApiServer", cco.FkubeApiServer, "Federation API host")
	fs.StringVar(&cco.FkubeName, "fkubeName", cco.FkubeName, "Federation Name")
	fs.StringVar(&cco.FkubeNamespace, "fkubeNamespace", cco.FkubeNamespace, "Federation Namespace")
}
