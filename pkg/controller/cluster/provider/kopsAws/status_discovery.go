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
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elb"
	"k8s.io/kops/pkg/apis/kops"
	"k8s.io/kops/upup/pkg/fi"
	"k8s.io/kops/upup/pkg/fi/cloudup/awsup"
	"k8s.io/kops/upup/pkg/fi/cloudup/gce"
)

// This will wrap low level blocking API calls so they can easily be faked/mocked in a unit test
type statusDiscoveryToolSetInterface interface {
	cloudupBuildCloud(cluster *kops.Cluster) (fi.Cloud, error)
	gceCloudGetApiIngressStatus(cluster *kops.Cluster, gceCloud *gce.GCECloud) ([]kops.ApiIngressStatus, error)
	awstasksFindLoadBalancerByNameTag(awsCloud awsup.AWSCloud, name string) (*elb.LoadBalancerDescription, error)
}

// cloudDiscoveryStatusStore implements status.Store by inspecting cloud objects.
// Likely temporary until we validate our status usage
type cloudDiscoveryStatusStore struct {
	toolSet statusDiscoveryToolSetInterface
}

var _ kops.StatusStore = &cloudDiscoveryStatusStore{}

func (s *cloudDiscoveryStatusStore) GetApiIngressStatus(cluster *kops.Cluster) ([]kops.ApiIngressStatus, error) {
	cloud, err := s.toolSet.cloudupBuildCloud(cluster)
	if err != nil {
		return nil, err
	}

	if gceCloud, ok := cloud.(*gce.GCECloud); ok {
		return s.toolSet.gceCloudGetApiIngressStatus(cluster, gceCloud)
	}

	if awsCloud, ok := cloud.(awsup.AWSCloud); ok {
		name := "api." + cluster.Name
		lb, err := s.toolSet.awstasksFindLoadBalancerByNameTag(awsCloud, name)
		if err != nil {
			return nil, fmt.Errorf("error looking for AWS ELB: %v", err)
		}
		if lb == nil {
			return nil, nil
		}
		var ingresses []kops.ApiIngressStatus

		if lb != nil {
			lbDnsName := aws.StringValue(lb.DNSName)
			if lbDnsName == "" {
				return nil, fmt.Errorf("Found ELB %q, but it did not have a DNSName", name)
			}

			ingresses = append(ingresses, kops.ApiIngressStatus{Hostname: lbDnsName})
		}

		return ingresses, nil
	}

	return nil, fmt.Errorf("API Ingress Status not implemented for %T", cloud)
}
