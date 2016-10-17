#!/bin/bash

# Get the parent directory of where this script is.
SOURCE="${BASH_SOURCE[0]}"
while [ -h "$SOURCE" ] ; do SOURCE="$(readlink "$SOURCE")"; done
DIR="$( cd -P "$( dirname "$SOURCE" )/.." && pwd )"

# Change into that dir because we expect that.
cd $DIR

# we need a version if not stop
if [[ -z ${VERSION} ]];then
  echo "define env VERSION ..."
  exit 2
fi

# iterataion version for packages
DEB_INTERATION=${DEB_INTERATION:-"1"}

# By default compile for Linux/amd64.
if [[ -z $GOOS ]]; then
  GOOS=linux
fi

if [[ -z $GOARCH ]]; then
  GOARCH=amd64
fi

# Delete old files
echo "Removing old binary..."
rm -f bin/*
rm -rf dist/*.deb
mkdir -p bin/

for d in $(ls -d dist/*/); do
  rm -rf ${d}usr/bin
done

# build the bin
GOOS=linux GOARCH=amd64 make bin

# create deb packages 
cd dist

for d in $(ls); do
  mkdir -p ${d}/usr/bin
  cp ${DIR}/bin/flywheel ${d}/usr/bin
  fpm -s dir -t deb -C ${d} --name \
    flywheel --version ${VERSION} \
    --iteration ${d}-${DEB_INTERATION} \
    --license "Apache-2.0" \
    --maintainer "dejan.golja@fairfaxmedia.com.au" \
    --url "https://github.com/fairfaxmedia/flywheel" \
    --description "HTTP proxy for AWS cost control" .
  cd ..
done
