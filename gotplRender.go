package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"io/fs"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"

	"sigs.k8s.io/kustomize/api/hasher"
	"sigs.k8s.io/kustomize/api/resmap"
	"sigs.k8s.io/kustomize/api/resource"

	"github.com/Masterminds/sprig"
	getter "github.com/hashicorp/go-getter"
	"github.com/pkg/errors"
	kyaml "sigs.k8s.io/kustomize/kyaml/yaml"

	"github.com/k0kubun/pp"
)

const recursionMaxNums = 1000

var templateOpts = []string{} // PLACEHOLDER
var _ EngineInterface = (*GotplRender)(nil)

// GotplRender is engine plugin for render-gotpl-fn
// from a remote or local go templates render,filter k8s manifests
type GotplRender struct {
	FunctionConfig

	sourcesDir string
	renderTemp string
	rf         *resmap.Factory
	//rh         *resmap.PluginHelpers
	engine *template.Template
}


func NewEngine(fnConfig *FunctionConfig) (GotplRender, error) {
	r := GotplRender{
		FunctionConfig: *fnConfig,
		//Logger: *fnLogger,

		// TODO,
		// enrich with some global values mapping
		// gomplateConfig, other context, some credentials per source
		// range args.Dependencies, update GotplOpts: args.GlobalXYZ
		// some tempDirs, local sources
	}
	return r, nil
}

// SprigCustomFuncs are custom functions for Gotpl engine
var SprigCustomFuncs = map[string]interface{}{
	"handleEnvVars": func(rawEnvs interface{}) map[string]string {
		envs := map[string]string{}
		if str, ok := rawEnvs.(string); ok {
			err := json.Unmarshal([]byte(str), &envs)
			if err != nil {
				log.Fatal("failed to unmarshal Envs,", err)
			}
		}
		return envs
	},
	"toBool": func(value interface{}) bool {
		if value == nil {
			return false
		}
		switch value.(type) {
		case bool:
			return value.(bool)
		case int:
			if value.(int) >= 1 {
				return true
			}
		case string:
			switch strings.ToLower(value.(string)) {
			case "true", "yes", "on", "enable", "enabled", "1":
				{
					return true
				}
			}
		}
		return false
	},
	//
	// Shameless copy from:
	// https://github.com/helm/helm/blob/master/pkg/engine/engine.go#L107
	"toYaml": func(v interface{}) string {
		data, err := kyaml.Marshal(v) // FIXME yaml2
		if err != nil {
			// Swallow errors inside of a template.
			return ""
		}
		return strings.TrimSuffix(string(data), "\n")
	},
	// Some more Helm template functions:
	// https://github.com/helm/helm/blob/master/pkg/engine/funcs.go
}

// Config uses the input plugin configurations `config` to setup the generator
// func (p *GotplRender) Config(h *resmap.PluginHelpers, config []byte) error {
// 	p.rh = h
// 	err := kyaml.Unmarshal(config, p)
// 	if err != nil {
// 		return err
// 	}
// 	return nil
// }

// Generate fetch, render and return manifests from remote sources
func (p *GotplRender) Generate() (resMap resmap.ResMap, err error) {
	log.Println("Generate")

	// tempdir render
	p.renderTemp, err = ensureWorkDir(os.Getenv("RENDER_TEMP"))
	if err != nil {
		return nil, fmt.Errorf("failed to create render dir: %w", err)
	}

	// cleanup
	// defer os.RemoveAll(p.renderTemp)

	// tempdir sources
	p.sourcesDir, err = ensureWorkDir(os.Getenv("SOURCES_DIR"))
	if err != nil {
		return nil, fmt.Errorf("failed to create sources dir: %w", err)
	}

	// update,validate source spec
	log.Println("EvalSources")
	err = p.EvalSources()
	if err != nil {
		return nil, fmt.Errorf("failed to validate source: %w", err)
	}

	// fetch dependencies
	log.Println("FetchSources")
	err = p.FetchSources()
	if err != nil {
		return nil, fmt.Errorf("failed to get remote source: %w", err)
	}

	// render
	log.Println("RednerSources")
	p.rf = NewResMapFactory()
	resMap, err = p.RenderSources()
	if err != nil {
		return nil, fmt.Errorf("template rendering failed: %v", err)
	}
	return resMap, err
}

// TODO, move source processing to independent pkg or function.go
// Eval sources to update/enrich/validate
func (p *GotplRender) EvalSources() (err error) {
	for idx, rs := range p.Sources {
		// update destDir
		p.Sources[idx].destDir = filepath.Join(p.sourcesDir, rs.Name)

		// normalize Values for rendering, ie: `server:port:111` to `.server_port: 111`
		// map[string]interface{} keys are flattened to single level key `_` delimited
		nv := make(map[string]interface{})
		if len(rs.FlattenValuesBy) > 0 {
			FlattenMap(rs.FlattenValuesBy, p.Values, nv)
		} else {
			FlattenMap(DefaultResource.FlattenValuesBy, p.Values, nv)
		}
		p.Values = nv
	}
	return nil
}

