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

package cluster

import (
	"fmt"
	"github.com/kubernetes-incubator/cluster-manager/pkg/common"
	testutils "github.com/kubernetes-incubator/cluster-manager/pkg/common/test"
	opt "github.com/kubernetes-incubator/cluster-manager/pkg/controller/cluster/options"
	"github.com/kubernetes-incubator/cluster-manager/pkg/controller/cluster/provider"
	"github.com/kubernetes-incubator/cluster-manager/pkg/controller/cluster/util"
	navarkospkgcommon "github.com/kubernetes-incubator/navarkos/pkg/common"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/apiserver/pkg/registry/generic/registry"
	restclient "k8s.io/client-go/rest"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/client-go/util/workqueue"
	federationv1beta1 "k8s.io/kubernetes/federation/apis/federation/v1beta1"
	federationclientset "k8s.io/kubernetes/federation/client/clientset_generated/federation_clientset"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/client/clientset_generated/internalclientset"
	"os"
	"reflect"
	"strconv"
	"testing"
	"time"
)

type fakeClusterToolSet struct {
	cluster                   *federationv1beta1.Cluster
	clusterList               *federationv1beta1.ClusterList
	getClusterFailureCount    int
	getClusterFailureMax      int
	updateClusterFailureCount int
	updateClusterFailureMax   int
	updateError               error
	updater                   *clusterUpdater
	executionSequenceList     []string // Use this to list the order of sequence of the kopsToolSet api calls
	sequenceIndex             *int     // This will be used to identify the current index of the executed function in the sequence list. Set this to 0 initially.
	functionWithError         string   // Use this to identify the function that would return an error
	functionWithErrorNumber   int      // Use this to identify the instance # of the function that will return the error if there are multiple of them in the sequence
	functionWithErrorCount    int      // This will be internally used to count the number of times a function is called to be used by functionWithErrorNumber

	t *testing.T
}

var optimisticLockError error = apierrors.NewConflict(schema.GroupResource{}, "Cluster", fmt.Errorf(registry.OptimisticLockErrorMsg))

func resetClusterData(fct *fakeClusterToolSet) {
	fct.cluster.Annotations = make(map[string]string)
	if fct.cluster.Spec.SecretRef != nil {
		fct.cluster.Spec.SecretRef.Name = ""
	}
	if fct.cluster.Spec.ServerAddressByClientCIDRs != nil && len(fct.cluster.Spec.ServerAddressByClientCIDRs) > 0 {
		fct.cluster.Spec.ServerAddressByClientCIDRs[0].ServerAddress = ""
	}
}

func (fct *fakeClusterToolSet) getCluster(clusterName string) (*federationv1beta1.Cluster, error) {
	testutils.CheckExecutionSequence(fct.executionSequenceList, testutils.CallerfunctionName(), fct.sequenceIndex, fct.t)
	if err := testutils.CheckIfFunctionHasError(fct.functionWithError, testutils.CallerfunctionName(), fct.functionWithErrorNumber, &fct.functionWithErrorCount, fct.t); err != nil {
		return nil, err
	}
	var err error
	if fct.getClusterFailureCount < fct.getClusterFailureMax {
		err = errors.Errorf("Get cluster failure count: %d", fct.getClusterFailureCount)
		fct.getClusterFailureCount += 1
		resetClusterData(fct)
	} else {
		err = nil
	}
	return fct.cluster, err
}

func (fct *fakeClusterToolSet) updateCluster(cluster *federationv1beta1.Cluster) (*federationv1beta1.Cluster, error) {
	testutils.CheckExecutionSequence(fct.executionSequenceList, testutils.CallerfunctionName(), fct.sequenceIndex, fct.t)
	if err := testutils.CheckIfFunctionHasError(fct.functionWithError, testutils.CallerfunctionName(), fct.functionWithErrorNumber, &fct.functionWithErrorCount, fct.t); err != nil {
		return nil, err
	}

	var err error
	if fct.updateClusterFailureCount < fct.updateClusterFailureMax {
		if fct.updateError == nil {
			err = errors.Errorf("Update cluster failure count: %d", fct.updateClusterFailureCount)
		} else {
			err = fct.updateError
		}
		fct.updateClusterFailureCount += 1
		resetClusterData(fct)
	} else {
		err = nil
	}
	return fct.cluster, err
}

func (fct *fakeClusterToolSet) listClusters(options metav1.ListOptions) (*federationv1beta1.ClusterList, error) {
	testutils.CheckExecutionSequence(fct.executionSequenceList, testutils.CallerfunctionName(), fct.sequenceIndex, fct.t)
	if err := testutils.CheckIfFunctionHasError(fct.functionWithError, testutils.CallerfunctionName(), fct.functionWithErrorNumber, &fct.functionWithErrorCount, fct.t); err != nil {
		return nil, err
	}
	if fct.clusterList != nil {
		return fct.clusterList, nil
	}
	return nil, nil
}

func (fct *fakeClusterToolSet) watchClusters(options metav1.ListOptions) (watch.Interface, error) {
	testutils.CheckExecutionSequence(fct.executionSequenceList, testutils.CallerfunctionName(), fct.sequenceIndex, fct.t)
	if err := testutils.CheckIfFunctionHasError(fct.functionWithError, testutils.CallerfunctionName(), fct.functionWithErrorNumber, &fct.functionWithErrorCount, fct.t); err != nil {
		return nil, err
	}
	return nil, nil
}

func (fct *fakeClusterToolSet) resetCluster(cluster *federationv1beta1.Cluster) error {
	testutils.CheckExecutionSequence(fct.executionSequenceList, testutils.CallerfunctionName(), fct.sequenceIndex, fct.t)
	if err := testutils.CheckIfFunctionHasError(fct.functionWithError, testutils.CallerfunctionName(), fct.functionWithErrorNumber, &fct.functionWithErrorCount, fct.t); err != nil {
		return err
	}
	return nil
}

func (fct *fakeClusterToolSet) deleteCluster(clusterName string) error {
	testutils.CheckExecutionSequence(fct.executionSequenceList, testutils.CallerfunctionName(), fct.sequenceIndex, fct.t)
	if err := testutils.CheckIfFunctionHasError(fct.functionWithError, testutils.CallerfunctionName(), fct.functionWithErrorNumber, &fct.functionWithErrorCount, fct.t); err != nil {
		return err
	}
	return nil
}

func (fct *fakeClusterToolSet) createCluster(cluster *federationv1beta1.Cluster) (*federationv1beta1.Cluster, error) {
	testutils.CheckExecutionSequence(fct.executionSequenceList, testutils.CallerfunctionName(), fct.sequenceIndex, fct.t)
	if err := testutils.CheckIfFunctionHasError(fct.functionWithError, testutils.CallerfunctionName(), fct.functionWithErrorNumber, &fct.functionWithErrorCount, fct.t); err != nil {
		return nil, err
	}
	return cluster, nil
}

