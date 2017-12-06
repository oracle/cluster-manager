//+build test

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

// This code will be included in the build of the unit test only when the "-tags test" is added in go test. The
// source block included in this file will contain customized components (like functions) specific for the need
// of unit test or to provide dummy implementation of some functions to replace the actual ones which will be
// excluded in the build of the unit test.

package kopsAws

import (
	"k8s.io/kops/util/pkg/vfs"
)

// This is a dummy newToolSet used to replace the actual one during unit test.
func newToolSet(kopsStatePath vfs.Path) toolSetInterface {
	return nil
}
