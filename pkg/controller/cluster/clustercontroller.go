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
	"github.com/golang/glog"
	"github.com/kubernetes-incubator/cluster-manager/pkg/common"
	opt "github.com/kubernetes-incubator/cluster-manager/pkg/controller/cluster/options"
	"github.com/kubernetes-incubator/cluster-manager/pkg/controller/cluster/provider"
	providerkops "github.com/kubernetes-incubator/cluster-manager/pkg/controller/cluster/provider/kopsAws"
	provideroke "github.com/kubernetes-incubator/cluster-manager/pkg/controller/cluster/provider/oke"
	"github.com/kubernetes-incubator/cluster-manager/pkg/controller/cluster/util"
	navarkospkgcommon "github.com/kubernetes-incubator/navarkos/pkg/common"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/client-go/util/workqueue"
	federationv1beta1 "k8s.io/kubernetes/federation/apis/federation/v1beta1"
	clustercache "k8s.io/kubernetes/federation/client/cache"
	federationclientset "k8s.io/kubernetes/federation/client/clientset_generated/federation_clientset"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/v1"
	"k8s.io/kubernetes/pkg/controller"
	"strconv"
	"time"
)

const (
	ClusterManagerName          = "cluster-manager"
	KubeConfReadRetryDelay      = 15 * time.Second
	DefaultClusterScaleUpSize   = 1
	DefaultClusterScaleDownSize = 1
	DummyServerAddress          = "https://213.0.0.1"
	MaxRetryCount               = 3
	BackoffIncrementalDuration  = 5 * time.Second
	BackoffMaxDuration          = 30 * time.Minute
	BackoffExpiryDuration       = 2 * time.Hour
)

type clusterUpdaterRetryItem struct {
	// value of the item.
	value interface{}
	// When the item should be delivered.
	deliveryTime time.Time
}

// A structure that pushes the items to the target channel at a given time.
type clusterUpdateRetrier struct {
	// Channel to deliver the data when their time comes.
	targetChannel chan *clusterUpdaterRetryItem
	// Store for data
	data []clusterUpdaterRetryItem
	// Channel to feed the main goroutine with updates.
	updateChannel chan *clusterUpdaterRetryItem
	// To stop the main goroutine.
	stopChannel chan struct{}
}

func newClusterUpdateRetrier() *clusterUpdateRetrier {
	return &clusterUpdateRetrier{
		targetChannel: make(chan *clusterUpdaterRetryItem, 100),
		updateChannel: make(chan *clusterUpdaterRetryItem, 1000),
		stopChannel:   make(chan struct{}),
	}
}

// push the item at the end of the queue
func (r *clusterUpdateRetrier) push(item clusterUpdaterRetryItem) {
	r.data = append(r.data, item)
}

// pop the item from the beginning of the queue
func (r *clusterUpdateRetrier) pop() (item clusterUpdaterRetryItem) {
	item = r.data[0]
	r.data = append(r.data[:0], r.data[1:]...)
	return
}

// deliver all items due before or equal to timestamp.
func (r *clusterUpdateRetrier) deliver(timestamp time.Time) {
	for len(r.data) > 0 {
		if timestamp.Before(r.data[0].deliveryTime) {
			return
		}
		item := r.pop()
		r.targetChannel <- &item
	}
}

func (r *clusterUpdateRetrier) run() {
	for {
		now := time.Now()
		r.deliver(now)

		nextWakeUp := now.Add(time.Hour)
		if len(r.data) > 0 {
			nextWakeUp = r.data[0].deliveryTime
		}
		sleepTime := nextWakeUp.Sub(now)

		select {
		case <-time.After(sleepTime):
			break // just wake up and process the data
		case item := <-r.updateChannel:
			r.push(*item)
		case <-r.stopChannel:
			return
		}
	}
}

// starts the clusterUpdateRetrier.
func (r *clusterUpdateRetrier) start() {
	go r.run()
}

// stops the clusterUpdateRetrier. Undelivered items are discarded.
func (r *clusterUpdateRetrier) stop() {
	close(r.stopChannel)
}

// delivers the value after the given delay.
func (r *clusterUpdateRetrier) deliverAfter(value interface{}, delay time.Duration) {
	r.updateChannel <- &clusterUpdaterRetryItem{
		value:        value,
		deliveryTime: time.Now().Add(delay),
	}
}