func getTotalExpectedExecutionTime(noOfRetries int, incrementalDuration time.Duration) time.Duration {
	totalExpectedExecution := time.Duration(noOfRetries*(noOfRetries+1)/2) * incrementalDuration
	totalExpectedExecution += (totalExpectedExecution * 50 / 100) // Add buffer of 50% of expected time to ensure execution completeness
	return totalExpectedExecution
}

/*
	Tests following scenarios:
	1. Cluster encounters 3 update failures before it succeeds in the 4th try
	2. Cluster encounters 3 update 'registry.OptimisticLockErrorMsg' failures and will also encounter 3 get failures before update succeeds

	Verifies the following:
	1. Correct sequence call of the clusterToolSet functions
	2. Correct updated cluster when the update operation finally succeeds
*/
func TestClusterUpdateRetrier(t *testing.T) {
	updateNoErrorsOperationSequenceList := []string{
		testutils.GetFunctionName((*fakeClusterToolSet).updateCluster),
	}

	update3XErrorsOperationSequenceList := []string{
		testutils.GetFunctionName((*fakeClusterToolSet).updateCluster),
		testutils.GetFunctionName((*fakeClusterToolSet).getCluster),
		testutils.GetFunctionName((*fakeClusterToolSet).updateCluster),
		testutils.GetFunctionName((*fakeClusterToolSet).getCluster),
		testutils.GetFunctionName((*fakeClusterToolSet).updateCluster),
		testutils.GetFunctionName((*fakeClusterToolSet).getCluster),
		testutils.GetFunctionName((*fakeClusterToolSet).updateCluster),
	}

	update3XErrorsGet3XErrorsOperationSequenceList := []string{
		testutils.GetFunctionName((*fakeClusterToolSet).updateCluster),
		testutils.GetFunctionName((*fakeClusterToolSet).getCluster),
		testutils.GetFunctionName((*fakeClusterToolSet).getCluster),
		testutils.GetFunctionName((*fakeClusterToolSet).getCluster),
		testutils.GetFunctionName((*fakeClusterToolSet).getCluster),
		testutils.GetFunctionName((*fakeClusterToolSet).updateCluster),
		testutils.GetFunctionName((*fakeClusterToolSet).getCluster),
		testutils.GetFunctionName((*fakeClusterToolSet).updateCluster),
		testutils.GetFunctionName((*fakeClusterToolSet).getCluster),
		testutils.GetFunctionName((*fakeClusterToolSet).updateCluster),
	}

	cc := &ClusterController{
		updateRetrier: newClusterUpdateRetrier(),
	}
	cc.startUpdateRetrierWithClusterUpdateHandler()

	tests := []struct {
		injectNoOfClusterUpdateFailures int
		injectNoOfClusterGetFailures    int
		updateError                     error
		executionSequenceList           []string
	}{
		{0, 0, nil, updateNoErrorsOperationSequenceList},
		{3, 0, nil, update3XErrorsOperationSequenceList},
		{3, 3, optimisticLockError, update3XErrorsGet3XErrorsOperationSequenceList},
	}

	for _, testData := range tests {
		testCluster := &federationv1beta1.Cluster{}
		testCluster.Name = "TestCluster"
		testCluster.Annotations = make(map[string]string)
		testCluster.Spec.ServerAddressByClientCIDRs = []federationv1beta1.ServerAddressByClientCIDR{{}}

		updater := newClusterUpdater(cc, testCluster)

		executionIndex := 0
		dummyClusterToolSet := &fakeClusterToolSet{
			cluster: testCluster,

			updateClusterFailureMax: testData.injectNoOfClusterUpdateFailures,
			getClusterFailureMax:    testData.injectNoOfClusterGetFailures,
			updateError:             testData.updateError,
			updater:                 updater,
			executionSequenceList:   testData.executionSequenceList,
			sequenceIndex:           &executionIndex,
			t:                       t,
		}
		cc.clusterToolSet = dummyClusterToolSet
		updater.backoff.incrementalDuration = 100 * time.Millisecond

		updater.setAnnotation(navarkospkgcommon.NavarkosClusterStateKey, navarkospkgcommon.NavarkosClusterStateReady)
		updater.setAnnotation(navarkospkgcommon.NavarkosClusterNodeCountKey, "25")
		updater.setServerAddress(DummyServerAddress)
		updater.setSecretRefName(dummyClusterToolSet.cluster.Name)
		cc.clusterUpdate(updater)

		// Delay to ensure queuing and pushing between channels and invoking handler has completed
		totalExpectedExecutionTime := getTotalExpectedExecutionTime(dummyClusterToolSet.updateClusterFailureMax, updater.backoff.incrementalDuration)
		totalExpectedExecutionTime += getTotalExpectedExecutionTime(dummyClusterToolSet.getClusterFailureMax, updater.backoff.incrementalDuration)
		time.Sleep(totalExpectedExecutionTime)

		if dummyClusterToolSet.updateClusterFailureCount != dummyClusterToolSet.updateClusterFailureMax {
			t.Errorf("Count of update failure is %d, but expected count is %d!", dummyClusterToolSet.updateClusterFailureCount, dummyClusterToolSet.updateClusterFailureMax)
		}
		if !reflect.DeepEqual(testCluster.Annotations, updater.clusterAnnotation.Annotations) {
			t.Errorf("Cluster annotations is %v, but expected value is %v!", testCluster.Annotations, updater.clusterAnnotation.Annotations)
		}
		if updater.secretRefName != nil {
			if testCluster.Spec.SecretRef == nil {
				t.Errorf("Cluster SecretRefName was not updated!")
			} else if testCluster.Spec.SecretRef.Name != *updater.secretRefName {
				t.Errorf("Cluster SecretRefName is '%s', but expected value is '%s'!", testCluster.Spec.SecretRef.Name, *updater.secretRefName)
			}
		}
		if updater.serverAddress != nil {
			if testCluster.Spec.ServerAddressByClientCIDRs[0].ServerAddress != *updater.serverAddress {
				t.Errorf("Cluster ServerAddressByClientCIDRs is '%s', but expected value is '%s'!", testCluster.Spec.ServerAddressByClientCIDRs[0].ServerAddress, *updater.serverAddress)
			}
		}

		testutils.VerifyExecutionSequenceCompleted(testData.executionSequenceList, executionIndex, t)
	}
	cc.updateRetrier.stop()
}

