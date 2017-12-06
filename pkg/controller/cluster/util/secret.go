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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	clientcmdlatest "k8s.io/client-go/tools/clientcmd/api/latest"
	federationapi "k8s.io/kubernetes/federation/apis/federation"
	"k8s.io/kubernetes/pkg/api"
)

type SecretToolSetInterface interface {
	CreateSecret(namespace string, secret *api.Secret) (*api.Secret, error)
	DeleteSecret(namespace, name string) error
}

const (
	KubeconfigSecretDataKey = "kubeconfig"
)

func CreateKubeconfigSecret(secretToolSet SecretToolSetInterface, kubeconfig *clientcmdapi.Config, namespace, name, federationName, clusterName string, dryRun bool) (*api.Secret, error) {
	configBytes, err := Write(*kubeconfig)
	if err != nil {
		return nil, err
	}
	annotations := map[string]string{
		federationapi.FederationNameAnnotation: federationName,
	}

	if clusterName != "" {
		annotations[federationapi.ClusterNameAnnotation] = clusterName
	}

	// Build the secret object with the minified and flattened
	// kubeconfig content.
	secret := &api.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			Annotations: annotations,
		},
		Data: map[string][]byte{
			KubeconfigSecretDataKey: configBytes,
		},
	}

	if !dryRun {
		return secretToolSet.CreateSecret(namespace, secret)
	}
	return secret, nil
}

func DeleteKubeconfigSecret(secretToolSet SecretToolSetInterface, namespace string, name string) error {
	return secretToolSet.DeleteSecret(namespace, name)
}

func Write(config clientcmdapi.Config) ([]byte, error) {
	return runtime.Encode(clientcmdlatest.Codec, &config)
}
