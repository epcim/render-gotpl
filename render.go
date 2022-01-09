package main

// FIXME ^^ it might be indepentent pkg

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	//"fs"
	"io/ioutil"
	"log"
	"math/rand"
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
	kyaml "sigs.k8s.io/kustomize/kyaml/yaml"
)

// RenderPlugin is a plugin to generate k8s resources
// from a remote or local go templates.
type RenderPlugin struct {
	//types.GotplInflatorArgs
	PluginConfig

	sourcesDir string
	renderTemp string
	rf         *resmap.Factory
	rh         *resmap.PluginHelpers
}

// RemoteResource is specification for remote templates (git, s3, http...)
type RemoteResource struct {
	// local name for remote
	Name string `json:"name,omitempty" yaml:"name,omitempty"`
	// go-getter compatible uri to remote
	Repo string `json:"repo" yaml:"repo"`
	// go-getter creds profile for private repos, s3, etc..
	RepoCreds string `json:"repoCreds" yaml:"repoCreds"`
	// PLACEHOLDER, subPath at repo
	Path string `json:"path,omitempty" yaml:"path,omitempty"`
	// pull policy
	Pull string `json:"pull,omitempty" yaml:"pull,omitempty"`
	// template
	Template        string `json:"template,omitempty" yaml:"template,omitempty"`
	TemplatePattern string `json:"templatePattern,omitempty" yaml:"templatePattern,omitempty"`
	TemplateOpts    string `json:"templateOpts,omitempty" yaml:"templateOpts,omitempty"`
	// kinds
	Kinds []string `json:"kinds,omitempty" yaml:"kinds,omitempty"`

	// destDir is where the resource is cloned
	destDir string
}

type PluginConfig struct {
	//Engine       string                 `json:"engine,omitempty" yaml:"engine,omitempty"`
	//Config       string
	Sources []RemoteResource       `json:"sources,omitempty" yaml:"sources,omitempty"`
	Values  map[string]interface{} `json:"values,omitempty" yaml:"values,omitempty"`
}

var gotplFilePattern = "*.t*pl"
var renderedManifestFilePattern = "*.rendered.y*ml" // FIXME, prefix it
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
	//
	// Shameless copy from:
	// https://github.com/helm/helm/blob/master/pkg/engine/engine.go#L107
	//
	// Some more Helm template functions:
	// https://github.com/helm/helm/blob/master/pkg/engine/funcs.go
	//
	"toYaml": func(v interface{}) string {
		data, err := kyaml.Marshal(v) // FIXME yaml2
		if err != nil {
			// Swallow errors inside of a template.
			return ""
		}
		return strings.TrimSuffix(string(data), "\n")
	},
}

// Config uses the input plugin configurations `config` to setup the generator
//func (p *plugin) Config(
func (p *RenderPlugin) Config(h *resmap.PluginHelpers, config []byte) error {
	p.rh = h
	err := kyaml.Unmarshal(config, p)
	if err != nil {
		return err
	}
	return nil
}