/*
	Tests that all data being queued will be processed accordingly,
	i.e. data will be pushed at the end of the queue and popped from the beginning of the queue.
*/
func TestClusterUpdateRetrierQueueing(t *testing.T) {
	cc := &ClusterController{
		updateRetrier: newClusterUpdateRetrier(),
	}
	var receivedUpdaters []*clusterUpdater
	cc.updateRetrier.startWithHandler(func(item *clusterUpdaterRetryItem) {
		receivedUpdaters = append(receivedUpdaters, item.value.(*clusterUpdater))
	})

	var sendUpdaters []*clusterUpdater
	var totalExpectedExecution time.Duration
	noOfUpdaters := 100
	for i := 0; i < noOfUpdaters; i++ {
		indexStr := strconv.Itoa(i)
		testCluster := &federationv1beta1.Cluster{}
		testCluster.Name = "TestCluster-" + indexStr
		testCluster.Annotations = make(map[string]string)
		testCluster.Spec.ServerAddressByClientCIDRs = []federationv1beta1.ServerAddressByClientCIDR{{}}

		updater := newClusterUpdater(cc, testCluster)
		updater.backoff.incrementalDuration = 2 * time.Millisecond
		totalExpectedExecution += updater.backoff.incrementalDuration

		updater.setAnnotation(navarkospkgcommon.NavarkosClusterStateKey, navarkospkgcommon.NavarkosClusterStateReady)
		updater.setAnnotation(navarkospkgcommon.NavarkosClusterNodeCountKey, indexStr)
		updater.setServerAddress(DummyServerAddress)
		updater.setSecretRefName(testCluster.Name)

		cc.retryClusterUpdate(updater)
		sendUpdaters = append(sendUpdaters, updater)

	}

	// Delay to ensure queuing and pushing between channels and invoking handler has completed
	totalExpectedExecution += time.Second // add 1 second time buffer to ensure completeness of execution
	time.Sleep(totalExpectedExecution)

	if !reflect.DeepEqual(receivedUpdaters, sendUpdaters) {
		t.Errorf("Received Cluster Updaters is %v, but expected value is %v!", receivedUpdaters, sendUpdaters)
	}

	cc.updateRetrier.stop()
}

// Tests that incremental duration is computed correctly across iterations
func TestClusterUpdateRetrierBackoffIncrementalDuration(t *testing.T) {
	cc := &ClusterController{
		updateRetrier: newClusterUpdateRetrier(),
	}

	var startTime time.Time
	noOfRetries := 5
	retryCount := 0
	cc.updateRetrier.startWithHandler(func(item *clusterUpdaterRetryItem) {
		if retryCount < noOfRetries {
			retryCount++
			updater := item.value.(*clusterUpdater)
			duration := time.Now().Sub(startTime)
			if duration < updater.backoff.duration {
				t.Errorf("Invalid execution duration of %d, while the expected duration is %d", duration, updater.backoff.duration)
			}
			startTime = time.Now()
			cc.retryClusterUpdate(updater)
		}
	})

	testCluster := &federationv1beta1.Cluster{}
	testCluster.Name = "TestCluster"

	updater := newClusterUpdater(cc, testCluster)
	updater.backoff.incrementalDuration = 500 * time.Millisecond

	startTime = time.Now()
	expectedExecutionTime := getTotalExpectedExecutionTime(noOfRetries, updater.backoff.incrementalDuration)
	cc.retryClusterUpdate(updater)

	// Delay to ensure queuing and pushing between channels and invoking handler has completed
	time.Sleep(expectedExecutionTime)

	if retryCount != noOfRetries {
		t.Errorf("No of retries is %d but expected value is %d!", retryCount, noOfRetries)
	}

	cc.updateRetrier.stop()
}

/*
	Tests following scenarios:
	1. Updater that will expire in 2 seconds with 1 second incremental duration
	2. Updater that will expire in 10 seconds with 1 second incremental duration

	Verifies the following:
	1. Scenario 1 should fail as the updater will expire and cluster will not be updated
	2. Scenario 2 should succeed and cluster eventually updated
	3. Correct sequence of execution for Scenario 1 & 2
*/
func TestClusterUpdateRetrierExpiryDuration(t *testing.T) {
	ExpiredUpdaterOperationSequenceList := []string{
		testutils.GetFunctionName((*fakeClusterToolSet).updateCluster),
		testutils.GetFunctionName((*fakeClusterToolSet).getCluster),
		testutils.GetFunctionName((*fakeClusterToolSet).updateCluster),
	}

	NormalUpdaterOperationSequenceList := []string{
		testutils.GetFunctionName((*fakeClusterToolSet).updateCluster),
		testutils.GetFunctionName((*fakeClusterToolSet).getCluster),
		testutils.GetFunctionName((*fakeClusterToolSet).updateCluster),
		testutils.GetFunctionName((*fakeClusterToolSet).getCluster),
		testutils.GetFunctionName((*fakeClusterToolSet).updateCluster),
		testutils.GetFunctionName((*fakeClusterToolSet).getCluster),
		testutils.GetFunctionName((*fakeClusterToolSet).updateCluster),
	}

	cc := &ClusterController{
		updateRetrier: newClusterUpdateRetrier(),
	}
	cc.startUpdateRetrierWithClusterUpdateHandler()

	tests := []struct {
		expiryDuration                  time.Duration
		injectNoOfClusterUpdateFailures int
		executionSequenceList           []string
		expectsUpdaterIsExpired         bool
	}{
		{2 * time.Second, 3, ExpiredUpdaterOperationSequenceList, true},
		{10 * time.Second, 3, NormalUpdaterOperationSequenceList, false},
	}

	for _, testData := range tests {
		testCluster := &federationv1beta1.Cluster{}
		testCluster.Name = "TestCluster"
		testCluster.Annotations = make(map[string]string)
		testCluster.Spec.ServerAddressByClientCIDRs = []federationv1beta1.ServerAddressByClientCIDR{{}}

		updater := newClusterUpdater(cc, testCluster)

		executionIndex := 0
		dummyClusterToolSet := &fakeClusterToolSet{
			cluster: testCluster,

			updateClusterFailureMax: testData.injectNoOfClusterUpdateFailures,
			updater:                 updater,
			executionSequenceList:   testData.executionSequenceList,
			sequenceIndex:           &executionIndex,
			t:                       t,
		}
		cc.clusterToolSet = dummyClusterToolSet
		updater.backoff.incrementalDuration = time.Second
		updater.backoff.expiryDuration = testData.expiryDuration

		updater.setAnnotation(navarkospkgcommon.NavarkosClusterStateKey, navarkospkgcommon.NavarkosClusterStateReady)
		cc.clusterUpdate(updater)

		// Delay to ensure queuing and pushing between channels and invoking handler has completed
		// add 2 sec time buffer to ensure completeness of execution
		totalExpectedExecution := time.Duration(dummyClusterToolSet.updateClusterFailureMax*2) * updater.backoff.incrementalDuration
		time.Sleep(totalExpectedExecution + 2*time.Second)

		if testData.expectsUpdaterIsExpired {
			if !updater.backoff.hasExpired() {
				t.Errorf("Updater should have expired but is not!")
			}
			if len(testCluster.Annotations) > 0 {
				t.Errorf("Update cluster should have failed, but annotations was updated with value of %v!", testCluster.Annotations)
			}
		} else {
			if updater.backoff.hasExpired() {
				t.Errorf("Updater has expired but should not have been!")
			}
			if !reflect.DeepEqual(testCluster.Annotations, updater.clusterAnnotation.Annotations) {
				t.Errorf("Cluster annotations is %v, but expected value is %v!", testCluster.Annotations, updater.clusterAnnotation.Annotations)
			}
		}

		testutils.VerifyExecutionSequenceCompleted(testData.executionSequenceList, executionIndex, t)
	}
	cc.updateRetrier.stop()
}

