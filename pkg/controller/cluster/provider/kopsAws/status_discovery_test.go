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
	"github.com/aws/aws-sdk-go/service/elb"
	testutils "github.com/kubernetes-incubator/cluster-manager/pkg/common/test"
	"k8s.io/kops/pkg/apis/kops"
	"k8s.io/kops/upup/pkg/fi"
	"k8s.io/kops/upup/pkg/fi/cloudup/awsup"
	"k8s.io/kops/upup/pkg/fi/cloudup/gce"
	"k8s.io/kops/upup/pkg/fi/cloudup/vsphere"
	"strconv"
	"testing"
)

type fakeStatusDiscoveryToolSet struct {
	cloudVendor             fi.Cloud
	externalLoadBalancer    *elb.LoadBalancerDescription
	executionSequenceList   []string
	sequenceIndex           *int
	functionWithError       string
	functionWithErrorNumber int
	functionWithErrorCount  int
	t                       *testing.T
}

func (fsdts *fakeStatusDiscoveryToolSet) cloudupBuildCloud(cluster *kops.Cluster) (fi.Cloud, error) {
	testutils.CheckExecutionSequence(fsdts.executionSequenceList, testutils.CallerfunctionName(), fsdts.sequenceIndex, fsdts.t)
	if err := testutils.CheckIfFunctionHasError(fsdts.functionWithError, testutils.CallerfunctionName(), fsdts.functionWithErrorNumber, &fsdts.functionWithErrorCount, fsdts.t); err != nil {
		return nil, err
	}
	return fsdts.cloudVendor, nil
}

func (fsdts *fakeStatusDiscoveryToolSet) gceCloudGetApiIngressStatus(cluster *kops.Cluster, gceCloud *gce.GCECloud) ([]kops.ApiIngressStatus, error) {
	testutils.CheckExecutionSequence(fsdts.executionSequenceList, testutils.CallerfunctionName(), fsdts.sequenceIndex, fsdts.t)
	if err := testutils.CheckIfFunctionHasError(fsdts.functionWithError, testutils.CallerfunctionName(), fsdts.functionWithErrorNumber, &fsdts.functionWithErrorCount, fsdts.t); err != nil {
		return nil, err
	}
	return []kops.ApiIngressStatus{}, nil
}

func (fsdts *fakeStatusDiscoveryToolSet) awstasksFindLoadBalancerByNameTag(awsCloud awsup.AWSCloud, name string) (*elb.LoadBalancerDescription, error) {
	testutils.CheckExecutionSequence(fsdts.executionSequenceList, testutils.CallerfunctionName(), fsdts.sequenceIndex, fsdts.t)
	if err := testutils.CheckIfFunctionHasError(fsdts.functionWithError, testutils.CallerfunctionName(), fsdts.functionWithErrorNumber, &fsdts.functionWithErrorCount, fsdts.t); err != nil {
		return nil, err
	}
	return fsdts.externalLoadBalancer, nil
}

