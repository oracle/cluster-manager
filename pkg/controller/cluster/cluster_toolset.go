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

package cluster

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	federationv1beta1 "k8s.io/kubernetes/federation/apis/federation/v1beta1"
	federationclientset "k8s.io/kubernetes/federation/client/clientset_generated/federation_clientset"
)

var _ clusterToolSetInterface = &clusterToolSet{}

type clusterToolSet struct {
	federationClient federationclientset.Interface
}

func newClusterToolSet(federationClient federationclientset.Interface) clusterToolSetInterface {
	return &clusterToolSet{federationClient: federationClient}
}

func (ct *clusterToolSet) getCluster(clusterName string) (*federationv1beta1.Cluster, error) {
	return ct.federationClient.FederationV1beta1().Clusters().Get(clusterName, metav1.GetOptions{})
}

func (ct *clusterToolSet) updateCluster(cluster *federationv1beta1.Cluster) (*federationv1beta1.Cluster, error) {
	return ct.federationClient.FederationV1beta1().Clusters().Update(cluster)
}

func (ct *clusterToolSet) listClusters(options metav1.ListOptions) (*federationv1beta1.ClusterList, error) {
	return ct.federationClient.FederationV1beta1().Clusters().List(options)
}

func (ct *clusterToolSet) watchClusters(options metav1.ListOptions) (watch.Interface, error) {
	return ct.federationClient.FederationV1beta1().Clusters().Watch(options)
}

func (ct *clusterToolSet) deleteCluster(clusterName string) error {
	return ct.federationClient.FederationV1beta1().Clusters().Delete(clusterName, &metav1.DeleteOptions{})
}

func (ct *clusterToolSet) createCluster(cluster *federationv1beta1.Cluster) (*federationv1beta1.Cluster, error) {
	return ct.federationClient.FederationV1beta1().Clusters().Create(cluster)
}
