#!/bin/bash

set -eux

MECAB_VERSION=0.996

mkdir build
pushd build

wget https://packages.groonga.org/source/groonga/groonga-${GROONGA_VERSION}.tar.gz
tar xf groonga-${GROONGA_VERSION}.tar.gz
pushd groonga-${GROONGA_VERSION}

pushd vendor
ruby download_mecab.rb
popd

cmake \
  -S . \
  -B ../groonga.build \
  --preset=release-maximum \
  -DCMAKE_INSTALL_PREFIX=/usr/local
cmake --build ../groonga.build
cmake --install ../groonga.build
popd

wget https://packages.groonga.org/source/pgroonga/pgroonga-${PGROONGA_VERSION}.tar.gz
tar xf pgroonga-${PGROONGA_VERSION}.tar.gz
pushd pgroonga-${PGROONGA_VERSION}
make PGRN_DEBUG=yes HAVE_MSGPACK=1 MSGPACK_PACKAGE_NAME=msgpack-c -j$(nproc)
make install
popd

git clone --branch "v$PGVECTOR_VERSION" https://github.com/pgvector/pgvector.git
pushd pgvector
make -j$(nproc)
make install
popd

popd
rm -rf build