/*
	Tests following scenarios:
	1. Vendor is GCE
    2. Vendor is AWS
	3. LoadBalancer returned has name or does not have name
	4. LoadBalancer returned is nil

	Checks the following:
	1. Correct execution sequence of statusDiscoveryToolSet functions
	2. Checks the validity of the returned values
*/
func TestGetApiIngressStatus(t *testing.T) {
	awsOperationSequenceList := []string{
		testutils.GetFunctionName((*fakeStatusDiscoveryToolSet).cloudupBuildCloud),
		testutils.GetFunctionName((*fakeStatusDiscoveryToolSet).awstasksFindLoadBalancerByNameTag),
	}

	gceOperationSequenceList := []string{
		testutils.GetFunctionName((*fakeStatusDiscoveryToolSet).cloudupBuildCloud),
		testutils.GetFunctionName((*fakeStatusDiscoveryToolSet).gceCloudGetApiIngressStatus),
	}

	const (
		LoadBalancerNil         = "LoadBalancerNil"
		LoadBalancerWithName    = "LoadBalancerWithName"
		LoadBalancerWithoutName = "LoadBalancerWithoutName"
	)

	tests := []struct {
		cloudVendor              fi.Cloud
		externalLoadBalancerType string
		executionSequenceList    []string
		expectsSuccess           bool
	}{
		{&gce.GCECloud{}, LoadBalancerNil, gceOperationSequenceList, true},
		{&awsup.MockAWSCloud{}, LoadBalancerWithName, awsOperationSequenceList, true},
		{&awsup.MockAWSCloud{}, LoadBalancerWithoutName, awsOperationSequenceList, false},
		{&awsup.MockAWSCloud{}, LoadBalancerNil, awsOperationSequenceList, true},
	}

	kopsCluster := &kops.Cluster{}
	for i, test := range tests {
		var externalLoadBalancer *elb.LoadBalancerDescription
		if test.externalLoadBalancerType == LoadBalancerWithName || test.externalLoadBalancerType == LoadBalancerWithoutName {
			externalLoadBalancer = &elb.LoadBalancerDescription{}
			if test.externalLoadBalancerType == LoadBalancerWithName {
				dnsName := "testdns" + strconv.Itoa(i)
				externalLoadBalancer.DNSName = &dnsName
			}
		}
		executionIndex := 0
		statusDiscovery := &cloudDiscoveryStatusStore{
			&fakeStatusDiscoveryToolSet{
				cloudVendor:           test.cloudVendor,
				externalLoadBalancer:  externalLoadBalancer,
				executionSequenceList: test.executionSequenceList,
				sequenceIndex:         &executionIndex,
				t:                     t,
			},
		}
		ingressStatus, err := statusDiscovery.GetApiIngressStatus(kopsCluster)
		if test.expectsSuccess {
			if err != nil {
				t.Errorf("GetApiIngressStatus has error %v for ingress %v!", err, ingressStatus)
			}
			if externalLoadBalancer != nil {
				if ingressStatus == nil {
					t.Errorf("GetApiIngressStatus is not able to get result")
				} else if _, ok := test.cloudVendor.(awsup.AWSCloud); ok {
					if ingressStatus[0].Hostname != *externalLoadBalancer.DNSName {
						t.Errorf("GetApiIngressStatus Hostname is %s, but should be %s!", ingressStatus[0].Hostname, *externalLoadBalancer.DNSName)
					}
				}
			} else {
				if _, ok := test.cloudVendor.(awsup.AWSCloud); ok {
					if ingressStatus != nil {
						t.Errorf("GetApiIngressStatus (on AWS cloud) return value should be nil because the externalLoadBalancer is nil!")
					}
				} else if ingressStatus == nil {
					t.Errorf("GetApiIngressStatus (on GCE cloud) return value should not be nil!")
				}
			}
		} else {
			if err == nil {
				t.Errorf("GetApiIngressStatus expects an error but did not get one!")
			}
		}
		testutils.VerifyExecutionSequenceCompleted(test.executionSequenceList, executionIndex, t)
	}
}

// Simulate statusDeliveryToolSet functions returning errors
func TestGetApiIngressStatusErrors(t *testing.T) {
	dnsName := "testdns"
	externalLoadBalancerWithName := &elb.LoadBalancerDescription{DNSName: &dnsName}

	tests := []struct {
		cloudVendor             fi.Cloud
		externalLoadBalancer    *elb.LoadBalancerDescription
		functionWithError       string
		functionWithErrorNumber int
	}{
		{&gce.GCECloud{}, nil, testutils.GetFunctionName((*fakeStatusDiscoveryToolSet).gceCloudGetApiIngressStatus), 1},
		{&awsup.MockAWSCloud{}, externalLoadBalancerWithName, testutils.GetFunctionName((*fakeStatusDiscoveryToolSet).cloudupBuildCloud), 1},
		{&awsup.MockAWSCloud{}, externalLoadBalancerWithName, testutils.GetFunctionName((*fakeStatusDiscoveryToolSet).awstasksFindLoadBalancerByNameTag), 1},
		{&vsphere.VSphereCloud{}, nil, "", 0}, // This vendor is not supported so this will error out.
	}

	kopsCluster := &kops.Cluster{}
	for _, test := range tests {
		statusDiscovery := &cloudDiscoveryStatusStore{
			&fakeStatusDiscoveryToolSet{
				t:                       t,
				cloudVendor:             test.cloudVendor,
				externalLoadBalancer:    test.externalLoadBalancer,
				functionWithError:       test.functionWithError,
				functionWithErrorNumber: test.functionWithErrorNumber,
			},
		}
		_, err := statusDiscovery.GetApiIngressStatus(kopsCluster)
		if err == nil {
			t.Errorf("GetApiIngressStatus is expected to have error in %s!", test.functionWithError)
		}
	}
}