// starts Cluster Update Retrier with a handler listening on the target channel.
func (r *clusterUpdateRetrier) startWithHandler(handler func(*clusterUpdaterRetryItem)) {
	go func() {
		for {
			select {
			case item := <-r.targetChannel:
				handler(item)
			case <-r.stopChannel:
				return
			}
		}
	}()
	r.start()
}

type backoffItem struct {
	duration            time.Duration
	startTime           time.Time
	incrementalDuration time.Duration
	maxDuration         time.Duration
	expiryDuration      time.Duration
}

func (b *backoffItem) next() time.Duration {
	newDuration := b.duration + b.incrementalDuration
	if newDuration <= b.maxDuration {
		b.duration = newDuration
	} else {
		b.duration = b.maxDuration
	}
	return b.duration
}

func (b *backoffItem) hasExpired() bool {
	// backoff will not expire if backoffExpiryDuration is <= 0
	if b.expiryDuration > 0 && time.Now().Sub(b.startTime) >= b.expiryDuration {
		return true
	}
	return false
}

type clusterUpdater struct {
	// Make sure to set this to ClusterController when this is instantiated or update will fail
	cc *ClusterController
	// cluster object to update
	cluster     *federationv1beta1.Cluster
	needsResync bool
	isDirty     bool // Make sure to set this to true when fields are updated
	backoff     *backoffItem
	// Objects that will be updated in the Cluster
	clusterAnnotation *util.ClusterAnnotation
	serverAddress     *string
	secretRefName     *string
}

func newClusterUpdater(cc *ClusterController, cluster *federationv1beta1.Cluster) *clusterUpdater {
	return &clusterUpdater{
		cc:                cc,
		cluster:           cluster,
		clusterAnnotation: &util.ClusterAnnotation{},
		backoff: &backoffItem{
			incrementalDuration: BackoffIncrementalDuration,
			maxDuration:         BackoffMaxDuration,
			expiryDuration:      BackoffExpiryDuration,
		},
	}
}

func (cu *clusterUpdater) reset() {
	cu.needsResync = false
	cu.isDirty = false
	cu.clusterAnnotation.Annotations = nil
	cu.clusterAnnotation.DeleteAnnotations = nil
	cu.serverAddress = nil
	cu.secretRefName = nil
	cu.backoff.duration = 0
	cu.backoff.startTime = time.Time{}
}

func (cu *clusterUpdater) setAnnotation(key string, value string) {
	cu.clusterAnnotation.SetAnnotation(key, value)
	cu.isDirty = true
}

func (cu *clusterUpdater) deleteAnnotation(key string) {
	cu.clusterAnnotation.DeleteAnnotation(key)
	cu.isDirty = true
}

func (cu *clusterUpdater) copyAnnotation(newClusterAnnotation *util.ClusterAnnotation) {
	if newClusterAnnotation.Annotations != nil && len(newClusterAnnotation.Annotations) > 0 {
		// Add all Annotations
		for key, value := range newClusterAnnotation.Annotations {
			cu.clusterAnnotation.SetAnnotation(key, value)
		}
		cu.isDirty = true
	}
	if newClusterAnnotation.DeleteAnnotations != nil && len(newClusterAnnotation.DeleteAnnotations) > 0 {
		// Add all DeleteAnnotations
		cu.clusterAnnotation.DeleteAnnotations = append(cu.clusterAnnotation.DeleteAnnotations, newClusterAnnotation.DeleteAnnotations...)
		cu.isDirty = true
	}
}

func (cu *clusterUpdater) setSecretRefName(secretRefName string) {
	cu.secretRefName = &secretRefName
	cu.isDirty = true
}

func (cu *clusterUpdater) setServerAddress(serverAddress string) {
	cu.serverAddress = &serverAddress
	cu.isDirty = true
}

func (cu *clusterUpdater) resync(clusterName string) error {
	cluster, err := cu.cc.clusterToolSet.getCluster(clusterName)
	if err == nil {
		cu.cluster = cluster
		cu.updateData()
		cu.needsResync = false
	} else {
		glog.Errorf("Error refreshing cluster : %v, error is : %v", clusterName, err)
		cu.needsResync = true
	}
	return err
}