// TODO, move source processing to independent pkg or function.go
// FetchSources calls go-getter to fetch remote sources
func (p *GotplRender) FetchSources() (err error) {
	for _, rs := range p.Sources {

		// ensure fetch destination (ie: <sourcesDir>/<sourceName>)
		// if _, err := os.Stat(rs.destDir); os.IsNotExist(err) {
		// 	_ = os.MkdirAll(rs.destDir, 0770)
		// }

		// skip if update is not requested
		updateSource, err := strconv.ParseBool(os.Getenv("UPDATE_SOURCE"))
		if err != nil {
			updateSource = false
		}
		_, err = os.Stat(rs.destDir)
		if !os.IsNotExist(err) && (!rs.Update || !updateSource) {
			continue
		}

		//fetch
		pwd, err := os.Getwd()
		if err != nil {
			return err
		}
		opts := []getter.ClientOption{}

		//options ...
		//https://github.com/hashicorp/go-getter/blob/main/cmd/go-getter/main.go
		//https://github.com/hashicorp/go-getter#git-git
		gettercreds, err := getRepoCreds(rs.RepoCreds)
		if err != nil {
			return err
		}

		client := &getter.Client{
			Ctx: context.TODO(),
			// FIXME, private repos, go-getter, credentials does not work this way
			// we use workaround, volume mount with all required repositories prefetched
			// unfortunatelly special chars in ?sshkey= gets escaped
			// further, kustomize team will remove ENV support from function calls
			// mounting key as volume per function specification is dumb, insecure
			Src:     fmt.Sprintf("%s%s", rs.Repo, gettercreds),
			Dst:     rs.destDir,
			Pwd:     pwd,
			Mode:    getter.ClientModeAny,
			Options: opts,
		}

		err = client.Get()
		if err != nil {
			return fmt.Errorf("failed to fetch source %w", err)
		}
	}
	return nil
}

// RenderSources render gotpl manifests
func (p *GotplRender) RenderSources() (resMap resmap.ResMap, err error) {
	var out bytes.Buffer
	resMap = resmap.New()
	for _, rs := range p.Sources {

		if rs.TemplateGlob == "" {
			rs.TemplateGlob = DefaultResource.TemplateGlob
		}

		// find templates
		templates, err := TemplateFinder(rs.destDir, rs.TemplateGlob)
		if os.IsNotExist(err) {
			continue
		} else if err != nil {
			return nil, err
		}

		// actual render
		for _, t := range templates {
			out.WriteString("\n---\n")
			err := p.GotplRenderBuf(t, &out)
			if err != nil {
				return nil, err
			}
		}

		// convert to resMap (bytes)
		resMapSrc, err := p.rf.NewResMapFromBytes(out.Bytes())
		if err != nil {
			return nil, err
		}

		// filter kinds
		if len(rs.Kinds) == 0 {
			rs.Kinds = DefaultResource.Kinds
		}
		err = p.FilterKinds(rs.Kinds, resMapSrc)
		if err != nil {
			return nil, fmt.Errorf("failed to filter kinds %s for source %s", strings.Join(rs.Kinds, ","), rs.Name)
		}

		// append single source to output
		err = resMap.AppendAll(resMapSrc)
		if err != nil {
			return nil, err
		}
	}

	// convert to kyaml resource map
	return resMap, nil
}

// GotplRenderBuf process templates to buffer
func (p *GotplRender) GotplRenderBuf(t string, out *bytes.Buffer) error {

	// read template
	tContent, err := ioutil.ReadFile(t)
	if err != nil {
		return fmt.Errorf("read template failed: %w", err)
	}

	// init
	fMap := sprig.TxtFuncMap()
	for k, v := range SprigCustomFuncs {
		fMap[k] = v
	}
	p.engine = template.New(t)
	p.engine.Funcs(fMap).Option(templateOpts...)
	contextFuncs(p.engine)

	tpl := template.Must(
		//.ParseGlob("*.gotpl")
		p.engine.Parse(string(tContent)),
		//template.New(t).Funcs(fMap).Parse(string(tContent)),
	)

	//render
	err = tpl.Execute(out, p.Values)
	if err != nil {
		log.Printf("Failed to render template %s, with context: %s\n", filepath.Base(t), pp.Sprint(p.Values))
		return err
	}
	return nil
}