// Generate fetch, render and return manifests from remote sources
func (p *RenderPlugin) Generate() (resMap resmap.ResMap, err error) {

	//DEBUG
	debug, _ := strconv.ParseBool(os.Getenv("DEBUG"))
	//for _, e := range os.Environ() {
	//    pair := strings.SplitN(e, "=", 2)
	//	fmt.Printf("#DEBUG %s='%s'\n", pair[0], pair[1])
	//}

	// FIXME - hardcoded /envs/ will go away and will be replaced by config option
	// var pluginConfigRoot = os.Getenv("KUSTOMIZE_PLUGIN_CONFIG_ROOT")
	// if os.Getenv("KUSTOMIZE_GOTPLINFLATOR_ROOT") == "" {
	// 	var envsPath = strings.SplitAfter(pluginConfigRoot, "/envs/")
	// 	if len(envsPath) > 1 {
	// 		os.Setenv("KUSTOMIZE_GOTPLINFLATOR_ROOT", filepath.Join(envsPath[0], "../repos"))
	// 		os.Setenv("ENV", strings.Split(envsPath[1], "/")[0])
	// 	}
	// }

	// where to fetch, render, otherwise tempdir
	//p.GotplInflatorRoot = os.Getenv("KUSTOMIZE_GOTPLINFLATOR_ROOT")

	// tempdir
	p.renderTemp, err = ensureWorkDir(os.Getenv("RENDER_TEMP"))
	if err != nil {
		return nil, fmt.Errorf("failed to create render dir: %w", err)
	}
	if !debug {
		defer os.RemoveAll(p.renderTemp)
	}

	p.sourcesDir, err = ensureWorkDir(os.Getenv("SOURCES_DIR"))
	if err != nil {
		return nil, fmt.Errorf("failed to create sources dir: %w", err)
	}

	// normalize values for template rendering
	// map[string]interface{} is flattened to . delimited keys
	nv := make(map[string]interface{})
	FlattenMap("", p.Values, nv)
	p.Values = nv

	//DEBUG
	for k, v := range p.Values {
		fmt.Printf("#%s:%s\n", k, v)
	}

	// fetch dependencies
	err = p.FetchSources()
	if err != nil {
		return nil, fmt.Errorf("failed to get remote source: %w", err)
	}

	// FIXME, in parralel to buffer
	// render to files
	// err = p.RenderSources()
	// if err != nil {
	// 	return nil, fmt.Errorf("template rendering failed: %v", err)
	// }
	p.rf = NewResMapFactory()
	resMap, err = p.RenderSources()
	if err != nil {
		return nil, fmt.Errorf("template rendering failed: %v", err)
	}
	// prepare output buffer
	// 	output.WriteString("\n---\n")
	// 	output.WriteString(`apiVersion: traefik.containo.us/v1alpha1
	// kind: IngressRoute
	// metadata:
	//   name: hass-server
	//   namespace: home
	// spec:
	//   entryPoints:
	//   - websecure
	//   routes:
	//   - kind: Rule
	//     match: HostRegexp('hass.apealive.{tld:(local|net)}')
	//     priority: 10
	//     services:
	//     - name: hass-home-assistant
	//       port: 8123
	//       namespace: home
	//   - kind: Rule
	//     match: HostRegexp('hass.apealive.{tld:(local|net)}')
	//     priority: 11
	//     services:
	//     - name: hass-home-assistant
	//       port: 8123
	//       scheme: h2c
	//       namespace: home
	//   tls:
	//     domains:
	//       - main: apealive.net
	//         sans:
	//         - h.apealive.net
	//         - hass.apealive.net
	//         - homeassistant.apealive.net
	// `)
	// 	output.WriteString("\n---\n")

	// collect, filter, parse manifests
	// err = p.ReadManifests(&output)
	// if err != nil {
	// 	return nil, fmt.Errorf("read manifest failed: %w", err)
	// }

	// not requeired in container
	// cleanup
	// var cleanRenderTemp = os.Getenv("RENDER_CLEANUP")
	// if cleanRenderTemo != "" {
	// 	err = p.CleanWorkdir()
	// 	if err != nil {
	// 		return nil, fmt.Errorf("Cleanup failed: %v", err)
	// 	}
	// }
	//resmap.NewPluginHelpers()
	//resmap.N
	//p.rf = resmap.NewFactory()

	//xx, aa = p.rf.NewResMapFromBytes(output.Bytes())

	//res p.h.ResmapFactory()

	//resMap, err = p.rf.NewResMapFromBytes(output.Bytes())

	//return nil, fmt.Errorf("debug: read these manifests %v, of len %d", resMap, len(output.Bytes()))

	// try to remove the contents before first "---" because debug, etc..
	// stdoutStr := output.String()
	// if idx := strings.Index(stdoutStr, "---"); idx != -1 {
	// 	return p.rf.NewResMapFromBytes([]byte(stdoutStr[idx:]))
	// }
	return resMap, err
}

