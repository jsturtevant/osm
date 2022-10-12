package certificate

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openservicemesh/osm/pkg/apis/config/v1alpha2"

	tassert "github.com/stretchr/testify/assert"
)

func TestGetNamespacedMRC(t *testing.T) {
	tests := []struct {
		name           string
		mrc            *v1alpha2.MeshRootCertificate
		expectedOutput string
	}{
		{
			name: "Valid MRC with namespace and name",
			mrc: &v1alpha2.MeshRootCertificate{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "Namespace",
					Name:      "Name",
				},
			},
			expectedOutput: "Namespace/Name",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert := tassert.New(t)

			namespacedMRC := getNamespacedMRC(tt.mrc)
			assert.Equal(tt.expectedOutput, namespacedMRC)

		})
	}
}

func TestValidateMRCIntents(t *testing.T) {
	tests := []struct {
		name   string
		mrc1   *v1alpha2.MeshRootCertificate
		mrc2   *v1alpha2.MeshRootCertificate
		result error
	}{
		{
			name:   "One valid MRC and one nil",
			mrc1:   &v1alpha2.MeshRootCertificate{},
			mrc2:   nil,
			result: ErrMRCNotFound,
		},
		{
			name: "Invalid mrc intent foo",
			mrc1: &v1alpha2.MeshRootCertificate{
				Spec: v1alpha2.MeshRootCertificateSpec{
					Intent: "foo",
				},
			},
			mrc2: &v1alpha2.MeshRootCertificate{
				Spec: v1alpha2.MeshRootCertificateSpec{
					Intent: v1alpha2.ActiveIntent,
				},
			},
			result: ErrInvalidMRCIntent,
		},
		{
			name: "Invalid mrc intent combination of active and active",
			mrc1: &v1alpha2.MeshRootCertificate{
				Spec: v1alpha2.MeshRootCertificateSpec{
					Intent: v1alpha2.ActiveIntent,
				},
			},
			mrc2: &v1alpha2.MeshRootCertificate{
				Spec: v1alpha2.MeshRootCertificateSpec{
					Intent: v1alpha2.ActiveIntent,
				},
			},
			result: ErrInvalidMRCIntentCombination,
		},
		{
			name: "Valid mrc intent combination of active and passive",
			mrc1: &v1alpha2.MeshRootCertificate{
				Spec: v1alpha2.MeshRootCertificateSpec{
					Intent: v1alpha2.ActiveIntent,
				},
			},
			mrc2: &v1alpha2.MeshRootCertificate{
				Spec: v1alpha2.MeshRootCertificateSpec{
					Intent: v1alpha2.PassiveIntent,
				},
			},
			result: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert := tassert.New(t)
			err := ValidateMRCIntents(tt.mrc1, tt.mrc2)
			assert.Equal(tt.result, err)

		})
	}
}

func TestHandleSingleMRC(t *testing.T) {
	tests := []struct {
		name   string
		mrc    *v1alpha2.MeshRootCertificate
		result error
	}{
		{
			name: "Single mrc is not active",
			mrc: &v1alpha2.MeshRootCertificate{
				Spec: v1alpha2.MeshRootCertificateSpec{
					Intent: v1alpha2.PassiveIntent,
				},
			},
			result: ErrInvalidMRCIntent,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert := tassert.New(t)

			m := &Manager{}
			err := m.handleSingleMRC(tt.mrc)
			assert.Equal(tt.result, err)
		})
	}
}

func TestSetIssuers(t *testing.T) {
	tests := []struct {
		name                       string
		mrc1                       *v1alpha2.MeshRootCertificate
		mrc2                       *v1alpha2.MeshRootCertificate
		result                     error
		expectedSigningIssuerID    string
		expectedValidatingIssuerID string
	}{
		{
			name:   "mrc is nil",
			mrc1:   nil,
			mrc2:   nil,
			result: ErrMRCNotFound,
		},
		{
			name: "mrc1 is active and mrc2 is passive",
			mrc1: &v1alpha2.MeshRootCertificate{
				ObjectMeta: metav1.ObjectMeta{
					Name: "mrc1",
				},
				Spec: v1alpha2.MeshRootCertificateSpec{
					Intent: v1alpha2.ActiveIntent,
				},
			},
			mrc2: &v1alpha2.MeshRootCertificate{
				ObjectMeta: metav1.ObjectMeta{
					Name: "mrc2",
				},
				Spec: v1alpha2.MeshRootCertificateSpec{
					Intent: v1alpha2.PassiveIntent,
				},
			},
			result:                     nil,
			expectedSigningIssuerID:    "mrc1",
			expectedValidatingIssuerID: "mrc2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert := tassert.New(t)

			m := &Manager{
				mrcClient: &fakeMRCClient{},
			}
			err := m.setIssuers(tt.mrc1, tt.mrc2)
			assert.Equal(tt.result, err)

			if err != nil {
				assert.Nil(m.signingIssuer)
				assert.Nil(m.validatingIssuer)
			} else {
				assert.Equal(tt.expectedSigningIssuerID, m.signingIssuer.ID)
				assert.Equal(tt.expectedValidatingIssuerID, m.validatingIssuer.ID)
			}
		})
	}
}
