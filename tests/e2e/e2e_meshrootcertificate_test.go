package e2e

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	cmapi "github.com/jetstack/cert-manager/pkg/apis/certmanager/v1"
	cmmeta "github.com/jetstack/cert-manager/pkg/apis/meta/v1"
	certman "github.com/jetstack/cert-manager/pkg/client/clientset/versioned"

	"github.com/openservicemesh/osm/pkg/apis/config/v1alpha2"
	"github.com/openservicemesh/osm/pkg/constants"
	"github.com/openservicemesh/osm/tests/framework"
	. "github.com/openservicemesh/osm/tests/framework"
)

var _ = OSMDescribe("MeshRootCertificate",
	OSMDescribeInfo{
		Tier:   2,
		Bucket: 11,
	},
	func() {
		Context("with Tressor", func() {
			It("rotates certificates", func() {
				basicCertRotationScenario()
			})
		})

		Context("with CertManager", func() {
			It("rotates certificates", func() {
				basicCertRotationScenario(WithCertManagerEnabled())
			})
		})

		Context("with Vault", func() {
			It("rotates certificates", func() {
				basicCertRotationScenario(WithVault())
			})
		})
	})

func basicCertRotationScenario(installOptions ...InstallOsmOpt) {
	var (
		clientNamespace     = framework.RandomNameWithPrefix("client")
		serverNamespace     = framework.RandomNameWithPrefix("server")
		clientContainerName = framework.RandomNameWithPrefix("container")
		ns                  = []string{clientNamespace, serverNamespace}
	)

	By("installing with MRC enabled")
	installOptions = append(installOptions, WithMeshRootCertificateEnabled())
	installOpts := Td.GetOSMInstallOpts(installOptions...)
	Expect(Td.InstallOSM(installOpts)).To(Succeed())

	// no secrets are created in Vault case
	if installOpts.CertManager != Vault {
		By("checking the certificate exists")
		err := Td.WaitForCABundleSecret(Td.OsmNamespace, OsmCABundleName, time.Second*5)
		Expect(err).NotTo(HaveOccurred())
	}

	By("checking HTTP traffic for client -> server pod after initial MRC creation")
	// Create namespaces
	for _, n := range ns {
		Expect(Td.CreateNs(n, nil)).To(Succeed())
		Expect(Td.AddNsToMesh(true, n)).To(Succeed())
	}

	// Get simple pod definitions for the HTTP server
	destinationPort := fortioHTTPPort
	serverSvcAccDef, serverPodDef, serverSvcDef, err := Td.SimplePodApp(
		SimplePodAppDef{
			PodName:   framework.RandomNameWithPrefix("pod"),
			Namespace: serverNamespace,
			Image:     fortioImageName,
			Ports:     []int{destinationPort},
			OS:        Td.ClusterOS,
		})
	Expect(err).NotTo(HaveOccurred())

	_, err = Td.CreateServiceAccount(serverNamespace, &serverSvcAccDef)
	Expect(err).NotTo(HaveOccurred())
	dstPod, err := Td.CreatePod(serverNamespace, serverPodDef)
	Expect(err).NotTo(HaveOccurred())
	_, err = Td.CreateService(serverNamespace, serverSvcDef)
	Expect(err).NotTo(HaveOccurred())

	// Expect it to be up and running in it's receiver namespace
	Expect(Td.WaitForPodsRunningReady(serverNamespace, 60*time.Second, 1, nil)).To(Succeed())

	// Get simple Pod definitions for the client
	clientSvcAccDef, clientPodDef, clientSvcDef, err := Td.SimplePodApp(SimplePodAppDef{
		PodName:       framework.RandomNameWithPrefix("pod"),
		Namespace:     clientNamespace,
		ContainerName: clientContainerName,
		Image:         fortioImageName,
		Ports:         []int{destinationPort},
		OS:            Td.ClusterOS,
	})
	Expect(err).NotTo(HaveOccurred())

	_, err = Td.CreateServiceAccount(clientNamespace, &clientSvcAccDef)
	Expect(err).NotTo(HaveOccurred())
	srcPod, err := Td.CreatePod(clientNamespace, clientPodDef)
	Expect(err).NotTo(HaveOccurred())
	_, err = Td.CreateService(clientNamespace, clientSvcDef)
	Expect(err).NotTo(HaveOccurred())

	// Expect it to be up and running in it's receiver namespace
	Expect(Td.WaitForPodsRunningReady(clientNamespace, 60*time.Second, 1, nil)).To(Succeed())

	// Deploy allow rule client->server
	httpRG, trafficTarget := Td.CreateSimpleAllowPolicy(
		SimpleAllowPolicy{
			RouteGroupName:    "routes",
			TrafficTargetName: "target",

			SourceNamespace:      clientNamespace,
			SourceSVCAccountName: clientSvcAccDef.Name,

			DestinationNamespace:      serverNamespace,
			DestinationSvcAccountName: serverSvcAccDef.Name,
		})

	// Configs have to be put into a monitored NS, and osm-system can't be by cli
	_, err = Td.CreateHTTPRouteGroup(serverNamespace, httpRG)
	Expect(err).NotTo(HaveOccurred())
	_, err = Td.CreateTrafficTarget(serverNamespace, trafficTarget)
	Expect(err).NotTo(HaveOccurred())

	// All ready. Expect client to reach server
	// Need to get the pod though.
	verifySuccessfulPodConnection(srcPod, dstPod, serverSvcDef, clientContainerName, destinationPort)

	By("checking that another cert with active intent cannot be created")
	time.Sleep(time.Second * 10)
	activeNotAllowed := "not-allowed"
	_, err = createMeshRootCertificate(activeNotAllowed, v1alpha2.ActiveIntent, installOpts.CertManager)
	Expect(err).Should(HaveOccurred())
	Expect(err.Error()).Should(ContainSubstring("cannot create MRC %s/%s with intent active. An MRC with active intent already exists in the control plane namespace", Td.OsmNamespace, activeNotAllowed))

	By("creating a second certificate with passive intent")
	newCertName := "osm-mrc-2"
	_, err = createMeshRootCertificate(newCertName, v1alpha2.PassiveIntent, installOpts.CertManager)
	Expect(err).NotTo(HaveOccurred())

	// no secrets are created in Vault case
	if installOpts.CertManager != Vault {
		By("ensuring the new CA secret exists")
		err = Td.WaitForCABundleSecret(Td.OsmNamespace, newCertName, time.Second*90)
		Expect(err).NotTo(HaveOccurred())
	}

	By("checking bootstrap secrets are updated after creating MRC with passive intent")

	podSelector := constants.EnvoyUniqueIDLabelName

	srvPod, err := Td.Client.CoreV1().Pods(serverPodDef.Namespace).Get(context.Background(), serverPodDef.Name, metav1.GetOptions{})
	Expect(err).To(BeNil())

	clientPod, err := Td.Client.CoreV1().Pods(clientPodDef.Namespace).Get(context.Background(), clientPodDef.Name, metav1.GetOptions{})
	Expect(err).To(BeNil())

	srvPodUUID := srvPod.GetLabels()[podSelector]
	clientPodUUID := clientPod.GetLabels()[podSelector]

	srvSecretName := fmt.Sprintf("envoy-bootstrap-config-%s", srvPodUUID)
	clientSecretName := fmt.Sprintf("envoy-bootstrap-config-%s", clientPodUUID)

	// TODO(jaellio): add a time.wait instead of waiting so long in call
	err = Td.WaitForBootstrapSecretUpdate(serverPodDef.Namespace, srvSecretName, "osm-mesh-root-certificate", newCertName, time.Second*30)
	Expect(err).NotTo(HaveOccurred())
	err = Td.WaitForBootstrapSecretUpdate(clientPodDef.Namespace, clientSecretName, "osm-mesh-root-certificate", newCertName, time.Second*30)
	Expect(err).NotTo(HaveOccurred())

	// TODO(#4835) add checks for the correct statuses for the two certificates and complete cert rotation
	verifySuccessfulPodConnection(srcPod, dstPod, serverSvcDef, clientContainerName, destinationPort)
	// TODO(jeallio)

	// 1. Verify service certs were updated?
	// 2. Verify validation contexts were updated
	// 3. Verify webhooks were updated
	// 5. verify xds cert updated (not sure how?)

	// Update oms-mesh-root-certificate from active to deactive intent

	// 1. Verify service certs were updated?
	// 2. Verify validation contexts were updated
	// 3. Verify webhooks were updated
	// 4. Verify bootstrap certs updated
	// 5. verify xds cert updated (not sure how?)
	// 5. Verify traffic

	// Update osm-mrc-2 from passive to active

	// 1. Verify service certs were updated?
	// 2. Verify validation contexts were updated
	// 3. Verify webhooks were updated
	// 4. Verify bootstrap certs updated
	// 5. verify xds cert updated (not sure how?)
	// 5. Verify traffic

	// Update osm-mesh-root-certificate from deactive to inactive

	// 1. Verify service certs were updated?
	// 2. Verify validation contexts were updated
	// 3. Verify webhooks were updated
	// 4. Verify bootstrap certs updated
	// 5. verify xds cert updated (not sure how?)
	// 5. Verify traffic
}