// FetchSources calls go-getter to fetch remote sources
func (p *RenderPlugin) FetchSources() error {
	for idx, rs := range p.Sources {

		// ensure fetch destination (ie: <soucesDir>/reponame-branch)
		var err error
		rs.destDir, err = p.setFetchDst(idx)
		if err != nil {
			return fmt.Errorf("failed to process download destination for resource %s", rs.Name)
		}

		// skip fetch if is present and not forced
		//updateSource, _ := strconv.ParseBool(os.Getenv("UPDATE_SOURCE"))
		_, err = os.Stat(rs.destDir)
		// if err == nil && !updateSource { //FIXME
		// 	continue
		// }

		// ensuue destination
		if os.IsNotExist(err) {
			_ = os.MkdirAll(rs.destDir, 0770)
		}

		////DEBUG
		////fmt.println("# go-getter", rs.repo, repotempdir)
		//cmd := exec.Command("go-getter", rs.Repo, repotempdir)
		////cmd.stdout = os.stdout
		////cmd.stderr = os.stderr
		//err = cmd.Run()
		//if err != nil {
		//	return fmt.Errorf("go-getter failed to clone repo %s", err)
		//}

		//fetch
		pwd, err := os.Getwd()
		if err != nil {
			return err
		}
		opts := []getter.ClientOption{}

		//Chan, other options, ...
		//https://github.com/hashicorp/go-getter/blob/main/cmd/go-getter/main.go

		//Handle credentials
		//https://github.com/hashicorp/go-getter#git-git
		gettercreds, err := getRepoCreds(rs.RepoCreds)
		if err != nil {
			return err
		}

		client := &getter.Client{
			Ctx:     context.TODO(),
			Src:     rs.Repo + gettercreds,
			Dst:     rs.destDir,
			Pwd:     pwd,
			Mode:    getter.ClientModeAny,
			Options: opts,
		}

		// PLACEHOLDER detectors/getters
		//httpGetter := &getter.HttpGetter{
		//	Netrc: true,
		//}
		//	Detectors: []getter.Detector{
		//		new(getter.GitHubDetector),
		//		new(getter.GitLabDetector),
		//		new(getter.GitDetector),
		//		new(getter.S3Detector),
		//		new(getter.GCSDetector),
		//		new(getter.FileDetector),
		//		new(getter.BitBucketDetector),
		//	},
		//	Getters: map[string]getter.Getter{
		//		"file":  new(getter.FileGetter),
		//		"git":   new(getter.GitGetter),
		//		"hg":    new(getter.HgGetter),
		//		"s3":    new(getter.S3Getter),
		//		"http":  httpGetter,
		//		"https": httpGetter,
		//	},
		err = client.Get()
		if err != nil {
			return fmt.Errorf("failed to fetch source %w", err)
		}
	}
	return nil
}

//WalkMatch returns list of files matching regex pattern
func WalkMatch(root, pattern string) (found []string, err error) {
	err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
			// TODO recursive
		}
		if matched, err := filepath.Match(pattern, filepath.Base(path)); err != nil {
			return err
		} else if matched {
			found = append(found, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return found, nil
}

var files []os.FileInfo

func walker(path string, info os.FileInfo, err error) error {
	if strings.HasSuffix(info.Name(), ".txt") {
		files = append(files, info)
	}
	return nil
}

// RenderSources render gotpl manifests
func (p *RenderPlugin) RenderSources() (resMap resmap.ResMap, err error) {
	var out bytes.Buffer
	for _, rs := range p.Sources {
		// find templates
		if rs.TemplatePattern != "" {
			gotplFilePattern = rs.TemplatePattern
		}

		// DEBUG
		msg := fmt.Sprintf("debug: pattern %v", gotplFilePattern)
		writeDebug(&out, msg)

		var ffff string
		err := filepath.Walk("/tmp", walker)
		if err != nil {
			fmt.Errorf("failed %w", err)
		} else {
			for _, f := range files {
				ffff = ffff + f.Name()
				// This is where we'd like to open the file
			}
		}
		msg = fmt.Sprintf("debug: files v %s:  %v", "/tmp", ffff)
		writeDebug(&out, msg)

		templates, err := WalkMatch(rs.destDir, gotplFilePattern)

		// DEBUG
		// msg = fmt.Sprintf("debug templaters len %d", len(templates))
		// writeDebug(&out, msg)
		// msg = fmt.Sprintf("debug: templaters %v", templates)
		// writeDebug(&out, msg)

		//if errors.Is(err, fs.IsNotExist) {
		if os.IsNotExist(err) {
			continue
		} else if err != nil {
			return nil, err
		}

		// actual render, per template (feature: render all templates together per source)
		for _, t := range templates {
			out.WriteString("\n---\n")
			err := p.GotplRenderBuf(t, &out)
			if err != nil {
				return nil, err
			}
		}
	}

	resMap, err = p.rf.NewResMapFromBytes(out.Bytes())
	if err != nil {
		return nil, err
	}
	return resMap, nil
}

// GotplRenderBuf process templates to buffer
func (p *RenderPlugin) GotplRenderBuf(t string, out *bytes.Buffer) error {

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
	//tOpt := strings.Split(rs.TemplateOpts, ",")
	tpl := template.Must(
		//.Option(tOpt)
		//.ParseGlob("*.gotpl")
		template.New(t).Funcs(fMap).Parse(string(tContent)),
	)

	//render
	err = tpl.Execute(out, p.Values)
	if err != nil {
		return err
	}
	return nil
}

const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

func RandStringBytes(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = letterBytes[rand.Intn(len(letterBytes))]
	}
	return string(b)
}

