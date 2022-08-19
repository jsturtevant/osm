package tresor

import (
	tassert "github.com/stretchr/testify/assert"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/openservicemesh/osm/pkg/certificate"
	"github.com/spiffe/go-spiffe/v2/svid/x509svid"
)

const (
	rootCertPem = "sample_certificate.pem"
	rootKeyPem  = "sample_private_key.pem"
)

var _ = Describe("Test Certificate Manager", func() {
	defer GinkgoRecover()

	const serviceFQDN = "a.b.c"

	Context("Test issuing a certificate from a newly created CA", func() {
		validity := 3 * time.Second
		cn := certificate.CommonName("Test CA")
		rootCertCountry := "US"
		rootCertLocality := "CA"
		rootCertOrganization := testCertOrgName

		rootCert, err := NewCA(cn, 1*time.Hour, rootCertCountry, rootCertLocality, rootCertOrganization)
		if err != nil {
			GinkgoT().Fatalf("Error loading CA from files %s and %s: %s", rootCertPem, rootKeyPem, err.Error())
		}
		m, newCertError := New(rootCert, "org", 2048, false)
		It("should issue a certificate", func() {
			Expect(newCertError).ToNot(HaveOccurred())
			cert, issueCertificateError := m.IssueCertificate(serviceFQDN, validity)
			Expect(issueCertificateError).ToNot(HaveOccurred())
			Expect(cert.GetCommonName()).To(Equal(certificate.CommonName(serviceFQDN)))

			x509Cert, err := certificate.DecodePEMCertificate(rootCert.GetCertificateChain())
			Expect(err).ToNot(HaveOccurred())

			issuingCAPEM, err := certificate.EncodeCertDERtoPEM(x509Cert.Raw)
			Expect(err).ToNot(HaveOccurred())
			Expect([]byte(cert.GetIssuingCA())).To(Equal([]byte(issuingCAPEM)))

			pemCert := cert.GetCertificateChain()
			xCert, err := certificate.DecodePEMCertificate(pemCert)
			Expect(err).ToNot(HaveOccurred())
			Expect(xCert.Subject.CommonName).To(Equal(serviceFQDN))

			pemRootCert := cert.GetIssuingCA()
			xRootCert, err := certificate.DecodePEMCertificate(pemRootCert)
			Expect(err).ToNot(HaveOccurred(), string(pemRootCert))
			Expect(xRootCert.Subject.CommonName).To(Equal(cn.String()))
		})
	})

	Context("Test nil certificate issue", func() {
		m, newCertError := New(nil, "org", 2048, false)
		It("should return nil and error of no certificate", func() {
			Expect(m).To(BeNil())
			Expect(newCertError).To(Equal(errNoIssuingCA))
		})
	})

	Context("Test issuing a certificate when a root certificate is empty", func() {
		validity := 1 * time.Hour

		m := &CertManager{}
		It("should return errNoIssuingCA error", func() {
			cert, issueCertificateError := m.IssueCertificate(serviceFQDN, validity)
			Expect(cert).To(BeNil())
			Expect(issueCertificateError).To(Equal(errNoIssuingCA))
		})
	})

	Context("Test issuing a certificate when spiffe", func() {

		It("should return errNoIssuingCA error", func() {

		})
	})
})

func TestCertRotation(t *testing.T) {
	testCases := []struct {
		name          string
		cn            string
		expected      string
		errorExpected bool
	}{
		{
			name:     "standard common name",
			cn:       "sa.ns.test.com",
			expected: "spiffe://test.com/sa/ns",
		},
		{
			name:     "common name with subdomain",
			cn:       "sa.ns.subdomain.test.com",
			expected: "spiffe://subdomain.test.com/sa/ns",
		},
		{
			name:     "common name with subdomain",
			cn:       "sa.ns.long.er.subdomain.test.com",
			expected: "spiffe://long.er.subdomain.test.com/sa/ns",
		},
		{
			name:          "no trust domain name",
			cn:            "sa.ns",
			errorExpected: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert := tassert.New(t)

			validity := 3 * time.Second
			cn := certificate.CommonName("Test CA")
			rootCertCountry := "US"
			rootCertLocality := "CA"
			rootCertOrganization := testCertOrgName

			rootCert, err := NewCA(cn, 1*time.Hour, rootCertCountry, rootCertLocality, rootCertOrganization)
			assert.NoError(err)

			// Create with SpiffeCompate enabled
			m, err := New(rootCert, "org", 2048, true)
			assert.NoError(err)

			cert, err := m.IssueCertificate(certificate.CommonName(tc.cn), validity)
			if tc.errorExpected {
				assert.Error(err)
				assert.Nil(cert)
				return
			}

			assert.NoError(err)
			assert.NotNil(cert)

			//rootx509, err := certificate.DecodePEMCertificate(rootCert.IssuingCA)
			//
			//bundle := []*x509.Certificate{rootx509}

			svid, err := x509svid.Parse(cert.CertChain, cert.PrivateKey)
			assert.NoError(err)
			assert.Equal(tc.expected, svid.ID.String())
		})
	}
}