// retry if update fails with registry.OptimisticLockErrorMsg
func (cu *clusterUpdater) update() error {
	if cu.cc != nil {
		var err error
		if cu.isDirty {
			if !cu.needsResync {
				cu.updateData()
			}
			clusterName := cu.cluster.Name
			for retryCount := 0; retryCount < MaxRetryCount; retryCount++ {
				if cu.needsResync {
					err = cu.resync(clusterName)
					if err != nil {
						break
					}
				}
				_, err = cu.cc.clusterToolSet.updateCluster(cu.cluster)
				if err != nil {
					cu.needsResync = true
					// apierrors.IsConflict() checks for error that has registry.OptimisticLockErrorMsg,
					// i.e. "the object has been modified; please apply your changes to the latest version and try again"
					// If this error is encountered, a resync and update of the cluster will be retried 'UpdateRetryCount' times
					if apierrors.IsConflict(err) {
						glog.Errorf("Error updating cluster '%s' containing '%v' with error of '%v'", clusterName, *cu.cluster, err)
					} else {
						break
					}
				} else {
					// Success
					cu.needsResync = false
					break
				}
			}
		}
		return err
	} else {
		return errors.Errorf("Update of Cluster will not work. ClusterController needs to be set in ClusterUpdater object!")
	}
}

func (cu *clusterUpdater) updateData() {
	cu.clusterAnnotation.ApplyAnnotation(cu.cluster.Annotations)
	if cu.serverAddress != nil {
		cu.cluster.Spec.ServerAddressByClientCIDRs[0].ServerAddress = *cu.serverAddress
	}
	if cu.secretRefName != nil {
		if cu.cluster.Spec.SecretRef == nil {
			cu.cluster.Spec.SecretRef = &v1.LocalObjectReference{
				Name: *cu.secretRefName,
			}
		} else {
			cu.cluster.Spec.SecretRef.Name = *cu.secretRefName
		}
	}
}

type clusterToolSetInterface interface {
	getCluster(clusterName string) (*federationv1beta1.Cluster, error)
	updateCluster(cluster *federationv1beta1.Cluster) (*federationv1beta1.Cluster, error)
	listClusters(options metav1.ListOptions) (*federationv1beta1.ClusterList, error)
	watchClusters(options metav1.ListOptions) (watch.Interface, error)
	deleteCluster(clusterName string) error
	createCluster(cluster *federationv1beta1.Cluster) (*federationv1beta1.Cluster, error)
}

type ClusterController struct {
	actionableStates sets.String
	// federationClient used to operate cluster
	federationClient federationclientset.Interface
	// cluster framework and store
	clusterController   cache.Controller
	clusterStore        clustercache.StoreToClusterLister
	workQueue           workqueue.Interface
	clusterProviders    map[string]provider.Provider
	federationName      string
	federationNamespace string
	updateRetrier       *clusterUpdateRetrier
	clusterToolSet      clusterToolSetInterface
	secretToolSet       util.SecretToolSetInterface
}

// StartClusterController starts a new cluster controller
func StartClusterController(config *restclient.Config, options *opt.ClusterControllerOptions, stopChan <-chan struct{}) {
	restclient.AddUserAgent(config, "cluster-controller")
	client := federationclientset.NewForConfigOrDie(config)
	controller := newClusterController(client, options)
	controller.Run(stopChan)
}

// newClusterController returns a new cluster controller
func newClusterController(federationClient federationclientset.Interface, options *opt.ClusterControllerOptions) *ClusterController {
	cc := &ClusterController{
		actionableStates:    make(sets.String),
		federationClient:    federationClient,
		workQueue:           workqueue.New(),
		clusterProviders:    make(map[string]provider.Provider),
		federationName:      options.FkubeName,
		federationNamespace: options.FkubeNamespace,
		updateRetrier:       newClusterUpdateRetrier(),
		clusterToolSet:      newClusterToolSet(federationClient),
		secretToolSet:       util.NewSecretToolSet(),
	}

	cc.getActionableStates()

	var err error
	cc.clusterProviders["kops"], err = providerkops.NewAwsKops(options)

	if err != nil {
		glog.Errorf("error initializing kops provider %v", err)
	}

	cc.clusterProviders["oke"], err = provideroke.NewOke(options)

	if err != nil {
		glog.Errorf("error initializing OKE provider %v", err)
	}

	cc.clusterStore.Store, cc.clusterController = cache.NewInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return cc.clusterToolSet.listClusters(options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return cc.clusterToolSet.watchClusters(options)
			},
		},
		&federationv1beta1.Cluster{},
		controller.NoResyncPeriodFunc(),
		cache.ResourceEventHandlerFuncs{
			AddFunc:    cc.onClusterAdd,
			UpdateFunc: cc.onClusterUpdate,
		},
	)
	return cc
}

