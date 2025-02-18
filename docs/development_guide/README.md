# Open Service Mesh Development Guide

- [Open Service Mesh Development Guide](#open-service-mesh-development-guide)
  - [Get Go-ing](#get-go-ing)
  - [Get the dependencies](#get-the-dependencies)
      - [Makefile](#makefile)
  - [Create Environment Variables](#create-environment-variables)
  - [Build and push OSM images](#build-and-push-osm-images)
    - [Examples](#examples)
  - [Code Formatting](#code-formatting)
  - [Putting it all together (inner development loop)](#putting-it-all-together-inner-development-loop)
    - [Making changes to OSM](#making-changes-to-osm)
    - [Using Tilt](#using-tilt)
  - [Testing your changes](#testing-your-changes)
      - [Unit Tests](#unit-tests)
        - [Mocking](#mocking)
      - [Integration Tests](#integration-tests)
    - [End-to-End (e2e) Tests](#end-to-end-e2e-tests)
      - [Simulation / Demo](#simulation--demo)
      - [Profiling](#profiling)
  - [Helm charts](#helm-charts)
    - [Custom Resource Definitions](#custom-resource-definitions)
    - [Updating Dependencies](#updating-dependencies)
  
Welcome to the Open Service Mesh development guide!
Thank you for joining us on a journey to build an SMI-native lightweight service mesh. The first of our [core principles](https://github.com/openservicemesh/osm#core-principles) is to create a system, which is "simple to understand and contribute to." We hope that you would find the source code easy to understand. If not - we invite you to help us fulfill this principle. There is no PR too small!

To understand _what_ Open Service Mesh does - take it for a spin and kick the tires. Install it on your Kubernetes cluster by following [the getting started guide](https://docs.openservicemesh.io/docs/getting_started/).

To get a deeper understanding of how OSM functions - take a look at the detailed [software design](/DESIGN.md).

When you are ready to jump in - [fork the repo](https://docs.github.com/en/github/getting-started-with-github/fork-a-repo) and then [clone it](https://docs.github.com/en/github/creating-cloning-and-archiving-repositories/cloning-a-repository) on your workstation.

The directories in the cloned repo will be structured approximately like this:

<details>
  <summary>Click to expand directory structure</summary>
This in a non-exhaustive list of the directories in the OSM repo. It is provided
as a birds-eye view of where the different components are located.

- `charts/` - contains OSM Helm chart
- `ci/` - tools and scripts for the continuous integration system
- `cmd/` - OSM command line tools
- `crd/` - Custom Resource Definitions needed by OSM
- `demo/` - scripts and Kubernetes resources needed to run the Bookstore demonstration of Open Service Mesh
- `docs/` - OSM documentation
- `pkg/` -
  - `catalog/` - Mesh Catalog component is the central piece of OSM, which collects inputs from all other components and dispatches configuration to the proxy control plane
  - `certificate/` - contains multiple implementations of 1st and 3rd party certificate issuers, as well as PEM and x509 certificate management tools
    - `providers/` -
      - `keyvault/` - implements integration with Azure Key Vault
      - `vault/` - implements integration with Hashicorp Vault
      - `tresor/` - OSM native certificate issuer
  - `debugger/` - web server and tools used to debug the service mesh and the controller
  - `endpoint/` - Endpoints are components capable of introspecting the participating compute platforms; these retrieve the IP addresses of the compute backing the services in the mesh. This directory contains integrations with supported compute providers.
    - `providers/` -
      - `azure/` - integrates with Azure
      - `kube/` - Kubernetes tools and informers integrations
  - `envoy/` - packages needed to translate SMI into xDS
    - `ads/` - Aggregated Discovery Service related tools
    - `cds/` - Cluster Discovery Service related tools
    - `cla/` - Cluster Load Assignment components
    - `eds/` - Endpoint Discovery Service tools
    - `lds/` - Listener Discovery Service tools
    - `rds/` - Route Discovery Service tools
    - `sds/` - Secret Discovery service related tools
  - `health/` - OSM controller liveness and readiness probe handlers
  - `ingress/` - package mutating the service mesh in response to the application of an Ingress Kubernetes resource
  - `injector/` - sidecar injection webhook and related tools
  - `kubernetes/` - Kubernetes event handlers and helpers
  - `logger/` - logging facilities
  - `metricsstore/` - OSM controller system metrics tools
  - `namespace/` - package with tools handling a service mesh spanning multiple Kubernetes namespaces.
  - `service/` - tools needed for easier handling of Kubernetes services
  - `signals/` - operating system signal handlers
  - `smi/` - SMI client, informer, caches and tools
  - `tests/` - test fixtures and other functions to make unit testing easier
  - `trafficpolicy/` - SMI related types
- `wasm/` - Source for a WebAssembly-based Envoy extension
</details>

The Open Service Mesh controller is written in Go.
It relies on the [SMI Spec](https://github.com/servicemeshinterface/smi-spec/).
OSM leverages [Envoy proxy](https://github.com/envoyproxy/envoy) as a data plane and Envoy's [XDS v3](https://www.envoyproxy.io/docs/envoy/latest/api-v3/api) protocol, which is offered in Go by [go-control-plane](https://github.com/envoyproxy/go-control-plane).

## Get Go-ing

This Open Service Mesh project uses [Go v1.19.0+](https://golang.org/). If you are not familiar with Go, spend some time with the excellent [Tour of Go](https://tour.golang.org/).

## Get the dependencies

The OSM packages rely on many external Go libraries.

Take a peek at the `go.mod` file in the root of this repository to see all dependencies.

Run `go get -d ./...` to download all required Go packages.

Also the project requires Docker. See how to [install Docker](https://docs.docker.com/engine/install/).

#### Makefile

Many of the operations within the OSM repo have GNU Makefile targets.
More notable:

- `make docker-build` builds and pushes all Docker images
- `make go-test` to run unit tests
- `make go-test-coverage` - run unit tests and output unit test coverage
- `make go-lint` runs golangci-lint
- `make go-fmt` - same as `go fmt ./...`
- `make go-vet` - same as `go vet ./...`

## Create Environment Variables

The OSM demos and examples rely on environment variables to make it usable on your localhost. The root of the OSM repository contains a file named `.env.example`. Copy the contents of this file into `.env`

```bash
cat .env.example > .env
```

The various environment variables are documented in the `.env.example` file itself. Modify the variables in `.env` to suite your environment.

Some of the scripts and build targets available expect an accessible container registry where to push the `osm-controller` and `init` docker images once compiled. The location and credential options for the container registry can be specified as environment variables declared in `.env`, as well as the target namespace where `osm-controller` will be installed on.

Additionally, if using `demo/` scripts to deploy OSM's provided demo on your own K8s cluster, the same container registry configured in `.env` will be used to pull OSM images on your K8s cluster.

```console
# K8S_NAMESPACE is the Namespace the control plane will be installed into
export K8S_NAMESPACE=osm-system

# CTR_REGISTRY is the URL of the container registry to use
export CTR_REGISTRY=<your registry>

# If no authentication to push to the container registry is required, the following steps may be skipped.
# For Azure Container Registry (ACR), the following command may be used: az acr credential show -n <your_registry_name> --query "passwords[0].value" | tr -d '"'
export CTR_REGISTRY_PASSWORD=<your password>

# Create docker secret in Kubernetes Namespace using following script:
./scripts/create-container-registry-creds.sh "$K8S_NAMESPACE"

```

(NOTE: these requirements are true for automatic demo deployment using the available demo scripts; [#1416](https://github.com/openservicemesh/osm/issues/1416) tracks an improvement to not strictly require these and use upstream images from official dockerhub registry if a user does not want/need changes on the code)

## Build and push OSM images

For development and/or testing locally compiled builds, pushing the local image to a container registry is still required. Several Makefile targets are available.

### Examples

Build and push all images:

```console
make docker-build
```

Build and push all images to a specific registry with a specific tag:

```console
make docker-build CTR_REGISTRY=myregistry CTR_TAG=mytag
```

Build all images and load them into the current docker instance, but do not push:

```console
make docker-build DOCKER_BUILDX_OUTPUT=type=docker
```

Build and push only the osm-controller image. Similar targets exist for all OSM and demo images:

```console
make docker-build-osm-controller
```

Build and push a particular image for multiple architectures:

```console
make docker-build-osm-bootstrap DOCKER_BUILDX_PLATFORM=linux/amd64,linux/arm64
```

## Code Formatting

All Go source code is formatted with `goimports`. The version of `goimports`
used by this project is specified in `go.mod`. To ensure you have the same
version installed, run `go install -mod=readonly golang.org/x/tools/cmd/goimports`. It's recommended that you set your IDE or
other development tools to use `goimports`. Formatting is checked during CI by
the `bin/fmt` script.

## Putting it all together (inner development loop)

Now that you have an overview of the various parts of the project such as build commands, environment variables and linting, we will give you a example 
workflow for development.  Modify as need for your environment if using kind locally doesn't fit your use case.

```bash
# use default setup
cp .env.example .env
source .env

# create a local kind cluster
make kind-up

# build osm control plane components and cli (includes changes to helm chart)
make build-osm-all

# deploy osm
./bin/osm install --set=osm.image.registry="$CTR_REGISTRY" --set=osm.image.pullPolicy=Always --set=osm.controllerLogLevel=trace --verbose 
```

> **note**: you can set any of the OSM chart parameters in [values.yaml](/charts/osm/values.yaml) such as osm.image.registry on OSM install using `--set`. For example, if you need to customize the deployment tags you can use `--set=osm.image.registry="$CTR_TAG"`

### Making changes to OSM 
Make required changes to the OSM controller and deploy them.  You could do this for any server component, see more on the [Makefile](#makefile) for how to build individual containers.

```bash
# only builds controller (see below for all build commands)
make docker-build-osm-controller
kubectl rollout restart deployment osm-controller  
```

If you made changes to OSM cli or helm chart you might want to refresh your deployment:

```bash
# clean up and rebuild everything
bin/osm uninstall mesh -f --mesh-name "$MESH_NAME" --delete-namespace -a
make build-osm-all

# redeploy 
./bin/osm install --set=osm.image.registry="$CTR_REGISTRY" --set=osm.image.pullPolicy=Always --verbose 
```

### Using Tilt
[Tilt](https://docs.tilt.dev/index.html) can automatically build and deploy your changes as you save the file.  This is slightly different from the above option where you manually build the components and restart them, instead it precompiles and deploys the components on the fly. After installing [tilt](https://docs.tilt.dev/install.html) and [kind](https://kind.sigs.k8s.io/docs/user/quick-start/#installation) run:

```
make tilt-up
```

This will create a kind cluster and deploy all the components.  If you edit one of the files and save it to disk, tilt will recompile that component and deploy it to the running pod.  

> note: currently works for controller pods (controller, injector, bootstrap) but not the init container

Customizing the tilt deployments can be done via `tilt_config.json` file.  For instance you can configure helm values to customize OSM HELM chart deployment.

## Testing your changes

The OSM repo has a few layers of tests:

- Unit tests
- End-to-end (e2e) tests
- Simulations

For tests in the OSM repo we have chosen to leverage the following:

- [Go testing](https://golang.org/pkg/testing/) for unit tests
- [Gomega](https://onsi.github.io/gomega/) and [Ginkgo](https://onsi.github.io/ginkgo/) frameworks for e2e tests

We follow Go's convention and add unit tests for the respective functions in files with the `_test.go` suffix. So if a function lives in a file `foo.go` we will write a unit test for it in the file `foo_test.go`.

Refer to a [unit test](/pkg/catalog/inbound_traffic_policies_test.go) and [e2e test](/tests/e2e/e2e_egress_policy_test.go) example should you need a starting point.

#### Unit Tests

The most rudimentary tests are the unit tests. We strive for test coverage above 80% where
this is pragmatic and possible.
Each newly added function should be accompanied by a unit test. Ideally, while working
on the OSM repository, we practice
[test-driven development](https://en.wikipedia.org/wiki/Test-driven_development),
and each change would be accompanied by a unit test.

To run all unit tests you can use the following `Makefile` target:

```bash
make go-test-coverage
```

You can run the tests exclusively for the package you are working on. For example the following command will
run only the tests in the package implementing OSM's
[Hashicorp Vault](https://www.vaultproject.io/) integration:

```bash
go test ./pkg/certificate/providers/vault/...
```

You can check the unit test coverage by using the `-cover` option:

```bash
go test -cover ./pkg/certificate/providers/vault/...
```

We have a dedicated tool for in-depth analysis of the unit-test code coverage:

```bash
./scripts/test-w-coverage.sh
```

Running the [test-w-coverage.sh](/scripts/test-w-coverage.sh) script will create
an HTML file with in-depth analysis of unit-test coverage per package, per
function, and it will even show lines of code that need work. Open the HTML
file this tool generates to understand how to improve test coverage:

```
open ./coverage/index.html
```

Once the file loads in your browser, scroll to the package you worked on to see current test coverage:

![package coverage](https://docs.openservicemesh.io/docs/images/unit-test-coverage-1.png)

Our overall guiding principle is to maintain unit-test coverage at or above 80%.

To understand which particular functions need more testing - scroll further in the report:

![per function](https://docs.openservicemesh.io/docs/images/unit-test-coverage-2.png)

And if you are wondering why a function, which we have written a test for, is not 100% covered,
you will find the per-function analysis useful. This will show you code paths that are not tested.

![per function](https://docs.openservicemesh.io/docs/images/unit-test-coverage-3.png)

##### Mocking

OSM uses the [GoMock](https://github.com/golang/mock) mocking framework to mock interfaces in unit tests.
GoMock's `mockgen` tool is used to autogenerate mocks from interfaces.

As an example, to create a mock client for the `Configurator` interface defined in the [configurator](/pkg/configurator) package, add the corresponding mock generation rule in the [rules file](/mockspec/rules) and generate the mocks using the command `make check-mocks` from the root of the OSM repo.

_Note: Autogenerated mock file names must be prefixed with `mock_`and suffixed with`_generated` as seen above. These files are excluded from code coverage reports._

When a mocked interface is changed, the autogenerated mock code must be regenerated.
More details can be found in [GoMock's documentation](https://github.com/golang/mock/blob/master/README.md).

#### Integration Tests

Unit tests focus on a single function. These ensure that with a specific input, the function
in question produces expected output or side effect. Integration tests, on the other hand,
ensure that multiple functions work together correctly. Integration tests ensure your new
code composes with other existing pieces.

Take a look at [the following test](/pkg/configurator/client_test.go),
which tests the functionality of multiple functions together. In this particular example, the test:

- uses a mock Kubernetes client via `testclient.NewSimpleClientset()` from the `github.com/openservicemesh/osm/pkg/gen/client/config/clientset/versioned/fake` library
- [creates a MeshConfig](/pkg/configurator/client_test.go#L50)
- [tests whether](/pkg/configurator/client_test.go#L63-L69) the underlying functions compose correctly by fetching the results of the top-level function `IsEgressEnabled()`

### End-to-End (e2e) Tests

End-to-end tests verify the behavior of the entire system. For OSM, e2e tests will install a control plane, install test workloads and SMI policies, and check that the workload is behaving as expected.

OSM's e2e tests are located in tests/e2e. The tests can be run using the `test-e2e` Makefile target. The Makefile target will also build the necessary container images and `osm` CLI binary before running the tests. The tests are written using Ginkgo and Gomega so they may also be directly invoked using `go test`. Be sure to build the `osm-controller` and `init` container images and `osm` CLI before directly invoking the tests. With either option, it is suggested to explicitly set the container registry location and tag to ensure up-to-date images are used by setting the `CTR_REGISTRY` and `CTR_TAG` environment variables.

In addition to the flags provided by `go test` and Ginkgo, there are several custom command line flags that may be used for e2e tests to configure global parameters like container image locations and cleanup behavior. The full list of custom flags can be found in [tests/e2e/](/tests/e2e#flags).

For more information, please refer to [OSM's E2E Readme](/tests/e2e/README.md).

#### Simulation / Demo

When we want to ensure that the entire system works correctly over time and
transitions state as expected - we run
[the demo included in the docs](/demo/README.md).
This type of test is the slowest, but also most comprehensive. This test will ensure that your changes
work with a real Kubernetes cluster, with real SMI policy, and real functions - no mocked or fake Go objects.

#### Profiling

OSM control plane exposes an HTTP server able to serve a number of resources.

For mesh visibility and debugabbility, one can refer to the endpoints provided under [pkg/debugger](/pkg/debugger) which contains a number of endpoints able to inspect and list most of the common structures used by the control plane at runtime.

Additionally, the current implementation of the debugger imports and hooks [pprof endpoints](https://golang.org/pkg/net/http/pprof/).
Pprof is a golang package able to provide profiling information at runtime through HTTP protocol to a connecting client.

Debugging endpoints can be turned on or off through the runtime argument `enable-debug-server`, normally set on the deployment at install time through the CLI.

Example usage:

```
scripts/port-forward-osm-debug.sh &
go tool pprof http://localhost:9091/debug/pprof/heap
```

From pprof tool, it is possible to extract a large variety of profiling information, from heap and cpu profiling, to goroutine blocking, mutex profiling or execution tracing. We suggest to refer to the [pprof documentation](https://golang.org/pkg/net/http/pprof/) for more information.

## Helm charts

The Open Service Mesh control plane chart is located in the
[`charts/osm`](/charts/osm) folder.

The [`charts/osm/values.yaml`](/charts/osm/values.yaml) file defines the default value for properties
referenced by the different chart templates.

The [`charts/osm/templates/`](/charts/osm/templates) folder contains the chart templates
for the different Kubernetes resources that are deployed as a part of the Open Service control plane installation.
The different chart templates are used as follows:

- `osm-*.yaml` chart templates are directly consumed by the `osm-controller` service.
- `mutatingwebhook.yaml` is used to deploy a `MutatingWebhookConfiguration` kubernetes resource that enables automatic sidecar injection
- `grafana-*.yaml` chart templates are used to deploy a Grafana instance when grafana installation is enabled
- `prometheus-*.yaml` chart templates are used to deploy a Prometheus instance when prometheus installation is enabled
- `fluentbit-configmap.yaml` is used to provide configurations for the fluent bit sidecar and its plugins when fluent bit is enabled
- `jaeger-*.yaml` chart templates are used to deploy a Jaeger instance when Jaeger deployment and tracing are enabled

### Custom Resource Definitions

The [`charts/osm/crds/`](/charts/osm/crds/) folder contains the charts corresponding to the SMI CRDs.
Experimental CRDs can be found under [`charts/osm/crds/experimental/`](/charts/osm/crds/experimental).

### Updating Dependencies

Dependencies for the OSM chart are listed in Chart.yaml. To update a dependency,
modify its version as needed in Chart.yaml, run `helm dependency update`, then
commit all changes to Chart.yaml, Chart.lock, and the charts/osm/charts
directory which stores the source for the updated dependency chart.