// FilterKinds
// https://kubectl.docs.kubernetes.io/faq/kustomize/eschewedfeatures/#removal-directives
// Kustomize lacks resource removal and multiple namespace manifests from bases, causing
// `already registered id: ~G_v1_Namespace|~X|sre\`
func (p *GotplRender) FilterKinds(kinds []string, rm resmap.ResMap) error {

	// per kinds item in soruce config
	// - !namespace,secrets    # to remove
	// - Deployment,ConfigMap  # to keep, but no glob
	for _, kindsItem := range kinds {
		negativeF := strings.Contains(kindsItem, "!")
		kindsItem := strings.ToLower(kindsItem)
		kindsItem = strings.ReplaceAll(kindsItem, "!", "")
		kindsList := strings.Split(kindsItem, ",")

		// across all resoures
		resources := rm.Resources()
		for r := range resources {
			k := strings.ToLower(resources[r].GetKind())
			if filterListFn(kindsList, negativeF, k) {
				rm.Remove(resources[r].CurId())
			}
		}
	}
	return nil
}

func NewResMapFactory() *resmap.Factory {
	resourceFactory := resource.NewFactory(&hasher.Hasher{})
	resourceFactory.IncludeLocalConfigs = true
	return resmap.NewFactory(resourceFactory)
}

// UTILS

//TemplateFinder returns list of files matching regex pattern
func TemplateFinder(root, pattern string) (found []string, err error) {
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if !d.IsDir() {
			baseFileName := filepath.Base(path)
			if matched, err := filepath.Match(pattern, baseFileName); err != nil {
				return err
			} else if matched {
				// `_` prefixed templates are not written to output
				// ie: _helpers.tpl
				if !strings.HasPrefix(baseFileName, "_") {
					found = append(found, path)
				}
				return nil
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return found, nil
}

// removeFilter returns true matching kinds
func filterListFn(list []string, negativeFilter bool, k string) bool {
	k = strings.TrimSpace(k)
	if negativeFilter {
		if stringInSlice(k, list) {
			return true
		}
	} else {
		if !stringInSlice(k, list) {
			return true
		}
	}
	return false
}

// ensureWorkDir prepare working directory
func ensureWorkDir(dir string) (string, error) {
	var err error
	if dir == "" {
		dir, err = ioutil.TempDir("", "fnGotplRender_")
		if err != nil {
			return "", err
		}
	} else {
		// create if missing
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			err := os.MkdirAll(dir, 0770)
			if err != nil {
				return "", err
			}
		}
	}
	return dir, nil
}

// stringInSlice boolean function
func stringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}

// FlattenMap flatten context values to snake_case
// How about https://godoc.org/github.com/jeremywohl/flatten
func FlattenMap(prefix string, src map[string]interface{}, dest map[string]interface{}) {
	for k, v := range src {
		switch child := v.(type) {
		case map[string]interface{}:
			FlattenMap(prefix+k, child, dest)
		case []interface{}:
			dest[k] = v
		default:
			dest[k] = fmt.Sprintf("%v", v)
		}
	}
}

//getRepoCreds read reference to credentials and returns go-getter URI
func getRepoCreds(repoCreds string) (string, error) {
	var cr = ""
	if repoCreds != "" {
		for _, e := range strings.Split(repoCreds, ",") {
			pair := strings.SplitN(e, "=", 2)
			//sshkey - for private git repositories
			if pair[0] == "sshkey" {
				key, err := ioutil.ReadFile(pair[1])
				if err != nil {
					return cr, err
				}
				keyb64 := base64.StdEncoding.EncodeToString([]byte(key))
				cr = fmt.Sprintf("%s?sshkey=%s", cr, string(keyb64))
			}
		}
	}
	return cr, nil
}

// contextFuncs adds context-specific functions to render engine
func contextFuncs(t *template.Template) {
	includedNames := make(map[string]int)
	funcMap := template.FuncMap{}

	// Add the `include` function adds posibility to `{{ define "xyz" }}` templates.
	funcMap["include"] = func(name string, data interface{}) (string, error) {
		var buf strings.Builder
		if v, ok := includedNames[name]; ok {
			if v > recursionMaxNums {
				return "", errors.Wrapf(fmt.Errorf("unable to execute template"), "rendering template has a nested reference name: %s", name)
			}
			includedNames[name]++
		} else {
			includedNames[name] = 1
		}
		err := t.ExecuteTemplate(&buf, name, data)
		includedNames[name]--
		return buf.String(), err
	}

	// Add the `required` function to fail on missing params or it's value
	funcMap["required"] = func(warn string, val interface{}) (interface{}, error) {
		if val == nil {
			return val, errors.Errorf("Missing value. %s (nil)", warn)
		} else if _, ok := val.(string); ok {
			if val.(string) == "" {
				return val, errors.Errorf("Missing value. %s %s", warn, val)
			}
		}
		return val, nil
	}

	t.Funcs(funcMap)
}

func dump(data interface{}){
    b,_:=json.MarshalIndent(data, "", "  ")
    fmt.Print(string(b))
}