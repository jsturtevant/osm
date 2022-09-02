#!/bin/bash

kubectl apply -f https://raw.githubusercontent.com/jsturtevant/spire-tutorials/osm/k8s/quickstart/spire-namespace.yaml
kubectl apply \
    -f https://raw.githubusercontent.com/jsturtevant/spire-tutorials/osm/k8s/quickstart/server-account.yaml \
    -f https://raw.githubusercontent.com/jsturtevant/spire-tutorials/osm/k8s/quickstart/spire-bundle-configmap.yaml \
    -f https://raw.githubusercontent.com/jsturtevant/spire-tutorials/osm/k8s/quickstart/server-cluster-role.yaml

kubectl apply \
    -f https://raw.githubusercontent.com/jsturtevant/spire-tutorials/osm/k8s/quickstart/server-configmap.yaml \
    -f https://raw.githubusercontent.com/jsturtevant/spire-tutorials/osm/k8s/quickstart/server-statefulset.yaml \
    -f https://raw.githubusercontent.com/jsturtevant/spire-tutorials/osm/k8s/quickstart/server-service.yaml

kubectl apply \
    -f https://raw.githubusercontent.com/jsturtevant/spire-tutorials/osm/k8s/quickstart/agent-account.yaml \
    -f https://raw.githubusercontent.com/jsturtevant/spire-tutorials/osm/k8s/quickstart/agent-cluster-role.yaml

kubectl apply \
    -f https://raw.githubusercontent.com/jsturtevant/spire-tutorials/osm/k8s/quickstart/agent-configmap.yaml \
    -f https://raw.githubusercontent.com/jsturtevant/spire-tutorials/osm/k8s/quickstart/agent-daemonset.yaml

# register agent
# note spiffe id for node agent cannot start with /spire 
# https://github.com/spiffe/spire/blob/f423beeb16098655674f97dba4bbb5bd0642a772/pkg/common/idutil/spiffeid.go#L39
kubectl wait -n spire -l statefulset.kubernetes.io/pod-name=spire-server-0 --for=condition=ready pod --timeout=-1s
kubectl exec -n spire spire-server-0 -- \
    /opt/spire/bin/spire-server entry create \
    -spiffeID spiffe://cluster.local/ns/spire/sa/spire-agent \
    -selector k8s_sat:cluster:osm \
    -selector k8s_sat:agent_ns:spire \
    -selector k8s_sat:agent_sa:spire-agent \
    -node

# workload registrations
kubectl exec -n spire spire-server-0 -- \
    /opt/spire/bin/spire-server entry create \
    -spiffeID spiffe://cluster.local/bookbuyer/bookbuyer \
    -parentID spiffe://cluster.local/ns/spire/sa/spire-agent \
    -selector k8s:ns:bookbuyer \
    -selector k8s:sa:bookbuyer 

kubectl exec -n spire spire-server-0 -- \
    /opt/spire/bin/spire-server entry create \
    -spiffeID spiffe://cluster.local/bookstore-v1/bookstore \
    -parentID spiffe://cluster.local/ns/spire/sa/spire-agent \
    -selector k8s:ns:bookstore \
    -selector k8s:sa:bookstore-v1 

kubectl exec -n spire spire-server-0 -- \
    /opt/spire/bin/spire-server entry create \
    -spiffeID spiffe://cluster.local/bookstore-v2/bookstore \
    -parentID spiffe://cluster.local/ns/spire/sa/spire-agent \
    -selector k8s:ns:bookstore \
    -selector k8s:sa:bookstore-v2

kubectl exec -n spire spire-server-0 -- \
    /opt/spire/bin/spire-server entry create \
    -spiffeID spiffe://cluster.local/bookwarehouse/bookwarehouse \
    -parentID spiffe://cluster.local/ns/spire/sa/spire-agent \
    -selector k8s:ns:bookwarehouse \
    -selector k8s:sa:bookwarehouse

kubectl exec -n spire spire-server-0 -- \
    /opt/spire/bin/spire-server entry create \
    -spiffeID spiffe://cluster.local/mysql/bookwarehouse \
    -parentID spiffe://cluster.local/ns/spire/sa/spire-agent \
    -selector k8s:ns:bookwarehouse \
    -selector k8s:sa:mysql

# leave out book thief and show how it can't get a secret.