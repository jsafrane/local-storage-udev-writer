/*
Copyright 2018 The Kubernetes Authors.

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
	"flag"
	"fmt"
	"os"
	"os/signal"
	"regexp"
	"syscall"

	"github.com/golang/glog"

	"github.com/jsafrane/local-storage-udev-writer/pkg/udev-writer"
)

var (
	version string

	// This is the regexp that --name must conform to (namespace + '-' + CR name)
	// from k8s.io/apimachinery/pkg/util/validation/validation.go
	dns1123LabelFmt     string = "[a-z0-9]([-a-z0-9]*[a-z0-9])?"
	dns1123SubdomainFmt string = dns1123LabelFmt + "(\\." + dns1123LabelFmt + ")*"
)

var (
	dns1123SubdomainRegexp = regexp.MustCompile("^" + dns1123SubdomainFmt + "$")
)

func main() {
	var (
		configFile  string
		name        string
		rulesDir    string
		useNSEnter  bool
		hostProcDir string
	)

	flag.StringVar(&configFile, "config", "/etc/local-storage-discoverer/rules.conf", "path to the configuration file")
	flag.StringVar(&name, "name", "", "unique name of the nodeGroup")
	// Use /run/ as the default path for rules files so they are removed after reboot.
	flag.StringVar(&rulesDir, "rules-dir", "/run/udev/rules.d", "path to udev rules.d directory where to pud udev rules")
	flag.BoolVar(&useNSEnter, "use-nsenter", false, "use /bin/nsenter to enter host's mount namespace to execute udev commands")
	flag.StringVar(&hostProcDir, "host-proc-dir", "/rootfs/proc", "path to host's /proc filesystems")
	flag.Parse()

	printVersion()

	if len(configFile) == 0 {
		glog.Fatal("Config file must be specified")
	}
	if len(name) == 0 {
		glog.Fatal("Name must be specified")
	}

	// We put name into udev rules file, therefore make sure it can't be misused
	// to escape double quotes by "\" or escape /dev/disk by using ".."
	// Using DNS format from Kubernetes name validation.
	if !dns1123SubdomainRegexp.MatchString(name) {
		glog.Fatalf("Name %q is not a valid name. Regex used for validation: %q.", name, dns1123SubdomainFmt)
	}

	if err := os.MkdirAll(rulesDir, 0600); err != nil {
		glog.Fatalf("can't create directory for udev rules: %s", err)
	}

	var exec udevwriter.ExecInterface
	if useNSEnter {
		exec = udevwriter.NewNSEnterExec(hostProcDir)
		glog.V(2).Infof("using nsenter")
	} else {
		exec = udevwriter.NewExec()
	}

	// set up signals so we handle the first shutdown signal gracefully
	stopCh := SetupSignalHandler()

	u := udevwriter.NewUdevSync(configFile, rulesDir, name, exec)
	u.Run(stopCh)
}

func printVersion() {
	fmt.Printf("Local Storage Discoverer %s\n", version)
}

func SetupSignalHandler() (stopCh <-chan struct{}) {
	stop := make(chan struct{})
	c := make(chan os.Signal, 2)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		close(stop)
		<-c
		os.Exit(1) // second signal. Exit directly.
	}()

	return stop
}
