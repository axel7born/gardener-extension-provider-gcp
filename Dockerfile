############# builder
FROM golang:1.20.5 AS builder

WORKDIR /go/src/github.com/gardener/gardener-extension-provider-gcp
COPY . .

ARG EFFECTIVE_VERSION

RUN make install EFFECTIVE_VERSION=$EFFECTIVE_VERSION

############# base image
FROM gcr.io/distroless/static-debian11:nonroot AS base

############# gardener-extension-provider-gcp
FROM base AS gardener-extension-provider-gcp
WORKDIR /

COPY charts /charts
COPY --from=builder /go/bin/gardener-extension-provider-gcp /gardener-extension-provider-gcp
ENTRYPOINT ["/gardener-extension-provider-gcp"]

############# gardener-extension-admission-gcp
FROM base AS gardener-extension-admission-gcp
WORKDIR /

COPY --from=builder /go/bin/gardener-extension-admission-gcp /gardener-extension-admission-gcp
ENTRYPOINT ["/gardener-extension-admission-gcp"]
