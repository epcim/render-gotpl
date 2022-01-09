// Copyright 2021 Google LLC
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

	//"github.com/GoogleContainerTools/kpt-functions-catalog/functions/go/render-gotpl/generated"
	//FIXME,"github.com/epcim/kpt-functions-catalog/functions/go/render-gotpl/generated"
	//"github.com/GoogleContainerTools/kpt-functions-catalog/functions/go/render-gotpl/third_party/sigs.k8s.io/kustomize/api/builtins"

	"sigs.k8s.io/kustomize/api/resmap"
	"sigs.k8s.io/kustomize/kyaml/fn/framework"
	"sigs.k8s.io/kustomize/kyaml/fn/framework/command"
	kyaml "sigs.k8s.io/kustomize/kyaml/yaml"
)

const (
	fnConfigGroup      = "fn.kpt.dev"
	fnConfigVersion    = "v1"
	fnConfigAPIVersion = fnConfigGroup + "/" + fnConfigVersion
	legacyFnConfigKind = "RenderGotplConfig"
	fnConfigKind       = "RenderGotpl"
)

//nolint
func main() {
	asp := GotplProcessor{}
	cmd := command.Build(&asp, command.StandaloneEnabled, false)

	cmd.Short = "Render go templates"                                           // generated.RenderGotplShort
	cmd.Long = "Fetch and render .gotpl manifests from provided context values" //generated.RenderGotplLong
	//cmd.Example = generated.RenderGotplExamples
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}

type GotplProcessor struct{}

func (slp *GotplProcessor) Process(resourceList *framework.ResourceList) error {
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

type gotplRenderFunction struct {
	kyaml.ResourceMeta `json:",inline" yaml:",inline"`
	plugin             RenderPlugin
}

func (f *gotplRenderFunction) Config(rn *kyaml.RNode) error {
	y, err := rn.String()
	if err != nil {
		return fmt.Errorf("cannot get YAML from RNode: %w", err)
	}
	kind, err := f.getKind(rn)
	if err != nil {
		return err
	}

	switch kind {
	case "ConfigMap":
		return fmt.Errorf("imperative run from cli not implemnted")
	case fnConfigKind:
		err = f.ConfigPlugin([]byte(y))
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("`functionConfig` must have kind: `%s`", (fnConfigKind))
	}
	return nil
}

func (f *gotplRenderFunction) Run(items []*kyaml.RNode) ([]*kyaml.RNode, error) {
	resmapFactory := NewResMapFactory()
	resMap, err := resmapFactory.NewResMapFromRNodeSlice(items)
	if err != nil {
		return nil, err
	}
	var rm resmap.ResMap

	rm, err = f.plugin.Generate()
	if err != nil {
		return nil, fmt.Errorf("failed to run generator: %w", err)
	}

	// // check for duplicates for idempotency
	// for i := range items {
	// 	resources := rm.Resources()
	// 	for r := range resources {
	// 		it := &resource.Resource{RNode: *items[i]}
	// 		if resources[r].CurId() == it.CurId() {
	// 			// don't attempt to add a resource with the same ID
	// 			err := rm.Remove(resources[r].CurId())
	// 			if err != nil {
	// 				return items, err
	// 			}
	// 		}
	// 	}
	// }

	err = resMap.AppendAll(rm)
	if err != nil {
		return nil, fmt.Errorf("failed to add generated resource: %w", err)
	}

	return resMap.ToRNodeSlice(), nil
}

func run(resourceList *framework.ResourceList) error {
	var fn gotplRenderFunction
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

func (f *gotplRenderFunction) ConfigPlugin(c []byte) (err error) {
	fnConfig := &PluginConfig{}
	if err = kyaml.Unmarshal(c, fnConfig); err != nil {
		return
	}

	f.plugin = RenderPlugin{
		PluginConfig: *fnConfig,

		// POSSIBLE FEATURES
		// enrich with some global values mapping
		// gomplateConfig, other context, some credentials per source
		// range args.Dependencies, update GotplOpts: args.GlobalXYZ
		// some tempDirs, local sources
	}
	return nil
}

func (f *gotplRenderFunction) getKind(rn *kyaml.RNode) (string, error) {
	meta, err := rn.GetMeta()
	if err != nil {
		return "", err
	}
	return meta.Kind, nil
}

func (f *gotplRenderFunction) validGVK(rn *kyaml.RNode, apiVersion, kind string) bool {
	meta, err := rn.GetMeta()
	if err != nil {
		return false
	}
	if meta.APIVersion != apiVersion || meta.Kind != kind {
		return false
	}
	return true
}