func (cc *ClusterController) getActionableStates() {
	if cc.actionableStates == nil {
		cc.actionableStates = make(sets.String)
	}
	cc.actionableStates.Insert(
		navarkospkgcommon.NavarkosClusterStatePending,
		navarkospkgcommon.NavarkosClusterStatePendingShutdown,
		navarkospkgcommon.NavarkosClusterStatePendingScaleUp,
		navarkospkgcommon.NavarkosClusterStatePendingScaleDown,
	)
}

func (cc *ClusterController) onClusterUpdate(oldObj, newObj interface{}) {
	cluster := newObj.(*federationv1beta1.Cluster)
	glog.V(1).Infof("%s observed an updated cluster: %v", ClusterManagerName, cluster.Name)
	cc.SubmitClusterForProvisioning(cluster)
}

func (cc *ClusterController) onClusterAdd(obj interface{}) {
	cluster := obj.(*federationv1beta1.Cluster)
	glog.V(1).Infof("%s observed a new cluster: %v", ClusterManagerName, cluster.Name)
	cc.SubmitClusterForProvisioning(cluster)
}

//all checks are done on cluster submission
//at this point we assume the aanotation is set and provider is supported
func (cc *ClusterController) getProvider(cluster *federationv1beta1.Cluster) provider.Provider {
	clusterClass := cluster.Annotations[common.ClusterManagerClusterClassKey]
	return cc.clusterProviders[clusterClass]
}

func (cc *ClusterController) SubmitClusterForProvisioning(cluster *federationv1beta1.Cluster) {
	if clusterClass, ok := cluster.Annotations[common.ClusterManagerClusterClassKey]; ok {

		if _, ok := cc.clusterProviders[clusterClass]; ok {
			if clusterStatus, ok := cluster.Annotations[navarkospkgcommon.NavarkosClusterStateKey]; ok && cc.actionableStates.Has(clusterStatus) {

				glog.V(1).Infof("Sending request to cluster provider for cluster %v with state %v", cluster.Name, clusterStatus)
				newCluster, err := api.Scheme.DeepCopy(cluster)
				if err != nil {
					glog.Errorf("Error deep copy %v", err)
				}
				cc.workQueue.Add(newCluster)
			} else {
				glog.Infof("Cluster %v is not in pending state ", cluster.Name)
			}
		} else {
			glog.Errorf("Cluster %v type %v is not supported!", cluster.Name, clusterClass)
		}
	} else {
		glog.V(4).Infof("Cluster %v has no provider class defined...", cluster.Name)
	}
}

// Run begins watching and syncing.
func (cc *ClusterController) Run(stopChan <-chan struct{}) {

	glog.Infof("Starting cluster controller watch")

	defer utilruntime.HandleCrash()
	go cc.clusterController.Run(stopChan)
	cc.startUpdateRetrierWithClusterUpdateHandler()

	go func() {
		<-stopChan
		cc.workQueue.ShutDown()
		cc.updateRetrier.stop()
	}()

	go wait.Until(cc.worker, time.Second, stopChan)

	go wait.Until(func() {
		if err := cc.reconcileWhenReady(); err != nil {
			glog.Errorf("Error updating clusters configs: %v", err)
		}
	}, KubeConfReadRetryDelay, stopChan)

}

func (cc *ClusterController) reconcileWhenReady() error {

	glog.V(1).Infof("Validating clusters with pending status")

	clusters, err := cc.clusterToolSet.listClusters(metav1.ListOptions{})
	if err != nil {
		return err
	}
	for _, cluster := range clusters.Items {
		if clusterStatus, ok := cluster.Annotations[navarkospkgcommon.NavarkosClusterStateKey]; ok {

			if clusterStatus == navarkospkgcommon.NavarkosClusterStateProvisioning ||
				clusterStatus == navarkospkgcommon.NavarkosClusterStateScalingUp ||
				clusterStatus == navarkospkgcommon.NavarkosClusterStateScalingDown {

				cc.validateCluster(&cluster, clusterStatus)

			} else if clusterStatus == navarkospkgcommon.NavarkosClusterStateJoining {

				cc.clusterJoin(&cluster)

			}
		}
	}

	return nil
}

