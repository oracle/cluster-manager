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

package util

import (
	"github.com/golang/glog"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	restclient "k8s.io/client-go/rest"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/client/clientset_generated/internalclientset"
)

var _ SecretToolSetInterface = &SecretToolSet{}

type SecretToolSet struct {
	clientset internalclientset.Interface
}

func NewSecretToolSet() SecretToolSetInterface {
	incc, err := restclient.InClusterConfig()

	if err != nil {
		glog.Errorf("Error in creating in-cluster config: %s", err)
		return nil
	}

	clientset, err := internalclientset.NewForConfig(incc)
	if err != nil {
		glog.Errorf("Error in creating secret clientset: %s", err)
		return nil
	}
	return &SecretToolSet{clientset: clientset}
}

func (st *SecretToolSet) CreateSecret(namespace string, secret *api.Secret) (*api.Secret, error) {
	return st.clientset.Core().Secrets(namespace).Create(secret)
}

func (st *SecretToolSet) DeleteSecret(namespace, name string) error {
	orphanDependents := false
	return st.clientset.Core().Secrets(namespace).Delete(name, &v1.DeleteOptions{OrphanDependents: &orphanDependents})
}
