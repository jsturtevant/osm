package certificate

import (
	"testing"

	tassert "github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openservicemesh/osm/pkg/apis/config/v1alpha2"
)

func TestValidateMRCIntents(t *testing.T) {
	tests := []struct {
		name   string
		mrc1   *v1alpha2.MeshRootCertificate
		mrc2   *v1alpha2.MeshRootCertificate
		result error
	}{
		{
			name:   "Two nil mrcs",
			mrc1:   nil,
			mrc2:   nil,
			result: ErrUnexpectedNilMRC,
		},
		{
			name: "Single invalid mrc intent passive",
			mrc1: &v1alpha2.MeshRootCertificate{
				Spec: v1alpha2.MeshRootCertificateSpec{
					Intent: v1alpha2.PassiveIntent,
				},
			},
			mrc2:   nil,
			result: ErrExpectedActiveMRC,
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
			result: ErrUnknownMRCIntent,
		},
		{
			name: "Invalid mrc intent combination of passive and passive",
			mrc1: &v1alpha2.MeshRootCertificate{
				Spec: v1alpha2.MeshRootCertificateSpec{
					Intent: v1alpha2.PassiveIntent,
				},
			},
			mrc2: &v1alpha2.MeshRootCertificate{
				Spec: v1alpha2.MeshRootCertificateSpec{
					Intent: v1alpha2.PassiveIntent,
				},
			},
			result: ErrInvalidMRCIntentCombination,
		},
		{
			name: "Valid mrc intent combination of active and active",
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
			result: nil,
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
		{
			name: "Single valid mrc intent active",
			mrc1: nil,
			mrc2: &v1alpha2.MeshRootCertificate{
				Spec: v1alpha2.MeshRootCertificateSpec{
					Intent: v1alpha2.ActiveIntent,
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

func TestShouldSetIssuers(t *testing.T) {
	tests := []struct {
		name                      string
		mrc1                      *v1alpha2.MeshRootCertificate
		mrc2                      *v1alpha2.MeshRootCertificate
		result                    bool
		currentSigningIssuerID    string
		currentValidatingIssuerID string
	}{
		{
			name:   "mrcs are nil",
			mrc1:   nil,
			mrc2:   nil,
			result: false,
		},
		{
			name: "mrc1 is active and issuers already set",
			mrc1: &v1alpha2.MeshRootCertificate{
				ObjectMeta: metav1.ObjectMeta{
					Name: "mrc1",
				},
				Spec: v1alpha2.MeshRootCertificateSpec{
					Intent: v1alpha2.ActiveIntent,
				},
			},
			mrc2:                      nil,
			result:                    false,
			currentSigningIssuerID:    "mrc1",
			currentValidatingIssuerID: "mrc1",
		},
		{
			name: "mrc1 is active and not set",
			mrc1: &v1alpha2.MeshRootCertificate{
				ObjectMeta: metav1.ObjectMeta{
					Name: "mrc1",
				},
				Spec: v1alpha2.MeshRootCertificateSpec{
					Intent: v1alpha2.ActiveIntent,
				},
			},
			mrc2:                      nil,
			result:                    true,
			currentSigningIssuerID:    "mrc",
			currentValidatingIssuerID: "mrc",
		},
		{
			name: "mrc2 is active and issuers already set",
			mrc1: nil,
			mrc2: &v1alpha2.MeshRootCertificate{
				ObjectMeta: metav1.ObjectMeta{
					Name: "mrc2",
				},
				Spec: v1alpha2.MeshRootCertificateSpec{
					Intent: v1alpha2.ActiveIntent,
				},
			},
			result:                    false,
			currentSigningIssuerID:    "mrc2",
			currentValidatingIssuerID: "mrc2",
		},
		{
			name: "mrc1 is active and mrc2 is passive and issuers not set",
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
			result:                    true,
			currentSigningIssuerID:    "mrc1",
			currentValidatingIssuerID: "mrc1",
		},
		{
			name: "mrc1 is active and mrc2 is passive and issuers already set",
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
			result:                    false,
			currentSigningIssuerID:    "mrc1",
			currentValidatingIssuerID: "mrc2",
		},
		{
			name: "mrc1 is active and mrc2 is active and issuers are set (mrc1.Name == signingIssuer.ID and mrc2.Name == validatingIssuer.ID)",
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
					Intent: v1alpha2.ActiveIntent,
				},
			},
			result:                    false,
			currentSigningIssuerID:    "mrc1",
			currentValidatingIssuerID: "mrc2",
		},
		{
			name: "mrc1 is active and mrc2 is active and issuers are set (mrc1.Name == validatingIssuer.ID and mrc2.Name == signingIssuer.ID)",
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
					Intent: v1alpha2.ActiveIntent,
				},
			},
			result:                    false,
			currentSigningIssuerID:    "mrc2",
			currentValidatingIssuerID: "mrc1",
		},
		{
			name: "mrc1 is passive and mrc2 is active and issuers already set",
			mrc1: &v1alpha2.MeshRootCertificate{
				ObjectMeta: metav1.ObjectMeta{
					Name: "mrc1",
				},
				Spec: v1alpha2.MeshRootCertificateSpec{
					Intent: v1alpha2.PassiveIntent,
				},
			},
			mrc2: &v1alpha2.MeshRootCertificate{
				ObjectMeta: metav1.ObjectMeta{
					Name: "mrc2",
				},
				Spec: v1alpha2.MeshRootCertificateSpec{
					Intent: v1alpha2.ActiveIntent,
				},
			},
			result:                    false,
			currentSigningIssuerID:    "mrc2",
			currentValidatingIssuerID: "mrc1",
		},
		{
			name: "mrc1 is passive and mrc2 is active and issuers not set",
			mrc1: &v1alpha2.MeshRootCertificate{
				ObjectMeta: metav1.ObjectMeta{
					Name: "mrc1",
				},
				Spec: v1alpha2.MeshRootCertificateSpec{
					Intent: v1alpha2.PassiveIntent,
				},
			},
			mrc2: &v1alpha2.MeshRootCertificate{
				ObjectMeta: metav1.ObjectMeta{
					Name: "mrc2",
				},
				Spec: v1alpha2.MeshRootCertificateSpec{
					Intent: v1alpha2.ActiveIntent,
				},
			},
			result:                    true,
			currentSigningIssuerID:    "mrc1",
			currentValidatingIssuerID: "mrc2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert := tassert.New(t)

			m := &Manager{
				signingIssuer:    &issuer{ID: tt.currentSigningIssuerID},
				validatingIssuer: &issuer{ID: tt.currentValidatingIssuerID},
			}
			err := m.shouldSetIssuers(tt.mrc1, tt.mrc2)
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
			name:   "mrcs are nil",
			mrc1:   nil,
			mrc2:   nil,
			result: ErrUnexpectedNilMRC,
		},
		{
			name: "mrc1 is active",
			mrc1: &v1alpha2.MeshRootCertificate{
				ObjectMeta: metav1.ObjectMeta{
					Name: "mrc1",
				},
				Spec: v1alpha2.MeshRootCertificateSpec{
					Intent: v1alpha2.ActiveIntent,
				},
			},
			mrc2:                       nil,
			result:                     nil,
			expectedSigningIssuerID:    "mrc1",
			expectedValidatingIssuerID: "mrc1",
		},
		{
			name: "mrc2 is active",
			mrc1: nil,
			mrc2: &v1alpha2.MeshRootCertificate{
				ObjectMeta: metav1.ObjectMeta{
					Name: "mrc2",
				},
				Spec: v1alpha2.MeshRootCertificateSpec{
					Intent: v1alpha2.ActiveIntent,
				},
			},
			result:                     nil,
			expectedSigningIssuerID:    "mrc2",
			expectedValidatingIssuerID: "mrc2",
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
		{
			name: "mrc1 is active and mrc2 is active",
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
					Intent: v1alpha2.ActiveIntent,
				},
			},
			result:                     nil,
			expectedSigningIssuerID:    "mrc1",
			expectedValidatingIssuerID: "mrc2",
		},
		{
			name: "mrc1 is passive and mrc2 is active",
			mrc1: &v1alpha2.MeshRootCertificate{
				ObjectMeta: metav1.ObjectMeta{
					Name: "mrc1",
				},
				Spec: v1alpha2.MeshRootCertificateSpec{
					Intent: v1alpha2.PassiveIntent,
				},
			},
			mrc2: &v1alpha2.MeshRootCertificate{
				ObjectMeta: metav1.ObjectMeta{
					Name: "mrc2",
				},
				Spec: v1alpha2.MeshRootCertificateSpec{
					Intent: v1alpha2.ActiveIntent,
				},
			},
			result:                     nil,
			expectedSigningIssuerID:    "mrc2",
			expectedValidatingIssuerID: "mrc1",
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

func TestFilterOutInactiveMRCs(t *testing.T) {
	tests := []struct {
		name            string
		mrcList         []*v1alpha2.MeshRootCertificate
		expectedMRCList []*v1alpha2.MeshRootCertificate
	}{
		{
			name:            "empty mrc list",
			mrcList:         []*v1alpha2.MeshRootCertificate{},
			expectedMRCList: []*v1alpha2.MeshRootCertificate{},
		},
		{
			name: "mrc list with only inactive mrcs",
			mrcList: []*v1alpha2.MeshRootCertificate{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "mrc2",
					},
					Spec: v1alpha2.MeshRootCertificateSpec{
						Intent: v1alpha2.InactiveIntent,
					},
				},
			},
			expectedMRCList: []*v1alpha2.MeshRootCertificate{},
		},
		{
			name: "mrc list with no inactive mrcs",
			mrcList: []*v1alpha2.MeshRootCertificate{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "mrc1",
					},
					Spec: v1alpha2.MeshRootCertificateSpec{
						Intent: v1alpha2.ActiveIntent,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "mrc2",
					},
					Spec: v1alpha2.MeshRootCertificateSpec{
						Intent: v1alpha2.PassiveIntent,
					},
				},
			},
			expectedMRCList: []*v1alpha2.MeshRootCertificate{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "mrc1",
					},
					Spec: v1alpha2.MeshRootCertificateSpec{
						Intent: v1alpha2.ActiveIntent,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "mrc2",
					},
					Spec: v1alpha2.MeshRootCertificateSpec{
						Intent: v1alpha2.PassiveIntent,
					},
				},
			},
		},
		{
			name: "mrc list with no inactive mrcs",
			mrcList: []*v1alpha2.MeshRootCertificate{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "mrc1",
					},
					Spec: v1alpha2.MeshRootCertificateSpec{
						Intent: v1alpha2.ActiveIntent,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "mrc2",
					},
					Spec: v1alpha2.MeshRootCertificateSpec{
						Intent: v1alpha2.InactiveIntent,
					},
				},
			},
			expectedMRCList: []*v1alpha2.MeshRootCertificate{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "mrc1",
					},
					Spec: v1alpha2.MeshRootCertificateSpec{
						Intent: v1alpha2.ActiveIntent,
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert := tassert.New(t)

			returnedMRCList := filterOutInactiveMRCs(tt.mrcList)
			assert.ElementsMatch(tt.expectedMRCList, returnedMRCList)
		})
	}
}