func (cc *ClusterController) validateCluster(cluster *federationv1beta1.Cluster, clusterStatus string) {
	clusterAnnotation, validationError := cc.getProvider(cluster).ValidateKubeCluster(*cluster)
	var updater *clusterUpdater
	if clusterAnnotation != nil {
		updater = newClusterUpdater(cc, cluster)
		updater.copyAnnotation(clusterAnnotation)
	}
	if validationError == nil {
		if updater == nil {
			updater = newClusterUpdater(cc, cluster)
		}
		nextClusterStatus := navarkospkgcommon.NavarkosClusterStateReady
		if clusterStatus == navarkospkgcommon.NavarkosClusterStateProvisioning {
			nextClusterStatus = navarkospkgcommon.NavarkosClusterStateJoining
		}
		updater.setAnnotation(navarkospkgcommon.NavarkosClusterStateKey, nextClusterStatus)
	}
	if updater != nil {
		cc.clusterUpdate(updater)
	}
}

func (cc *ClusterController) clusterJoin(cluster *federationv1beta1.Cluster) {
	glog.V(1).Infof("Cluster %s is joining federation", cluster.Name)
	kubeConf, err := cc.getProvider(cluster).ExportKubeConfig(*cluster)
	if err == nil && kubeConf != nil && kubeConf.Clusters != nil {
		clusterName := kubeConf.Contexts[kubeConf.CurrentContext].Cluster
		if clusterConf, ok := kubeConf.Clusters[clusterName]; ok {

			err := cc.createClusterSecrets(cluster, kubeConf)
			if err == nil {

				updater := newClusterUpdater(cc, cluster)
				updater.setAnnotation(navarkospkgcommon.NavarkosClusterStateKey, navarkospkgcommon.NavarkosClusterStateReady)
				updater.setServerAddress(clusterConf.Server)
				updater.setSecretRefName(cluster.Name)
				updater.updateData()
				if err = cc.resetCluster(cluster); err != nil {
					glog.Errorf("Unable to reset cluster '%s' with content of '%v' and error of '%v'", cluster.Name, cluster, err)
				}
			}
		} else {
			glog.Errorf("Error reading kube endpoint with context - %q, will retry in 15 seconds", kubeConf.CurrentContext)
		}
	} else {
		glog.Errorf("Error reading kube config from the store - %q, will retry in 15 seconds", err)
	}
}

// Delete and recreate Cluster. This is used during cluster provisioning to make hyperkube clustercontroller to
// retrigger healthstate monitoring
func (cc *ClusterController) resetCluster(cluster *federationv1beta1.Cluster) error {
	// Do multiple attempts for delete and create to allow a higher chance of success
	var err error
	for retry := 0; retry < MaxRetryCount; retry++ {
		if err = cc.clusterToolSet.deleteCluster(cluster.Name); err == nil {
			break
		}
		cluster, _ = cc.clusterToolSet.getCluster(cluster.Name)
	}
	if err != nil {
		return err
	}
	// Remove resource version before creating
	cluster.ResourceVersion = ""
	for retry := 0; retry < MaxRetryCount; retry++ {
		if _, err = cc.clusterToolSet.createCluster(cluster); err == nil {
			break
		}
	}
	return err
}

func (cc *ClusterController) createClusterSecrets(cluster *federationv1beta1.Cluster, kubeStructConfig *clientcmdapi.Config) error {

	glog.V(1).Infof("Generating kubeconfig file")
	_, err := cc.createSecret(kubeStructConfig, cc.federationNamespace, cc.federationName, cluster.Name, kubeStructConfig.CurrentContext, cluster.Name, false)

	if err != nil {
		glog.Errorf("Error creating secret: %s", err)
		return err
	}
	return nil
}

func (cc *ClusterController) createSecret(clientConfig *clientcmdapi.Config, namespace, federationName, joiningClusterName, contextName, secretName string, dryRun bool) (runtime.Object, error) {
	if cc.secretToolSet == nil {
		return nil, errors.Errorf("Secret Toolset was not successfully created for this clustercontroller")
	}

	// contents are inlined.
	err := clientcmdapi.FlattenConfig(clientConfig)
	if err != nil {
		glog.V(2).Infof("Failed to flatten the kubeconfig for the given context %q: %v", contextName, err)
		return nil, err
	}

	glog.V(1).Infof("Creating secret")

	return util.CreateKubeconfigSecret(cc.secretToolSet, clientConfig, namespace, secretName, federationName, joiningClusterName, dryRun)
}