// Test the backoff duration will never exceed max duration.
func TestClusterUpdateRetrierMaxDuration(t *testing.T) {
	cc := &ClusterController{}
	// c.startUpdateRetrierWithClusterUpdateHandler()

	tests := []struct {
		incrementalDuration time.Duration
		maxDuration         time.Duration
	}{
		{5 * time.Minute, 20 * time.Minute},
		{time.Second, 12 * time.Second},
	}

	testCluster := &federationv1beta1.Cluster{}
	for _, testData := range tests {
		updater := newClusterUpdater(cc, testCluster)
		updater.backoff.incrementalDuration = testData.incrementalDuration
		updater.backoff.maxDuration = testData.maxDuration

		var expectedDuration time.Duration
		for i := 0; i < 10; i++ {
			duration := updater.backoff.next()
			expectedDuration += updater.backoff.incrementalDuration
			if duration > updater.backoff.maxDuration {
				t.Errorf("Duration of %d cannot exceed the maxDuration of %d!", duration, updater.backoff.maxDuration)
			} else if duration != expectedDuration {
				if duration != updater.backoff.maxDuration {
					t.Errorf("Duration of %d is not equal to expectedDuration of %d or maxDuration of %d!", duration, expectedDuration, updater.backoff.maxDuration)
				}
			}
		}
	}
}

/*
	When cluster update returns an error with reason metav1.StatusReasonConflict or registry.OptimisticLockErrorMsg,
	it will retry the update 'UpdateRetryCount' no.of times in ClusterUpdater.update() before it hands it off to
	ClusterUpdateRetrier using queued retry operation. This test will verify, if the 'UpdateRetryCount' will first try
	doing the update retry the no. of times it is set, and beyond that, if failure continues, ClusterUpdateRetrier
	will take over the retry operation.
*/
func TestClusterUpdaterUpdateRetry(t *testing.T) {
	cc := &ClusterController{
		updateRetrier: newClusterUpdateRetrier(),
	}

	var updateRetrierHandlerInvoked bool
	cc.updateRetrier.startWithHandler(func(item *clusterUpdaterRetryItem) {
		updateRetrierHandlerInvoked = true
		cc.clusterUpdate(item.value.(*clusterUpdater))
	})
	tests := []struct {
		injectNoOfClusterUpdateFailures int
	}{
		{2}, // 2 update failures and 1 success, will end the retry
		{3}, // 3 update failures will hand it off to ClusterUpdateRetrier for further retries
	}

	for _, testData := range tests {
		testCluster := &federationv1beta1.Cluster{}
		testCluster.Name = "TestCluster"
		testCluster.Annotations = make(map[string]string)

		updater := newClusterUpdater(cc, testCluster)

		dummyClusterToolSet := &fakeClusterToolSet{
			cluster:                 testCluster,
			updateClusterFailureMax: testData.injectNoOfClusterUpdateFailures,
			updateError:             optimisticLockError, // apierrors.NewConflict(schema.GroupResource{}, "Cluster", fmt.Errorf(registry.OptimisticLockErrorMsg)),
			updater:                 updater,
			t:                       t,
		}
		cc.clusterToolSet = dummyClusterToolSet
		updater.backoff.incrementalDuration = 100 * time.Millisecond

		updater.setAnnotation(navarkospkgcommon.NavarkosClusterStateKey, navarkospkgcommon.NavarkosClusterStateReady)
		cc.clusterUpdate(updater)

		// Delay to ensure queuing and pushing between channels and invoking handler has completed
		totalExpectedExecution := getTotalExpectedExecutionTime(dummyClusterToolSet.updateClusterFailureMax, updater.backoff.incrementalDuration)
		time.Sleep(totalExpectedExecution)

		expectsUpdateRetrierHandlerInvoked := testData.injectNoOfClusterUpdateFailures >= MaxRetryCount
		if updateRetrierHandlerInvoked != expectsUpdateRetrierHandlerInvoked {
			t.Errorf("Cluster Update Retrier invoked is %t, but expects %t!", updateRetrierHandlerInvoked, expectsUpdateRetrierHandlerInvoked)
		}
		updateRetrierHandlerInvoked = false
	}

	cc.updateRetrier.stop()
}

type fakeProvider struct {
	kubeConfig              *clientcmdapi.Config
	clusterAnnotation       *util.ClusterAnnotation
	nodeCount               int
	executionSequenceList   []string // Use this to list the order of sequence of the kopsToolSet api calls
	sequenceIndex           *int     // This will be used to identify the current index of the executed function in the sequence list. Set this to 0 initially.
	functionWithError       string   // Use this to identify the function that would return an error
	functionWithErrorNumber int      // Use this to identify the instance # of the function that will return the error if there are multiple of them in the sequence
	functionWithErrorCount  int      // This will be internally used to count the number of times a function is called to be used by functionWithErrorNumber
	t                       *testing.T
}

func (fp *fakeProvider) CreateKubeCluster(newCluster federationv1beta1.Cluster) (*util.ClusterAnnotation, error) {
	testutils.CheckExecutionSequence(fp.executionSequenceList, testutils.CallerfunctionName(), fp.sequenceIndex, fp.t)
	if err := testutils.CheckIfFunctionHasError(fp.functionWithError, testutils.CallerfunctionName(), fp.functionWithErrorNumber, &fp.functionWithErrorCount, fp.t); err != nil {
		return nil, err
	}
	var clusterAnnotation *util.ClusterAnnotation
	if fp.clusterAnnotation != nil {
		clusterAnnotation = fp.clusterAnnotation
	}
	return clusterAnnotation, nil
}

func (fp *fakeProvider) DeleteKubeCluster(existingCluster federationv1beta1.Cluster) error {
	testutils.CheckExecutionSequence(fp.executionSequenceList, testutils.CallerfunctionName(), fp.sequenceIndex, fp.t)
	if err := testutils.CheckIfFunctionHasError(fp.functionWithError, testutils.CallerfunctionName(), fp.functionWithErrorNumber, &fp.functionWithErrorCount, fp.t); err != nil {
		return err
	}
	return nil
}

func (fp *fakeProvider) ValidateKubeCluster(fedCluster federationv1beta1.Cluster) (*util.ClusterAnnotation, error) {
	testutils.CheckExecutionSequence(fp.executionSequenceList, testutils.CallerfunctionName(), fp.sequenceIndex, fp.t)
	var clusterAnnotation *util.ClusterAnnotation
	if fp.clusterAnnotation != nil {
		clusterAnnotation = fp.clusterAnnotation
	}
	if err := testutils.CheckIfFunctionHasError(fp.functionWithError, testutils.CallerfunctionName(), fp.functionWithErrorNumber, &fp.functionWithErrorCount, fp.t); err != nil {
		return clusterAnnotation, err
	}
	return clusterAnnotation, nil
}