func createMeshRootCertificate(name string, intent v1alpha2.MeshRootCertificateIntent, certificateManagerType string) (*v1alpha2.MeshRootCertificate, error) {
	switch certificateManagerType {
	case DefaultCertManager:
		return createTressorMRC(name, intent)
	case CertManager:
		return createCertManagerMRC(name, intent)
	case Vault:
		return createVaultMRC(name, intent)
	default:
		Fail("should not be able to create MRC of unknown type")
		return nil, fmt.Errorf("should not be able to create MRC of unknown type")
	}
}

func createTressorMRC(name string, intent v1alpha2.MeshRootCertificateIntent) (*v1alpha2.MeshRootCertificate, error) {
	return Td.ConfigClient.ConfigV1alpha2().MeshRootCertificates(Td.OsmNamespace).Create(
		context.Background(), &v1alpha2.MeshRootCertificate{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: Td.OsmNamespace,
			},
			Spec: v1alpha2.MeshRootCertificateSpec{
				TrustDomain: "cluster.local",
				Intent:      intent,
				Provider: v1alpha2.ProviderSpec{
					Tresor: &v1alpha2.TresorProviderSpec{
						CA: v1alpha2.TresorCASpec{
							SecretRef: v1.SecretReference{
								Name:      name,
								Namespace: Td.OsmNamespace,
							},
						}},
				},
			},
		}, metav1.CreateOptions{})
}

