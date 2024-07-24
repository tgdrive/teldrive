FROM postgres:16-alpine


ARG PGROONGA_VERSION=3.2.1
ARG GROONGA_VERSION=14.0.5
ARG PGVECTOR_VERSION=0.7.3

ENV PGROONGA_VERSION=${PGROONGA_VERSION} \
    GROONGA_VERSION=${GROONGA_VERSION} \
    PGVECTOR_VERSION=${PGVECTOR_VERSION}

COPY build.sh /
RUN chmod +x /build.sh
RUN \
  apk add --no-cache --virtual=.build-dependencies \
    apache-arrow-dev \
    build-base \
    clang15-dev \
    cmake \
    git \
    gettext-dev \
    linux-headers \
    llvm15 \
    lz4-dev \
    msgpack-c-dev \
    rapidjson-dev \
    ruby \
    samurai \
    xsimd-dev \
    xxhash-dev \
    zlib-dev \
    zstd-dev && \
  /build.sh && \
  rm -f build.sh && \
  apk del .build-dependencies && \
  apk add --no-cache \
    libarrow \
    libxxhash \
    msgpack-c \
    zlib \
    zstd