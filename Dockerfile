FROM golang:1.16-alpine3.13
ENV CGO_ENABLED=0
WORKDIR /go/src/

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o /usr/local/bin/function ./
RUN go install github.com/hashicorp/go-getter/cmd/go-getter@latest

#RUN apk update && apk add --no-cache curl

#############################################kk

FROM alpine:3.13
COPY --from=0 /usr/local/bin/function /usr/local/bin/function
COPY --from=0 /go/bin/go-getter /usr/local/bin/go-getter
#COPY --from=0 /usr/local/bin/helm /usr/local/bin/helm

RUN apk update && apk add --no-cache git openssh-client
RUN mkdir -p /r && chmod ugo+rwx /r

RUN mkdir -p /root/.ssh &&\
    ssh-keyscan -t rsa github.com >> /root/.ssh/known_hosts &&\
    ssh-keyscan -t rsa gitlab.com >> /root/.ssh/known_hosts &&\
    chmod og-rwx -R /root/.ssh

ENV PATH /usr/local/bin:$PATH

# FIXME
ARG DEBUG=True
ENV DEBUG $DEBUG

ARG RENDER_TEMP=/r/output
ENV RENDER_TEMP $RENDER_TEMP
ARG SOURCES_DIR=/r/tmpl
ENV SOURCES_DIR $SOURCES_DIR
ARG UPDATE_SOURCE=False
ENV UPDATE_SOURCE $UPDATE_SOURCE

ENTRYPOINT ["function"]