func (fp *fakeProvider) ScaleKubeCluster(existingCluster federationv1beta1.Cluster, scaleSize int) (*util.ClusterAnnotation, error) {
	testutils.CheckExecutionSequence(fp.executionSequenceList, testutils.CallerfunctionName(), fp.sequenceIndex, fp.t)
	if err := testutils.CheckIfFunctionHasError(fp.functionWithError, testutils.CallerfunctionName(), fp.functionWithErrorNumber, &fp.functionWithErrorCount, fp.t); err != nil {
		return nil, err
	}
	var clusterAnnotation *util.ClusterAnnotation
	if fp.clusterAnnotation != nil {
		clusterAnnotation = fp.clusterAnnotation
	} else {
		clusterAnnotation = &util.ClusterAnnotation{}
	}
	if fp.nodeCount == 1 {
		clusterAnnotation.SetAnnotation(navarkospkgcommon.NavarkosClusterStateKey, navarkospkgcommon.NavarkosClusterStateReady)
		clusterAnnotation.SetAnnotation(navarkospkgcommon.NavarkosClusterNodeCountKey, strconv.Itoa(fp.nodeCount))
		return clusterAnnotation, nil
	}
	nodeCount := fp.nodeCount + scaleSize
	if nodeCount <= 0 {
		nodeCount = 1
	}

	clusterAnnotation.SetAnnotation(navarkospkgcommon.NavarkosClusterNodeCountKey, strconv.Itoa(nodeCount))
	return clusterAnnotation, nil
}

func (fp *fakeProvider) ExportKubeConfig(fedCluster federationv1beta1.Cluster) (*clientcmdapi.Config, error) {
	testutils.CheckExecutionSequence(fp.executionSequenceList, testutils.CallerfunctionName(), fp.sequenceIndex, fp.t)
	if err := testutils.CheckIfFunctionHasError(fp.functionWithError, testutils.CallerfunctionName(), fp.functionWithErrorNumber, &fp.functionWithErrorCount, fp.t); err != nil {
		return nil, err
	}
	return fp.kubeConfig, nil
}

func (fp *fakeProvider) GetProviderName() string {
	return "TestProvider"
}

type fakeSecretToolSet struct {
	clientset internalclientset.Interface
	t         *testing.T
}

func (fst *fakeSecretToolSet) CreateSecret(namespace string, secret *api.Secret) (*api.Secret, error) {
	return secret, nil
}

func (fst *fakeSecretToolSet) DeleteSecret(namespace, name string) error {
	return nil
}

// No validation here, just testing that execution completes in does not panic
func TestStartClusterController(t *testing.T) {
	stopChan := make(chan struct{})
	restClientCfg := &restclient.Config{}
	options := initClusterControllerOptions()
	StartClusterController(restClientCfg, options, stopChan)
	close(stopChan)
}

func TestNewClusterController(t *testing.T) {
	options := initClusterControllerOptions()
	federationClient := &federationclientset.Clientset{}
	cc := newClusterController(federationClient, options)
	if cc == nil {
		t.Errorf("newClusterController returned nil")
	} else {
		newcc := ClusterController{}
		newcc.getActionableStates()
		if cc.federationClient != federationClient {
			t.Errorf("FederationClient is %v when it is suppose to be %v", cc.federationClient, federationClient)
		}
		if !reflect.DeepEqual(cc.actionableStates, newcc.actionableStates) {
			t.Errorf("Actionable states are %v, but expected value are %v!", cc.actionableStates, newcc.actionableStates)
		}
		if cc.federationName != options.FkubeName {
			t.Errorf("FederationName is %s when it is suppose to be %s", cc.federationName, options.FkubeName)
		}
		if cc.federationNamespace != options.FkubeNamespace {
			t.Errorf("FederationNameSpace is %s when it is suppose to be %s", cc.federationNamespace, options.FkubeNamespace)
		}
		if cc.workQueue == nil {
			t.Errorf("WorkQueue is nil")
		}
		if cc.updateRetrier == nil {
			t.Errorf("UpdateRetrier is nil")
		}
	}
}

func initClusterControllerOptions() *opt.ClusterControllerOptions {
	options := &opt.ClusterControllerOptions{
		FkubeName:      "federation",
		FkubeNamespace: "federation-system",
	}
	os.Setenv("KUBERNETES_SERVICE_HOST", "http://KUBERNETES_SERVICE_HOST")
	os.Setenv("KUBERNETES_SERVICE_PORT", "8001")
	os.Setenv("AWS_ACCESS_KEY_ID", "AWS_ACCESS_KEY_ID")
	os.Setenv("AWS_SECRET_ACCESS_KEY", "AWS_SECRET_ACCESS_KEY")
	os.Setenv("OKE_BEARER_TOKEN", "OKE_BEARER_TOKEN")
	os.Setenv("OKE_AUTH_GROUP", "OKE_AUTH_GROUP")
	return options
}

func TestReconcileWhenReady(t *testing.T) {
	providerName := "TestProvider"

	testCluster1 := &federationv1beta1.Cluster{}
	testCluster1.Name = "TestCluster"
	testCluster1.Spec.ServerAddressByClientCIDRs = []federationv1beta1.ServerAddressByClientCIDR{{}}
	testCluster1.Annotations = make(map[string]string)
	testCluster1.Annotations[common.ClusterManagerClusterClassKey] = providerName
	testCluster1.Annotations[navarkospkgcommon.NavarkosClusterStateKey] = navarkospkgcommon.NavarkosClusterStateJoining

	testCluster2 := &federationv1beta1.Cluster{}
	testCluster2.Name = "TestCluster"
	testCluster2.Spec.ServerAddressByClientCIDRs = []federationv1beta1.ServerAddressByClientCIDR{{}}
	testCluster2.Annotations = make(map[string]string)
	testCluster2.Annotations[common.ClusterManagerClusterClassKey] = providerName
	testCluster2.Annotations[navarkospkgcommon.NavarkosClusterStateKey] = navarkospkgcommon.NavarkosClusterStateScalingUp

	clusterList := &federationv1beta1.ClusterList{}
	clusterList.Items = []federationv1beta1.Cluster{*testCluster1, *testCluster2}

	dummyClusterToolSet1 := &fakeClusterToolSet{
		clusterList: clusterList,
		t:           t,
	}

	cc := &ClusterController{
		clusterProviders:    make(map[string]provider.Provider),
		secretToolSet:       &fakeSecretToolSet{clientset: nil, t: t},
		federationNamespace: "federation-system",
		federationName:      "federation",
	}
	cc.clusterProviders[providerName] = &fakeProvider{
		kubeConfig: getKubeConfig(testCluster1),
	}

	dummyClusterToolSet2 := &fakeClusterToolSet{
		clusterList:             clusterList,
		t:                       t,
		functionWithError:       testutils.GetFunctionName((*fakeClusterToolSet).listClusters),
		functionWithErrorNumber: 1,
	}
	tests := []struct {
		dummyClusterToolSet *fakeClusterToolSet
		expectsSuccess      bool
	}{
		{dummyClusterToolSet1, true},
		{dummyClusterToolSet2, false},
	}

	for _, testData := range tests {
		cc.clusterToolSet = testData.dummyClusterToolSet
		err := cc.reconcileWhenReady()
		if testData.expectsSuccess && err != nil {
			t.Errorf("reconcileWhenReady encountered an error: %v", err)
		} else if !testData.expectsSuccess && err == nil {
			t.Errorf("reconcileWhenReady expects an error, but did not get one")
		}
	}
}

