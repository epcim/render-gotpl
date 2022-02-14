# render-gotpl

An KRM Function to render go templated manifests.

- [KRM Fn specification](https://github.com/kubernetes-sigs/kustomize/blob/master/cmd/config/docs/api-conventions/functions-spec.md)
- [go-getter](https://github.com/hashicorp/go-getter) is used to fetch sources
- gotpl + sprig rendering

TODO:
- render engine [gomplate](https://gomplate.ca/) is used to render templates

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
kustomize build --stack-trace --enable-alpha-plugins --network example --mount "type=bind,rw=true,src=$PWD/output,dst=/r/output"
```


## Render engine

- GotplRender
- ~Gomplate~ (Will fork later for independent Fn)


## Values

```yaml
values:
  nginx_cpu_request: "512m"
  nginx:
    cpu:
      limit:  "1000m"
    memory:
      limit:  "1024M"
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