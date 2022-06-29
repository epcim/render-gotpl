#!/bin/bash

## DRAFT
## SH implementation of Gotpl render KRM Fn (this)
## GO implementation of Gotpl render KRM Fn (https://github.com/epcim/render-gotpl-fn)

# $1 - optional path to manifests
[[ -z "$1" ]] || export TMPLPTH=$1
[[ -z "$2" ]] || export CONTEXT=$2

# defaults
export EXCLUDE_KIND=namespace
export GOTPLRENDER=gomplate
which gomplate &>/dev/null || export GOTPLRENDER="docker run -i --rm=true -u ${UUID} -v ${WORKDIR}:${WORKDIR} -w ${WORKDIR} hairyhenderson/gomplate:latest gomplate"

# read `kind: ResourceList` from stdin
read -r -t 1 -d $'\0' resourceList

# config
TMPLPTH=${TMPLPTH:-$(echo "$resourceList" | yq e '.functionConfig.spec.templates' -)}
CONTEXT=$(echo "$resourceList" | yq e '.functionConfig.spec.context' -)

# fixtures, to allow ./renderGotplFn.sh exection without KRM engine
CONTEXT=${CONTEXT//null/$(dirname $TMPLPTH)\/..\/context.yaml}
TMPLPTH=${TMPLPTH//null/$PWD\/manifests}

# print `kind: ResourceList` form $1/*.y*ml
function printResourceList() {
  echo "
kind: ResourceList
items:"
  # for each file, print individual documents as list & indent
  for f in $(ls $1/${2:-*.y*aml}); do 
    echo "---"; cat $f;
  done | awk '/./{print}' |\
      awk -v RS="---\n" '{printf "- %s",$0;}' |\
      sed -e 's/^/  /' -e 's/^  \-/-/' -e 's/- -[ -]*/- /g'
}

# stop if sourced
[[ "$0" != "$BASH_SOURCE" ]] && return

# serectl render & print
test -e $CONTEXT && export CONTEXT="-c .=$CONTEXT" || export CONTEXT=
${GOTPLRENDER} $CONTEXT --input-dir=$TMPLPTH --output-map=$TMPLPTH'/{{ .in | strings.ReplaceAll ".yaml.tmpl" ".yaml" }}' &> .log &&\
  printResourceList "$TMPLPTH" '*.y*ml' || {
    cat .log;
    rm .log;
  }

