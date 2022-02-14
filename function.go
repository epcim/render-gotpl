package main

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"strconv"

	//"github.com/GoogleContainerTools/kpt-functions-catalog/functions/go/render-gotpl/generated"
	//"github.com/epcim/kpt-functions-catalog/functions/go/render-gotpl/generated"
	//"github.com/GoogleContainerTools/kpt-functions-catalog/functions/go/render-gotpl/third_party/sigs.k8s.io/kustomize/api/builtins"

	"sigs.k8s.io/kustomize/api/resmap"
	kyaml "sigs.k8s.io/kustomize/kyaml/yaml"
)

const (
	fnGroup        = "fn.kpt.dev"
	fnVersion      = "v1"
	fnAPIVersion   = fnGroup + "/" + fnVersion
	fnKindGotpl    = "RenderGotpl"
	fnKindGomplate = "RenderGomplate"
)

type FunctionConfig struct {
	//Engine          string        `json:"engine,omitempty" yaml:"engine,omitempty"` // PLACEHOLDER
	//GomplateConfig  string        `json:"config,omitempty" yaml:"config,omitempty"` // PLACEHOLDER
	Sources []RemoteResource       `json:"sources,omitempty" yaml:"sources,omitempty"`
	Values  map[string]interface{} `json:"values,omitempty" yaml:"values,omitempty"`
}

// PLACEHOLDER, EngineInterface is facade for any render engine
// - decision to be taken whether to implement multiple engines
type EngineInterface interface {
	Generate() (resmap.ResMap, error)
}

// function implement https://github.com/kubernetes-sigs/kustomize/blob/master/cmd/config/docs/api-conventions/functions-spec.md#interface
type function struct {
	kyaml.ResourceMeta `json:",inline" yaml:",inline"`
	engine             GotplRender // EngineInterface
}

func (f *function) Config(rn *kyaml.RNode) error {
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
	case fnKindGotpl:
		err = f.configGotplRender([]byte(y))
		if err != nil {
			return err
		}
	case fnKindGomplate:
		return fmt.Errorf("`functionConfig`: `%s` not implemented", (fnKindGomplate))
	default:
		return fmt.Errorf("`functionConfig` must have kind: `%s`", (fnKindGotpl))
	}
	return nil
}

func (f *function) Run(items []*kyaml.RNode) ([]*kyaml.RNode, error) {

	// error logs from function
	log.SetOutput(os.Stderr)
	// FIXME, upstream will come with some extended logging from Fn, until that happen,
	// simply log to buffer. https://github.com/kubernetes-sigs/kustomize/issues/4398
	debug, _ := strconv.ParseBool(os.Getenv("DEBUG"))
	var buf bytes.Buffer
	if debug {
		log.SetOutput(&buf)
		log.SetFlags(log.Lshortfile)
	}
	defer func() {
		log.SetOutput(os.Stderr)
	}()

	// stdin
	resmapFactory := NewResMapFactory()
	resMap, err := resmapFactory.NewResMapFromRNodeSlice(items)
	if err != nil {
		return nil, err
	}
	var rm resmap.ResMap

	// render, process function
	rm, err = f.engine.Generate()
	if err != nil {
		return nil, fmt.Errorf("\n%s\nFailed to run function.engine.Generate(): %w", buf.String(), err)
	}

	// check for duplicates for idempotency
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

	// output
	err = resMap.AppendAll(rm)
	if err != nil {
		return nil, fmt.Errorf("failed to add generated resource: %w", err)
	}
	return resMap.ToRNodeSlice(), nil
}

// configGotplRender configure function engine
func (f *function) configGotplRender(c []byte) (err error) {

	// get plugin config from function spec.
	fnConfig := &FunctionConfig{}
	if err = kyaml.Unmarshal(c, fnConfig); err != nil {
		return err
	}

	// configure
	f.engine, err = NewEngine(fnConfig)
	if err != nil {
		return err
	}

	// TODO, enrich engine config
	// f.engine = GotplRender{
	// 	FunctionConfig: *fnConfig,
	// 	// gomplateConfig, other context, some credentials per source
	// 	// range args.Dependencies, update GotplOpts: args.GlobalXYZ
	// }

	return nil
}

// UTILS

func (f *function) getKind(rn *kyaml.RNode) (string, error) {
	meta, err := rn.GetMeta()
	if err != nil {
		return "", err
	}
	return meta.Kind, nil
}

func (f *function) validGVK(rn *kyaml.RNode, apiVersion, kind string) bool {
	meta, err := rn.GetMeta()
	if err != nil {
		return false
	}
	if meta.APIVersion != apiVersion || meta.Kind != kind {
		return false
	}
	return true
}
