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

// This will be used by Provider APIs in kubeprovider.go to return Annotations that need to be created or deleted.
// Currently, this is only being used by ScaleKubeClusterUp, ScaleKubeClusterDown and ValidateKubeCluster
type ClusterAnnotation struct {
	Annotations       map[string]string
	DeleteAnnotations []string
}

func (ca *ClusterAnnotation) SetAnnotation(key string, value string) {
	if ca.Annotations == nil {
		ca.Annotations = make(map[string]string)
	}
	ca.Annotations[key] = value
}

func (ca *ClusterAnnotation) DeleteAnnotation(key string) {
	ca.DeleteAnnotations = append(ca.DeleteAnnotations, key)
}

func (ca *ClusterAnnotation) ApplyAnnotation(annotations map[string]string) {
	if ca.Annotations != nil && len(ca.Annotations) > 0 {
		for key, value := range ca.Annotations {
			annotations[key] = value
		}
	}
	if ca.DeleteAnnotations != nil && len(ca.DeleteAnnotations) > 0 {
		for _, key := range ca.DeleteAnnotations {
			delete(annotations, key)
		}
	}
}