func TestWorkerQueue(t *testing.T) {
	providerName := "TestProvider"

	testCluster := &federationv1beta1.Cluster{}
	testCluster.Name = "TestCluster"
	testCluster.Spec.ServerAddressByClientCIDRs = []federationv1beta1.ServerAddressByClientCIDR{{}}
	testCluster.Annotations = make(map[string]string)
	testCluster.Annotations[common.ClusterManagerClusterClassKey] = providerName

	dummyClusterToolSet := &fakeClusterToolSet{
		cluster: testCluster,
		t:       t,
	}

	cc := &ClusterController{
		clusterProviders:    make(map[string]provider.Provider),
		clusterToolSet:      dummyClusterToolSet,
		workQueue:           workqueue.New(),
		secretToolSet:       &fakeSecretToolSet{clientset: nil, t: t},
		federationNamespace: "federation-system",
		federationName:      "federation",
	}
	cc.clusterProviders[providerName] = &fakeProvider{}

	tests := []struct {
		onClusterAdd              bool
		clusterStatus             string
		expectedNextClusterStatus string
	}{
		{true, navarkospkgcommon.NavarkosClusterStatePending, navarkospkgcommon.NavarkosClusterStateProvisioning},
		{false, navarkospkgcommon.NavarkosClusterStatePendingShutdown, navarkospkgcommon.NavarkosClusterStateOffline},
		{false, navarkospkgcommon.NavarkosClusterStatePendingScaleUp, navarkospkgcommon.NavarkosClusterStateScalingUp},
		{false, navarkospkgcommon.NavarkosClusterStatePendingScaleDown, navarkospkgcommon.NavarkosClusterStateScalingDown},
	}

	cc.actionableStates = make(sets.String)
	cc.getActionableStates()
	for _, testData := range tests {
		testCluster.Annotations[navarkospkgcommon.NavarkosClusterStateKey] = testData.clusterStatus

		if testData.onClusterAdd {
			cc.onClusterAdd(testCluster)
		} else {
			cc.onClusterUpdate(nil, testCluster)
		}
		go func() {
			timeout := time.After(2 * time.Second)
			tick := time.Tick(500 * time.Millisecond)
		loop:
			for {
				select {
				case <-timeout:
					t.Errorf("TestWorkerQueue has timed out on cluster status %s", testData.clusterStatus)
					cc.workQueue.ShutDown()
					break loop
				case <-tick:
					if clusterStatus, ok := testCluster.Annotations[navarkospkgcommon.NavarkosClusterStateKey]; ok {
						switch clusterStatus {
						case testData.clusterStatus:
							// just continue
						case testData.expectedNextClusterStatus:
							cc.workQueue.ShutDown()
							break loop
						default:
							t.Errorf("TestWorkerQueue encountered an invalid cluster status of %s when it is suppose to be either %s or %s",
								clusterStatus, testData.clusterStatus, testData.expectedNextClusterStatus)
						}

					}
				}
			}
		}()
		cc.worker()
		cc.workQueue = workqueue.New()
	}
}

func TestClusterJoin(t *testing.T) {
	providerName := "TestProvider"

	testCluster := &federationv1beta1.Cluster{}
	testCluster.Name = "TestCluster"
	testCluster.Annotations = make(map[string]string)
	testCluster.Annotations[common.ClusterManagerClusterClassKey] = providerName
	testCluster.Spec.ServerAddressByClientCIDRs = []federationv1beta1.ServerAddressByClientCIDR{{}}

	dummyClusterToolSet := &fakeClusterToolSet{
		cluster: testCluster,
		t:       t,
	}

	cc := &ClusterController{
		clusterProviders: make(map[string]provider.Provider),
		clusterToolSet:   dummyClusterToolSet,
		secretToolSet: &fakeSecretToolSet{
			clientset: nil,
			t:         t},
		federationNamespace: "federation-system",
		federationName:      "federation",
	}

	kubeConfig := getKubeConfig(testCluster)
	cc.clusterProviders[providerName] = &fakeProvider{
		kubeConfig: kubeConfig,
	}
	cc.clusterJoin(testCluster)
	if cluster, ok := kubeConfig.Clusters[testCluster.Name]; ok {
		if testCluster.Spec.ServerAddressByClientCIDRs[0].ServerAddress != cluster.Server {
			t.Errorf("Cluster %s has Server API Address of %s when it is suppose to be %s",
				testCluster.Name, testCluster.Spec.ServerAddressByClientCIDRs[0].ServerAddress, cluster.Server)
		}
		if testCluster.Spec.SecretRef.Name != testCluster.Name {
			t.Errorf("Cluster %s has Secret name of %s when it is suppose to be %s",
				testCluster.Name, testCluster.Spec.SecretRef.Name, testCluster.Name)
		}
	} else {
		t.Errorf("Cluster entry for %s does not exist in KubeConfig", testCluster.Name)
	}
}

func getKubeConfig(testCluster *federationv1beta1.Cluster) *clientcmdapi.Config {
	kubeConfig := clientcmdapi.NewConfig()

	// `{"preferences":{},"clusters":{"TestCluster":{"LocationOfOrigin":"","server":"https://api.TestCluster","certificate-authority-data":"MockedCertificateData"}},"users":{"TestCluster":{"LocationOfOrigin":"","client-certificate-data":"MockedCertificateData","client-key-data":"MockerClientKeyData","username":"admin","password":"Password"},"TestCluster-basic-auth":{"LocationOfOrigin":"","username":"admin","password":"Password"}},"contexts":{"TestCluster":{"LocationOfOrigin":"","cluster":"TestCluster","user":"TestCluster"}},"current-context":"TestCluster"}`
	// "clusters":{"TestCluster":{"LocationOfOrigin":"","server":"https://api.TestCluster","certificate-authority-data":"MockedCertificateData"}}
	cluster := clientcmdapi.NewCluster()
	cluster.Server = "https://api." + testCluster.Name
	cluster.CertificateAuthorityData = []byte("MockedCerticicateData")
	user := clientcmdapi.NewAuthInfo()

	// "users":{"TestCluster":{"LocationOfOrigin":"","client-certificate-data":"MockedCertificateData","client-key-data":"MockerClientKeyData","username":"admin","password":"Password"}
	user.ClientCertificateData = []byte("MockedCertificateData")
	user.ClientKeyData = []byte("MockedClientKeyData")
	user.Username = "admin"
	user.Password = "password"

	// "%s-basic-auth":{"LocationOfOrigin":"","username":"admin","password":"Password"}
	userBasicAuth := clientcmdapi.NewAuthInfo()
	userBasicAuth.Username = user.Username
	userBasicAuth.Password = user.Password

	// "contexts":{"%s":{"LocationOfOrigin":"","cluster":"%s","user":"%s"}},"current-context":"%s"
	context := clientcmdapi.NewContext()
	context.Cluster = testCluster.Name
	context.Cluster = testCluster.Name

	kubeConfig.Clusters[testCluster.Name] = cluster
	kubeConfig.AuthInfos[testCluster.Name] = user
	kubeConfig.AuthInfos[testCluster.Name+"-basic-auth"] = userBasicAuth
	kubeConfig.Contexts[testCluster.Name] = context
	kubeConfig.CurrentContext = testCluster.Name
	return kubeConfig
}

