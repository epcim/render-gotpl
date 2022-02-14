// Copyright 2022 Petr Michalec (epcim@apealive.net)
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"fmt"
	"os"

	"sigs.k8s.io/kustomize/kyaml/fn/framework"
	"sigs.k8s.io/kustomize/kyaml/fn/framework/command"
)

type Processor struct{}

//nolint
func main() {
	asp := Processor{}
	cmd := command.Build(&asp, command.StandaloneEnabled, false)

	cmd.Short = "Inflate golang templates"
	cmd.Long = "Fetch and render gotpl kubernetes manifests"
	//cmd.Example =
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func (slp *Processor) Process(resourceList *framework.ResourceList) error {
	err := run(resourceList)
	if err != nil {
		resourceList.Result = &framework.Result{
			Name: "render-gotpl",
			Items: []framework.ResultItem{
				{
					Message:  err.Error(),
					Severity: framework.Error,
				},
			},
		}
		return resourceList.Result
	}
	return nil
}

func run(resourceList *framework.ResourceList) error {
	var fn function
	err := fn.Config(resourceList.FunctionConfig)
	if err != nil {
		return fmt.Errorf("failed to configure function: %w", err)
	}
	resourceList.Items, err = fn.Run(resourceList.Items)
	if err != nil {
		//panic(err)
		return fmt.Errorf("failed to run function: %w", err)
	}
	return nil
}
