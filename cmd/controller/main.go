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

package main

import (
	goflag "flag"
	"fmt"
	"github.com/golang/glog"
	controller "github.com/kubernetes-incubator/cluster-manager/pkg/controller/cluster"
	opt "github.com/kubernetes-incubator/cluster-manager/pkg/controller/cluster/options"
	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apiserver/pkg/util/flag"
	"k8s.io/apiserver/pkg/util/logs"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/kubernetes/pkg/version/verflag"
	"os"
)

var Version string

func main() {

	version := goflag.Bool("version", false, "Prints version and exits")

	options := opt.NewCCO()
	options.AddFlags(pflag.CommandLine)

	flag.InitFlags()
	logs.InitLogs()
	defer logs.FlushLogs()

	glog.Infof("Version: %s", Version)
	if *version {
		os.Exit(0)
	}

	verflag.PrintAndExitIfRequested()

	if err := Run(options); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

}

func Run(options *opt.ClusterControllerOptions) error {

	restClientCfg, err := clientcmd.BuildConfigFromFlags(options.FkubeApiServer, "/etc/federation/controller-manager/kubeconfig")

	if err != nil || restClientCfg == nil {
		glog.V(2).Infof("Unable to build the rest client config from flags: %v", err)
		return err
	}

	// Override restClientCfg qps/burst settings from flags
	restClientCfg.QPS = 20.0
	restClientCfg.Burst = 30
	run := func() {
		err := StartController(restClientCfg, options)
		glog.Fatalf("Error running cluster-manager controller: %v", err)
		panic("unreachable")
	}
	run()
	panic("unreachable")
}

func StartController(restClientCfg *restclient.Config, options *opt.ClusterControllerOptions) error {
	glog.V(1).Infof("Initializing cluster-manager controller...")
	stopChan := wait.NeverStop
	controller.StartClusterController(restClientCfg, options, stopChan)
	select {}
}
