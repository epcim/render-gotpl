# render-gotpl

An KRM Function to render go templated manifests.
An generator to be used with Kubectl, Kustomize or Kpt...

## Usage: Shell implementation

An prototype.

```
kustomize build --enable-alpha-plugins --network --enable-exec --load-restrictor LoadRestrictionsNone ./example-exec
```

## Usage: Go implementation

Features:
- [go-getter](https://github.com/hashicorp/go-getter) interface to fetch dependencies
- render gotpl templates with [sprig library](https://github.com/Masterminds/sprig) and custom functions
- can render non-helm git repositories, subpaths etc..
- filter Kinds

Build:
```sh
docker build -t render-gotpl .

go build .
```

Kustomize usage:
```
kustomize build --enable-alpha-plugins --network ./example-container
```

In future `kubectl` versions:
```
kubectl -k apply ./example-exec
```


## Function

[KRM Fn specification](https://github.com/kubernetes-sigs/kustomize/blob/master/cmd/config/docs/api-conventions/functions-spec.md)

See upstream/other function examples:
- https://github.com/GoogleContainerTools/kpt-functions-catalog/blob/master/functions

My other functions:
- https://github.com/epcim/render-jsonnet-fn

Notable to mention:
- helm render function:
  - https://github.com/GoogleContainerTools/kpt-functions-catalog/tree/master/functions/go/render-helm-chart
  - https://github.com/GoogleContainerTools/kpt-functions-catalog/tree/master/examples/render-helm-chart-kustomize-private-git
  - https://github.com/GoogleContainerTools/kpt-functions-catalog/tree/master/examples/render-helm-chart-kustomize-inline-values


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

WORKAROUND than solution, waiting for some best practice from upstream.
Current interface to function does not allow to easily do such thing.

See this either: [render-helm-chart-kustomize-private-git](https://github.com/GoogleContainerTools/kpt-functions-catalog/tree/master/examples/render-helm-chart-kustomize-private-git)


Works:
```
# usage (private repos, mount pre-fetched repositories)
sources:
- name: minio
  repo: /r/repos/cicd-deployment//minio/k8s

# then:
kustomize build --enable-alpha-plugins --network --mount type=bind,src="$PWD/.repos",dst=/r/repos .

# dev
# see dockerfile for ENV variables
kustomize build --stack-trace --enable-alpha-plugins --network example --mount "type=bind,rw=true,src=$PWD/.output,dst=/r/output"
```

Not working:
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

