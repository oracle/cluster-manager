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

// This is the production implementation of all toolsets required by status_discovery.go. This code block will by
// default be a part of the build as long as "-tags test" is not added in go test/build. Otherwise if "-tags test" is
// added, this code will be excluded, so it will also be excluded in the code coverage. This implementation is a way to
// separate the production code which calls 3rd party APIs, from unit test implementation which will mock the behavior
// of those 3rd party API calls. Because they are 3rd party APIs, they don't need to be part of the code coverage.

package kopsAws

import (
	"github.com/aws/aws-sdk-go/service/elb"
	apiskops "k8s.io/kops/pkg/apis/kops"
	"k8s.io/kops/upup/pkg/fi"
	"k8s.io/kops/upup/pkg/fi/cloudup"
	"k8s.io/kops/upup/pkg/fi/cloudup/awstasks"
	"k8s.io/kops/upup/pkg/fi/cloudup/awsup"
	"k8s.io/kops/upup/pkg/fi/cloudup/gce"
)

var _ statusDiscoveryToolSetInterface = &statusDiscoveryToolSet{}

type statusDiscoveryToolSet struct{}

func (sdts *statusDiscoveryToolSet) cloudupBuildCloud(cluster *apiskops.Cluster) (fi.Cloud, error) {
	return cloudup.BuildCloud(cluster)
}

func (sdts *statusDiscoveryToolSet) gceCloudGetApiIngressStatus(cluster *apiskops.Cluster, gceCloud *gce.GCECloud) ([]apiskops.ApiIngressStatus, error) {
	return gceCloud.GetApiIngressStatus(cluster)
}

func (sdts *statusDiscoveryToolSet) awstasksFindLoadBalancerByNameTag(awsCloud awsup.AWSCloud, name string) (*elb.LoadBalancerDescription, error) {
	return awstasks.FindLoadBalancerByNameTag(awsCloud, name)
}