func createCertManagerMRC(name string, intent v1alpha2.MeshRootCertificateIntent) (*v1alpha2.MeshRootCertificate, error) {
	cert := &cmapi.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: cmapi.CertificateSpec{
			IsCA:       true,
			Duration:   &metav1.Duration{Duration: 90 * 24 * time.Hour},
			SecretName: name,
			CommonName: "osm-system",
			IssuerRef: cmmeta.ObjectReference{
				Name:  "selfsigned",
				Kind:  "Issuer",
				Group: "cert-manager.io",
			},
		},
	}

	ca := &cmapi.Issuer{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: cmapi.IssuerSpec{
			IssuerConfig: cmapi.IssuerConfig{
				CA: &cmapi.CAIssuer{
					SecretName: name,
				},
			},
		},
	}

	cmClient, err := certman.NewForConfig(Td.RestConfig)
	Expect(err).NotTo(HaveOccurred())

	_, err = cmClient.CertmanagerV1().Certificates(Td.OsmNamespace).Create(context.TODO(), cert, metav1.CreateOptions{})
	Expect(err).NotTo(HaveOccurred())

	_, err = cmClient.CertmanagerV1().Issuers(Td.OsmNamespace).Create(context.TODO(), ca, metav1.CreateOptions{})
	Expect(err).NotTo(HaveOccurred())

	return Td.ConfigClient.ConfigV1alpha2().MeshRootCertificates(Td.OsmNamespace).Create(
		context.Background(), &v1alpha2.MeshRootCertificate{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: Td.OsmNamespace,
			},
			Spec: v1alpha2.MeshRootCertificateSpec{
				TrustDomain: "cluster.local",
				Intent:      intent,
				Provider: v1alpha2.ProviderSpec{
					CertManager: &v1alpha2.CertManagerProviderSpec{
						IssuerName:  name,
						IssuerKind:  "Issuer",
						IssuerGroup: "cert-manager.io",
					},
				},
			},
		}, metav1.CreateOptions{})
}

