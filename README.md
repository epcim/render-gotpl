# render-gotpl

An KRM Function to render go templated manifests.
An generator to be used with Kubectl, Kustomize or Kpt...

FEATURES:
- [go-getter](https://github.com/hashicorp/go-getter) interface to fetch dependencies
- render gotpl templates with [sprig library](https://github.com/Masterminds/sprig) and custom functions

TODO:
- add render engine [gomplate](https://gomplate.ca/)
- independent pkg to fetch `sources`

## Usage
```sh
# build
docker build -t render-gotpl .

# usage (public repos)
kustomize build --enable-alpha-plugins --network ./example

# usage (private repos, mount pre-fetched repositories)
# sources:
# - name: minio
#   repo: /r/repos/cicd-deployment//minio/k8s
kustomize build --enable-alpha-plugins --network --mount type=bind,src="$PWD/.repos",dst=/r/repos .

# dev
# see dockerfile for ENV variables
kustomize build --stack-trace --enable-alpha-plugins --network example --mount "type=bind,rw=true,src=$PWD/.output,dst=/r/output"
```

Check this [Makefile](https://github.com/epcim/gitops-infra/blob/master/Makefile) and repo that wrap kustomize ans provide simple CLI to render,build,apply,diff. 
(I use this for bootstrap before proper CI/CD is in place. Hopefully Kustomize, KRM Fn will get better support in CD tools soon.)

## Function

[KRM Fn specification](https://github.com/kubernetes-sigs/kustomize/blob/master/cmd/config/docs/api-conventions/functions-spec.md)

See upstream/other function examples:
- https://github.com/GoogleContainerTools/kpt-functions-catalog/blob/master/functions
- https://github.com/epcim/render-jsonnet-fn


## Rendering

- GotplRender (internal)
- ~Gomplate~ (3rd party with external source support and tons of features)


## Values

```yaml
values:
  nginx_cpu_request: "512m"
  nginx:
    cpu:
      limit:  "1000m"
    memory:
      limit:  "1024M"
  some:
  - list
```

GotplRender will either flatten all nested keys, so `nginx_memory_limit: 1024` can be used in templates.

## Sources

### Public repos

```yaml
sources:
- name: example
  repo: git::https://github.com/epcim/k8s-kustomize-apps//example/manifests?ref=main
```

See go-getter documentation for more details: https://github.com/hashicorp/go-getter#url-format

### Private repos

DRAFT, not working yet, waiting for some best practice from upstream.
Current interface to function does not allow to easily do such thing.

From private repository, with ssh key or token mounted.
```yaml
apiVersion: fn.kpt.dev/v1
kind: RenderGotpl
metadata:
  name: render
  annotations:
    config.kubernetes.io/function: |
      container:
        network: true
        image: render-gotpl
        mounts:
        - type: bind
          src: /Users/xxx/.ssh/id_rsa
          dst: /tmp/id_rsa
sources:
- name: minio
  repo: git@gitlab.com:xxx/yyyy/cicd-deployment//minio/k8s?ref=master
  repoCreds: sshkey=/tmp/id_rsa
```