func writeDebug(out *bytes.Buffer, msg string) {
	out.WriteString("\n---\n")
	out.WriteString(fmt.Sprintf(`apiVersion: v1
kind: ConfigMap
metadata:
  name: hass-server%s
  namespace: home
data:
  debug: "%s"
`, RandStringBytes(4), msg))
	out.WriteString("\n---\n")
}

// GotplRender process templates
func (p *RenderPlugin) GotplRender(t string) error {

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
	//tOpt := strings.Split(rs.TemplateOpts, ",")
	tpl := template.Must(
		//.Option(tOpt)
		//.ParseGlob("*.gotpl")
		template.New(t).Funcs(fMap).Parse(string(tContent)),
	)

	//render
	var rb bytes.Buffer
	err = tpl.Execute(&rb, p.Values)
	if err != nil {
		return err
	}

	// FIXME, add redner_ prefix
	// write
	tBasename := strings.TrimSuffix(t, filepath.Ext(t))
	tBasename = strings.TrimSuffix(t, filepath.Ext(tBasename)) // removes .yaml
	err = ioutil.WriteFile(tBasename+".rendered.yaml", rb.Bytes(), 0640)
	if err != nil {
		//log.Fatal("Write template failed:", err)
		return fmt.Errorf("write template failed: %w", err)
	}
	return nil
}

// setFetchDst update rs.Dir with path where the repository is fetched (ie: tempdir/reponame-branch)
func (p *RenderPlugin) setFetchDst(idx int) (string, error) {

	// remote resource spec
	rs := p.Sources[idx]

	// identify fetched repo with branch/commit/etc..
	var reporefSpec = strings.SplitAfter(rs.Repo, "ref=")
	var reporef string
	if len(reporefSpec) > 1 {
		reporef = strings.Split(reporefSpec[1], "?")[0]
	}
	var reporeferal = fmt.Sprintf("%s-%s", rs.Name, reporef)
	var repotempdir = filepath.Join(p.sourcesDir, reporeferal)
	rs.destDir = repotempdir
	if rs.Path != "" {
		// if subpath in repo is defined
		rs.destDir = filepath.Join(p.sourcesDir, reporeferal, rs.Path)
	}
	p.Sources[idx].destDir = rs.destDir
	return rs.destDir, nil
}

// Temporary
// CleanRenderTemp remove intermediate files
func (p *RenderPlugin) CleanRenderTemp() error {
	err := os.RemoveAll(p.renderTemp)
	if err != nil {
		return err
	}
	return nil
}

// ReadManifests locate & filter rendered manifests and print them in bytes.Buffer
func (p *RenderPlugin) ReadManifests(output *bytes.Buffer) error {
	for _, rs := range p.Sources {
		manifests, err := WalkMatch(rs.destDir, renderedManifestFilePattern)
		if os.IsNotExist(err) {
			continue
		} else if err != nil {
			return err
		}

		for _, m := range manifests {
			mContent, err := ioutil.ReadFile(m)
			if err != nil {
				return err
			}
			// TODO - to function
			// test/parse rendered manifest
			mk := make(map[interface{}]interface{})
			err = kyaml.Unmarshal([]byte(mContent), &mk) // yamlv2
			if err != nil {
				return err
			}
			// Kustomize lacks resource removal and multiple namespace manifests from dependencies cause `already registered id: ~G_v1_Namespace|~X|sre\`
			// https://kubectl.docs.kubernetes.io/faq/kustomize/eschewedfeatures/#removal-directives
			k := mk["kind"]
			if k != nil {
				kLcs := strings.ToLower(k.(string))
				if k == "namespace" || stringInSlice("!"+kLcs, rs.Kinds) {
					continue
				}
				if len(rs.Kinds) == 0 || stringInSlice(kLcs, rs.Kinds) {
					output.Write([]byte(mContent))
					output.WriteString("\n---\n")
				}
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