func (cc *ClusterController) deleteSecret(secretName string) error {
	if cc.secretToolSet == nil {
		return errors.Errorf("Secret Toolset was not successfully created for this clustercontroller")
	}

	glog.V(1).Infof("Deleting secret %v", secretName)

	return util.DeleteKubeconfigSecret(cc.secretToolSet, cc.federationNamespace, secretName)
}

func (cc *ClusterController) workerFunction() bool {

	item, quit := cc.workQueue.Get()

	if quit {
		glog.V(1).Info("Got quit from the working queue")
		return true
	}
	defer cc.workQueue.Done(item)

	cluster := item.(*federationv1beta1.Cluster)

	// get the latest cluster status
	clusterName := cluster.Name
	cluster, err := cc.clusterToolSet.getCluster(clusterName)
	if err != nil {
		glog.Errorf("Unable to resync cluster %v, error is : %v", clusterName, err)
		return false
	}
	if clusterStatus, ok := cluster.Annotations[navarkospkgcommon.NavarkosClusterStateKey]; ok {

		var err error
		switch clusterStatus {
		case navarkospkgcommon.NavarkosClusterStatePending:
			err = cc.createCluster(cluster)
		case navarkospkgcommon.NavarkosClusterStatePendingShutdown:
			err = cc.shutdownCluster(cluster)
		case navarkospkgcommon.NavarkosClusterStatePendingScaleUp:
			err = cc.scaleCluster(cluster, true)
		case navarkospkgcommon.NavarkosClusterStatePendingScaleDown:
			err = cc.scaleCluster(cluster, false)
		default:
			glog.Errorf("State: %v on cluster %v is not yet supported", clusterStatus, cluster.Name)
			return false
		}

		if err != nil {
			glog.Errorf("Error updating cluster status: %v - %v , error is : %v", cluster.Name, cluster.Annotations[navarkospkgcommon.NavarkosClusterStateKey], err)
			return false
		}

	} else {
		glog.Errorf("Can not get state for cluster %v", cluster.Name)
	}
	return false

}

func (cc *ClusterController) shutdownCluster(cluster *federationv1beta1.Cluster) error {
	updater := newClusterUpdater(cc, cluster)
	updater.setAnnotation(navarkospkgcommon.NavarkosClusterStateKey, navarkospkgcommon.NavarkosClusterStateShuttingDown)
	err := updater.update()
	if err != nil {
		return err
	}

	err = cc.getProvider(cluster).DeleteKubeCluster(*cluster)
	if err != nil {
		glog.Errorf("Error shutdown cluster: %v ,error is : %v", cluster.Name, err)
		return err
	}

	//delete kubeconfig secret
	err = cc.deleteSecret(cluster.Name)
	if err != nil {
		glog.Errorf("Error deleting kubeconfig secret for cluster: %v ,error is : %v", cluster.Name, err)
		// return err
	}

	glog.Infof("Cluster: %v moving to offline state", cluster.Name)

	updater.reset()
	updater.setAnnotation(navarkospkgcommon.NavarkosClusterStateKey, navarkospkgcommon.NavarkosClusterStateOffline)
	updater.setServerAddress(DummyServerAddress)
	for _, clusterAnnotation := range getClusterAnnotationsToDelete() {
		updater.deleteAnnotation(clusterAnnotation)
	}
	cc.clusterUpdate(updater)

	return err
}

func getClusterAnnotationsToDelete() []string {
	clusterAnnotationsToDelete := []string{
		navarkospkgcommon.NavarkosClusterCapacityAllocatablePodsKey,
		navarkospkgcommon.NavarkosClusterCapacityPodsKey,
		navarkospkgcommon.NavarkosClusterCapacityUsedPodsKey,
		navarkospkgcommon.NavarkosClusterCapacityUsedPodsKey,
		navarkospkgcommon.NavarkosClusterCapacitySystemPodsKey,
		navarkospkgcommon.NavarkosClusterNodeCountKey,
		common.ClusterManagerClusterOKEIdKey,
		common.ClusterManagerClusterOKEWorkItemKey,
	}
	return clusterAnnotationsToDelete
}