func TestValidateCluster(t *testing.T) {
	providerName := "TestProvider"

	testCluster := &federationv1beta1.Cluster{}
	testCluster.Name = "TestCluster"
	testCluster.Spec.ServerAddressByClientCIDRs = []federationv1beta1.ServerAddressByClientCIDR{{}}

	dummyClusterToolSet := &fakeClusterToolSet{
		cluster: testCluster,
		t:       t,
	}

	cc := &ClusterController{
		clusterProviders:    make(map[string]provider.Provider),
		clusterToolSet:      dummyClusterToolSet,
		federationNamespace: "federation-system",
		federationName:      "federation",
	}

	workItemAnnotation := make(map[string]string)
	workItemAnnotation[common.ClusterManagerClusterOKEWorkItemKey] = "TestWorkItemID"

	deleteWorkItemClusterAnnotation := &util.ClusterAnnotation{}
	deleteWorkItemClusterAnnotation.DeleteAnnotation(common.ClusterManagerClusterOKEWorkItemKey)

	addClusterAnnotation := &util.ClusterAnnotation{}
	addClusterAnnotation.SetAnnotation("TestID", "TestData")

	multipleAnnotation := make(map[string]string)
	multipleAnnotation[common.ClusterManagerClusterOKEWorkItemKey] = "TestWorkItemID"
	multipleAnnotation["TestID"] = "TestData"

	multipleDataClusterAnnotation := &util.ClusterAnnotation{}
	multipleDataClusterAnnotation.DeleteAnnotation(common.ClusterManagerClusterOKEWorkItemKey)
	multipleDataClusterAnnotation.SetAnnotation("ValidationData", "Processing")

	tests := []struct {
		clusterStatus              string
		originalClusterAnnotations map[string]string
		returnedClusterAnnotations *util.ClusterAnnotation
		expectedNextClusterStatus  string
		functionWithError          string
		functionWithErrorNumber    int
	}{
		{navarkospkgcommon.NavarkosClusterStateProvisioning, nil, nil, navarkospkgcommon.NavarkosClusterStateJoining, "", 0},
		{navarkospkgcommon.NavarkosClusterStateScalingUp, nil, nil, navarkospkgcommon.NavarkosClusterStateReady, "", 0},
		{navarkospkgcommon.NavarkosClusterStateScalingDown, nil, nil, navarkospkgcommon.NavarkosClusterStateReady, "", 0},
		{navarkospkgcommon.NavarkosClusterStateProvisioning, nil, nil, "", testutils.GetFunctionName((*fakeProvider).ValidateKubeCluster), 1},
		{navarkospkgcommon.NavarkosClusterStateProvisioning, workItemAnnotation, deleteWorkItemClusterAnnotation, "", testutils.GetFunctionName((*fakeProvider).ValidateKubeCluster), 1},
		{navarkospkgcommon.NavarkosClusterStateProvisioning, multipleAnnotation, multipleDataClusterAnnotation, "", testutils.GetFunctionName((*fakeProvider).ValidateKubeCluster), 1},
	}

	for _, testData := range tests {
		cc.clusterProviders[providerName] = &fakeProvider{
			clusterAnnotation:       testData.returnedClusterAnnotations,
			functionWithError:       testData.functionWithError,
			functionWithErrorNumber: testData.functionWithErrorNumber,
		}
		testCluster.Annotations = make(map[string]string)
		testCluster.Annotations[common.ClusterManagerClusterClassKey] = providerName
		if testData.originalClusterAnnotations != nil {
			for key, value := range testData.originalClusterAnnotations {
				testCluster.Annotations[key] = value
			}
		}

		expectedAnnotations := make(map[string]string)
		for key, value := range testCluster.Annotations {
			expectedAnnotations[key] = value
		}
		if testData.expectedNextClusterStatus != "" {
			expectedAnnotations[navarkospkgcommon.NavarkosClusterStateKey] = testData.expectedNextClusterStatus
		}
		if testData.returnedClusterAnnotations != nil {
			testData.returnedClusterAnnotations.ApplyAnnotation(expectedAnnotations)
		}

		cc.validateCluster(testCluster, testData.clusterStatus)
		if !reflect.DeepEqual(testCluster.Annotations, expectedAnnotations) {
			t.Errorf("Cluster annotations is %v, but expected value is %v!", testCluster.Annotations, expectedAnnotations)
		}
	}
}

func TestShutdownCluster(t *testing.T) {
	providerName := "TestProvider"

	testCluster := &federationv1beta1.Cluster{}
	testCluster.Name = "TestCluster"
	testCluster.Annotations = make(map[string]string)
	testCluster.Annotations[common.ClusterManagerClusterClassKey] = providerName
	testCluster.Spec.ServerAddressByClientCIDRs = []federationv1beta1.ServerAddressByClientCIDR{{}}

	// Fill up the annotations with data that will be deleted once shutdown is called
	for i, key := range getClusterAnnotationsToDelete() {
		testCluster.Annotations[key] = "TestData-" + strconv.Itoa(i)
	}

	dummyClusterToolSet := &fakeClusterToolSet{
		cluster: testCluster,
		t:       t,
	}

	cc := &ClusterController{
		clusterProviders: make(map[string]provider.Provider),
		clusterToolSet:   dummyClusterToolSet,
		secretToolSet: &fakeSecretToolSet{
			clientset: nil,
			t:         t},
		federationNamespace: "federation-system",
		//federationName:      "federation",
	}

	cc.clusterProviders[providerName] = &fakeProvider{}
	cc.shutdownCluster(testCluster)
	// Check that all annotations that needs to be deleted are gone
	for _, key := range getClusterAnnotationsToDelete() {
		if _, ok := testCluster.Annotations[key]; ok {
			t.Errorf("Cluster annotation '%s' still exist when it should have been deleted", key)
		}
	}
	if testCluster.Spec.ServerAddressByClientCIDRs[0].ServerAddress != DummyServerAddress {
		t.Errorf("Cluster's ServerAddress is '%s' when it should have been reset to '%s'", testCluster.Spec.ServerAddressByClientCIDRs[0].ServerAddress, DummyServerAddress)
	}

}

