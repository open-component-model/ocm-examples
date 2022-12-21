#!/bin/bash

set -o errexit

USER=$1
PAT=$2
EMAIL=$3

# **note** Currently we require OCI registry credentials to be configured twice, this is a known issue and will be resolved in a future update to the ocm-controller

kubectl create ns ocm-system > /dev/null 2>&1 || true

kubectl create secret docker-registry -n ocm-system regcred \
    --docker-server=ghcr.io \
    --docker-username=$USER \
    --docker-password=$PAT \
    --docker-email=$EMAIL || true

kubectl create secret generic -n ocm-system creds --from-literal=username=$USER --from-literal=password=$PAT ||  true

kubectl create secret generic -n ocm-system publickey --from-file=key=./component/key.pub