func (cc *ClusterController) createCluster(cluster *federationv1beta1.Cluster) error {

	clusterAnnotation, err := cc.getProvider(cluster).CreateKubeCluster(*cluster)

	updater := newClusterUpdater(cc, cluster)
	if err != nil {
		glog.Errorf("Error provisioning cluster: '%v', error is '%v'", cluster.Name, err)
		updater.setAnnotation(navarkospkgcommon.NavarkosClusterStateKey, navarkospkgcommon.NavarkosClusterStateFailedProvision)
	} else {
		glog.Infof("Submitted cluster: %v for provisioning", cluster.Name)
		if clusterAnnotation != nil {
			updater.copyAnnotation(clusterAnnotation)
		}
		updater.setAnnotation(navarkospkgcommon.NavarkosClusterStateKey, navarkospkgcommon.NavarkosClusterStateProvisioning)
	}

	cc.clusterUpdate(updater)

	return err
}

func (cc *ClusterController) scaleCluster(cluster *federationv1beta1.Cluster, up bool) error {
	updater := newClusterUpdater(cc, cluster)
	if up {
		updater.setAnnotation(navarkospkgcommon.NavarkosClusterStateKey, navarkospkgcommon.NavarkosClusterStateScalingUp)
	} else {
		updater.setAnnotation(navarkospkgcommon.NavarkosClusterStateKey, navarkospkgcommon.NavarkosClusterStateScalingDown)
	}
	err := updater.update()
	if err != nil {
		return err
	}

	var scaleSize int
	if up {
		scaleSize = getAnnotationIntegerValue(cluster, common.ClusterManagerClusterScaleUpSizeKey, DefaultClusterScaleUpSize)
	} else {
		// if scale down, make scaleSize a negative value
		scaleSize = 0 - getAnnotationIntegerValue(cluster, common.ClusterManagerClusterScaleDownSizeKey, DefaultClusterScaleDownSize)
	}

	clusterAnnotation, err := cc.getProvider(cluster).ScaleKubeCluster(*cluster, scaleSize)

	updater.reset()
	if clusterAnnotation != nil {
		updater.copyAnnotation(clusterAnnotation)
	}
	if err != nil {
		glog.Errorf("Error scaling cluster '%v' to size of %d with error: %v", cluster.Name, scaleSize, err)
		if up {
			updater.setAnnotation(navarkospkgcommon.NavarkosClusterStateKey, navarkospkgcommon.NavarkosClusterStateFailedScaleUp)
		} else {
			updater.setAnnotation(navarkospkgcommon.NavarkosClusterStateKey, navarkospkgcommon.NavarkosClusterStateFailedScaleDown)
		}
	} else {
		glog.V(4).Infof("Successfully scaled cluster '%v' to size of %d", cluster.Name, scaleSize)
	}
	cc.clusterUpdate(updater)

	return err
}

func (cc *ClusterController) worker() {
	for {
		if quit := cc.workerFunction(); quit {
			glog.Infof("%s worker queue shutting down", ClusterManagerName)
			return
		}
	}
}

func (cc *ClusterController) startUpdateRetrierWithClusterUpdateHandler() {
	// Handler will pass unexpired updater to clusterUpdate() to begin the update retry
	cc.updateRetrier.startWithHandler(func(item *clusterUpdaterRetryItem) {
		updater := item.value.(*clusterUpdater)
		if !updater.backoff.hasExpired() {
			cc.clusterUpdate(updater)
		}
	})
}

func (cc *ClusterController) retryClusterUpdate(updater *clusterUpdater) {
	if updater.backoff.startTime.IsZero() {
		// Put a retry start time stamp at the start of retry operation
		// This will be used  to check if the updater has expired
		updater.backoff.startTime = time.Now()
	}
	if !updater.backoff.hasExpired() {
		delay := updater.backoff.next()
		cc.updateRetrier.deliverAfter(updater, delay)
	}
}

func (cc *ClusterController) clusterUpdate(updater *clusterUpdater) {
	err := updater.update()
	if err != nil {
		glog.V(3).Infof("Retrying updater for cluster %s with annotation %v", updater.cluster.Name, updater.cluster.Annotations)
		cc.retryClusterUpdate(updater)
	}
}

func getAnnotationIntegerValue(cluster *federationv1beta1.Cluster, annotationName string, defaultValue int) int {
	annotationOriginalValue, ok := cluster.Annotations[annotationName]
	if ok && annotationOriginalValue != "" {
		annotationConvertedValue, err := strconv.Atoi(annotationOriginalValue)
		if err == nil {
			return annotationConvertedValue
		}
	}
	return defaultValue
}