func createVaultMRC(name string, intent v1alpha2.MeshRootCertificateIntent) (*v1alpha2.MeshRootCertificate, error) {
	vaultPod, err := Td.GetPodsForLabel(Td.OsmNamespace, metav1.LabelSelector{
		MatchLabels: map[string]string{
			constants.AppLabel: "vault",
		},
	})
	Expect(err).NotTo(HaveOccurred())
	Expect(len(vaultPod)).Should(Equal(1))

	command := []string{"vault", "write", "pki/root/rotate/internal", "common_name=osm.root", fmt.Sprintf("issuer_name=%s", name)}
	stdout, stderr, err := Td.RunRemote(Td.OsmNamespace, vaultPod[0].Name, "vault", command)
	Td.T.Logf("Vault create new root output: %s, stderr:%s", stdout, stderr)
	Expect(err).NotTo(HaveOccurred())

	command = []string{"vault", "write", fmt.Sprintf("pki/roles/%s", name), "allow_any_name=true", "allow_subdomains=true", "max_ttl=87700h", "allowed_uri_sans=spiffe://*"}
	stdout, stderr, err = Td.RunRemote(Td.OsmNamespace, vaultPod[0].Name, "vault", command)
	Td.T.Logf("Vault create new role output: %s, stderr:%s", stdout, stderr)
	Expect(err).NotTo(HaveOccurred())

	return Td.ConfigClient.ConfigV1alpha2().MeshRootCertificates(Td.OsmNamespace).Create(
		context.Background(), &v1alpha2.MeshRootCertificate{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: Td.OsmNamespace,
			},
			Spec: v1alpha2.MeshRootCertificateSpec{
				TrustDomain: "cluster.local",
				Intent:      intent,
				Provider: v1alpha2.ProviderSpec{
					Vault: &v1alpha2.VaultProviderSpec{
						Host:     "vault." + Td.OsmNamespace + ".svc.cluster.local",
						Protocol: "http",
						Port:     8200,
						Role:     name,
						Token: v1alpha2.VaultTokenSpec{
							SecretKeyRef: v1alpha2.SecretKeyReferenceSpec{
								Name:      "osm-vault-token",
								Namespace: Td.OsmNamespace,
								Key:       "notused",
							}, // The test framework wires up the using default token right now so this isn't actually used
						},
					},
				},
			},
		}, metav1.CreateOptions{})
}

/*func verifiyUpdatedPodCert(pod *v1.Pod) {
	By("Verifying pod has updated certificates")

	// It can take a moment for envoy to load the certs
	Eventually(func() (string, error) {
		args := []string{"proxy", "get", "certs", pod.Name, fmt.Sprintf("-n=%s", pod.Namespace)}
		stdout, _, err := Td.RunLocal(filepath.FromSlash("../../bin/osm"), args...)
		Td.T.Logf("stdout:\n%s", stdout)
		return stdout.String(), err
	}, 10*time.Second).Should(ContainSubstring(fmt.Sprintf("\"uri\": \"spiffe://cluster.local/%s/%s", pod.Spec.ServiceAccountName, pod.Namespace)))
}*/

func verifySuccessfulPodConnection(srcPod, dstPod *v1.Pod, serverSvc v1.Service, clientContainerName string, destinationPort int) {
	By("Waiting for repeated request success")

	for i := 0; i < 2; i++ {
		cond := Td.WaitForRepeatedSuccess(func() bool {
			result :=
				Td.FortioHTTPLoadTest(FortioHTTPLoadTestDef{
					HTTPRequestDef: HTTPRequestDef{
						SourceNs:        srcPod.Namespace,
						SourcePod:       srcPod.Name,
						SourceContainer: clientContainerName,

						Destination: fmt.Sprintf("%s.%s:%d", serverSvc.Name, dstPod.Namespace, destinationPort),
					},
				})

			if result.Err != nil || result.HasFailedHTTPRequests() {
				Td.T.Logf("> REST req has failed requests: %v", result.Err)
				return false
			}
			Td.T.Logf("> REST req succeeded. Status codes: %v", result.AllReturnCodes())
			return true
		}, 5 /*consecutive success threshold*/, 90*time.Second /*timeout*/)
		Expect(cond).To(BeTrue())
		time.Sleep(time.Second * 6) // 6 seconds guarantee the certs are rotated.
	}
}
