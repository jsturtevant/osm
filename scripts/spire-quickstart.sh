#!/bin/bash

kubectl apply -f https://raw.githubusercontent.com/spiffe/spire-tutorials/main/k8s/quickstart/spire-namespace.yaml
kubectl apply \
    -f https://raw.githubusercontent.com/spiffe/spire-tutorials/main/k8s/quickstart/server-account.yaml \
    -f https://raw.githubusercontent.com/spiffe/spire-tutorials/main/k8s/quickstart/spire-bundle-configmap.yaml \
    -f https://raw.githubusercontent.com/spiffe/spire-tutorials/main/k8s/quickstart/server-cluster-role.yaml

kubectl apply \
    -f https://raw.githubusercontent.com/spiffe/spire-tutorials/main/k8s/quickstart/server-configmap.yaml \
    -f https://raw.githubusercontent.com/spiffe/spire-tutorials/main/k8s/quickstart/server-statefulset.yaml \
    -f https://raw.githubusercontent.com/spiffe/spire-tutorials/main/k8s/quickstart/server-service.yaml

kubectl apply \
    -f https://raw.githubusercontent.com/spiffe/spire-tutorials/main/k8s/quickstart/agent-account.yaml \
    -f https://raw.githubusercontent.com/spiffe/spire-tutorials/main/k8s/quickstart/agent-cluster-role.yaml

kubectl apply \
    -f https://raw.githubusercontent.com/spiffe/spire-tutorials/main/k8s/quickstart/agent-configmap.yaml \
    -f https://raw.githubusercontent.com/spiffe/spire-tutorials/main/k8s/quickstart/agent-daemonset.yaml