func TestCreateCluster(t *testing.T) {
	providerName := "TestProvider"

	testCluster := &federationv1beta1.Cluster{}
	testCluster.Name = "TestCluster"
	testCluster.Spec.ServerAddressByClientCIDRs = []federationv1beta1.ServerAddressByClientCIDR{{}}

	okeAnnotation := &util.ClusterAnnotation{}
	okeAnnotation.SetAnnotation(common.ClusterManagerClusterOKEIdKey, "TestClusterID")
	okeAnnotation.SetAnnotation(common.ClusterManagerClusterOKEWorkItemKey, "TestWorkItemID")
	okeAnnotation.SetAnnotation(navarkospkgcommon.NavarkosClusterNodeCountKey, "3")

	dummyClusterToolSet := &fakeClusterToolSet{
		cluster: testCluster,
		t:       t,
	}

	cc := &ClusterController{
		clusterProviders: make(map[string]provider.Provider),
		clusterToolSet:   dummyClusterToolSet,
	}

	tests := []struct {
		expectedNextClusterStatus  string
		returnedClusterAnnotations *util.ClusterAnnotation
		functionWithError          string
		functionWithErrorNumber    int
	}{
		{navarkospkgcommon.NavarkosClusterStateProvisioning, okeAnnotation, "", 0},
		{navarkospkgcommon.NavarkosClusterStateFailedProvision, nil, testutils.GetFunctionName((*fakeProvider).CreateKubeCluster), 1},
	}

	for i, testData := range tests {
		cc.clusterProviders[providerName] = &fakeProvider{
			clusterAnnotation:       testData.returnedClusterAnnotations,
			functionWithError:       testData.functionWithError,
			functionWithErrorNumber: testData.functionWithErrorNumber,
		}
		testCluster.Annotations = make(map[string]string)
		testCluster.Annotations[common.ClusterManagerClusterClassKey] = providerName

		expectedAnnotations := make(map[string]string)
		for key, value := range testCluster.Annotations {
			expectedAnnotations[key] = value
		}
		if testData.expectedNextClusterStatus != "" {
			expectedAnnotations[navarkospkgcommon.NavarkosClusterStateKey] = testData.expectedNextClusterStatus
		}
		if testData.returnedClusterAnnotations != nil {
			testData.returnedClusterAnnotations.ApplyAnnotation(expectedAnnotations)
		}

		cc.createCluster(testCluster)
		if !reflect.DeepEqual(testCluster.Annotations, expectedAnnotations) {
			t.Errorf("Cluster annotations is %v, but expected value is %v failing at test #%d", testCluster.Annotations, expectedAnnotations, i)
		}
	}
}

func TestScaleCluster(t *testing.T) {
	providerName := "TestProvider"

	testCluster := &federationv1beta1.Cluster{}
	testCluster.Name = "TestCluster"
	testCluster.Spec.ServerAddressByClientCIDRs = []federationv1beta1.ServerAddressByClientCIDR{{}}

	okeAnnotation := &util.ClusterAnnotation{}
	okeAnnotation.SetAnnotation(common.ClusterManagerClusterOKEWorkItemKey, "TestWorkItemID")

	dummyClusterToolSet := &fakeClusterToolSet{
		cluster: testCluster,
		t:       t,
	}

	cc := &ClusterController{
		clusterProviders: make(map[string]provider.Provider),
		clusterToolSet:   dummyClusterToolSet,
	}

	tests := []struct {
		up                         bool
		nodeCount                  int
		scaleSize                  int
		expectedNextClusterStatus  string
		returnedClusterAnnotations *util.ClusterAnnotation
		functionWithError          string
		functionWithErrorNumber    int
	}{
		{true, 5, 3, navarkospkgcommon.NavarkosClusterStateScalingUp, nil, "", 0},
		{false, 5, 3, navarkospkgcommon.NavarkosClusterStateScalingDown, nil, "", 0},
		{true, 5, 0, navarkospkgcommon.NavarkosClusterStateScalingUp, nil, "", 0},
		{false, 5, 0, navarkospkgcommon.NavarkosClusterStateScalingDown, nil, "", 0},
		{false, 1, 1, navarkospkgcommon.NavarkosClusterStateReady, nil, "", 0},
		{true, 5, 3, navarkospkgcommon.NavarkosClusterStateScalingUp, okeAnnotation, "", 0},
		{true, 5, 3, navarkospkgcommon.NavarkosClusterStateFailedScaleUp, nil, testutils.GetFunctionName((*fakeProvider).ScaleKubeCluster), 1},
		{false, 5, 3, navarkospkgcommon.NavarkosClusterStateFailedScaleDown, nil, testutils.GetFunctionName((*fakeProvider).ScaleKubeCluster), 1},
	}

	for i, testData := range tests {
		cc.clusterProviders[providerName] = &fakeProvider{
			nodeCount:               testData.nodeCount,
			clusterAnnotation:       testData.returnedClusterAnnotations,
			functionWithError:       testData.functionWithError,
			functionWithErrorNumber: testData.functionWithErrorNumber,
		}
		testCluster.Annotations = make(map[string]string)
		testCluster.Annotations[common.ClusterManagerClusterClassKey] = providerName

		var scaleSize int
		if testData.scaleSize > 0 {
			if testData.up {
				testCluster.Annotations[common.ClusterManagerClusterScaleUpSizeKey] = strconv.Itoa(testData.scaleSize)
				scaleSize = testData.scaleSize
			} else {
				testCluster.Annotations[common.ClusterManagerClusterScaleDownSizeKey] = strconv.Itoa(testData.scaleSize)
				scaleSize = 0 - testData.scaleSize
			}
		} else {
			if testData.up {
				scaleSize = DefaultClusterScaleUpSize
			} else {
				scaleSize = 0 - DefaultClusterScaleDownSize
			}

		}
		nodeCount := testData.nodeCount + scaleSize
		if nodeCount <= 0 {
			nodeCount = 1
		}

		expectedAnnotations := make(map[string]string)
		for key, value := range testCluster.Annotations {
			expectedAnnotations[key] = value
		}
		if testData.expectedNextClusterStatus != "" {
			expectedAnnotations[navarkospkgcommon.NavarkosClusterStateKey] = testData.expectedNextClusterStatus
			if testData.expectedNextClusterStatus != navarkospkgcommon.NavarkosClusterStateFailedScaleUp &&
				testData.expectedNextClusterStatus != navarkospkgcommon.NavarkosClusterStateFailedScaleDown {
				expectedAnnotations[navarkospkgcommon.NavarkosClusterNodeCountKey] = strconv.Itoa(nodeCount)
			}

		}
		if testData.returnedClusterAnnotations != nil {
			testData.returnedClusterAnnotations.ApplyAnnotation(expectedAnnotations)
		}

		cc.scaleCluster(testCluster, testData.up)
		if !reflect.DeepEqual(testCluster.Annotations, expectedAnnotations) {
			t.Errorf("Cluster annotations is %v, but expected value is %v failing at test #%d", testCluster.Annotations, expectedAnnotations, i)
		}
	}
}
