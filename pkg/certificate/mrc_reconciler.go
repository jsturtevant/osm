package certificate

import (
	"fmt"

	"github.com/openservicemesh/osm/pkg/apis/config/v1alpha2"
	"github.com/openservicemesh/osm/pkg/errcode"
)

func (m *Manager) handleMRCEvent(event MRCEvent) error {
	mrcList, err := m.mrcClient.ListMeshRootCertificates()
	if err != nil {
		return err
	}
	if len(mrcList) == 0 {
		// TODO(ajellio)
		return fmt.Errorf("")
	}
	if len(mrcList) > 2 {
		// TODO(jaellio): create error and errcode for more than 2 mrcs
		return fmt.Errorf("More than 2 MRCs found in control plane namespace. Ignoring event.")
	}

	var mrc1, mrc2 *v1alpha2.MeshRootCertificate
	mrc1 = mrcList[0]
	if len(mrcList) == 1 {
		return m.handleSingleMRC(mrc1)
	}

	mrc2 = mrcList[1]
	log.Debug().Msgf("")
	if err = ValidateMRCIntents(mrc1, mrc2); err != nil {
		return err
	}

	if err = m.setIssuers(mrc1, mrc2); err != nil {
		// TODO(jaellio): set error on MRC?
		return err
	}
	return nil
}

func (m *Manager) handleSingleMRC(mrc *v1alpha2.MeshRootCertificate) error {
	if mrc.Spec.Intent != v1alpha2.ActiveIntent {
		return ErrInvalidMRCIntent
	}

	issuer, err := m.getCertIssuer(mrc)
	if err != nil {
		return err
	}

	return m.updateIssuers(issuer, issuer)
}

var validMRCIntentCombinations = map[v1alpha2.MeshRootCertificateIntent][]v1alpha2.MeshRootCertificateIntent{
	// TODO(jaellio): considered nested map. Intent slice is small so iterating over the slice is fine for now
	v1alpha2.ActiveIntent: []v1alpha2.MeshRootCertificateIntent{
		v1alpha2.PassiveIntent,
		v1alpha2.DeactiveIntent,
		v1alpha2.InactiveIntent,
	},
	v1alpha2.PassiveIntent: []v1alpha2.MeshRootCertificateIntent{
		v1alpha2.ActiveIntent,
		v1alpha2.DeactiveIntent,
	},
	v1alpha2.DeactiveIntent: []v1alpha2.MeshRootCertificateIntent{
		v1alpha2.ActiveIntent,
		v1alpha2.PassiveIntent,
	},
	v1alpha2.InactiveIntent: []v1alpha2.MeshRootCertificateIntent{
		v1alpha2.ActiveIntent,
	},
}

func ValidateMRCIntents(mrc1, mrc2 *v1alpha2.MeshRootCertificate) error {
	if mrc1 == nil || mrc2 == nil {
		log.Error().Err(ErrMRCNotFound).Msg("cannot validate nil mrc")
		return ErrMRCNotFound
	}

	intent1 := mrc1.Spec.Intent
	intent2 := mrc2.Spec.Intent

	validIntents, ok := validMRCIntentCombinations[intent1]
	if !ok {
		log.Error().Err(ErrInvalidMRCIntent).
			Msgf("unable to find %s intent in set of valid intents. Invalid combination of %s intent and %s intent", intent1, intent1, intent2)
		return ErrInvalidMRCIntent
	}

	for _, intent := range validIntents {
		if intent2 == intent {
			log.Debug().Msgf("valid intent combination")
			return nil
		}
	}

	log.Error().Err(ErrInvalidMRCIntentCombination).Str(errcode.Kind, errcode.GetErrCodeWithMetric(errcode.ErrInvalidMRCIntentCombination)).
		Msgf("invalid combination of %s intent and %s intent", intent1, intent2)
	return ErrInvalidMRCIntentCombination
}

// TODO(jeallio): simplify to different data structure rather than user switch case
// or pass intent directly to set issuers. It has already be
func (m *Manager) setIssuers(mrc1, mrc2 *v1alpha2.MeshRootCertificate) error {
	if mrc1 == nil || mrc2 == nil {
		log.Error().Err(ErrMRCNotFound).Msg("cannot validate nil mrc")
		return ErrMRCNotFound
	}

	issuer1, err := m.getCertIssuer(mrc1)
	if err != nil {
		return err
	}
	issuer2, err := m.getCertIssuer(mrc2)
	if err != nil {
		return err
	}

	intent1 := mrc1.Spec.Intent
	intent2 := mrc2.Spec.Intent

	var signingIssuer, validatingIssuer *issuer
	switch intent1 {
	case v1alpha2.ActiveIntent:
		switch intent2 {
		case v1alpha2.PassiveIntent:
			signingIssuer = issuer1
			validatingIssuer = issuer2
		case v1alpha2.InactiveIntent:
			signingIssuer = issuer1
			validatingIssuer = issuer1
		case v1alpha2.DeactiveIntent:
			signingIssuer = issuer2
			validatingIssuer = issuer1
		default:
			log.Error().Err(ErrInvalidMRCIntentCombination).Str(errcode.Kind, errcode.GetErrCodeWithMetric(errcode.ErrInvalidMRCIntentCombination)).
				Msgf("invalid combination of %s intent and %s intent", intent1, intent2)
			return ErrInvalidMRCIntentCombination
		}
	case v1alpha2.PassiveIntent:
		switch intent2 {
		case v1alpha2.ActiveIntent:
			signingIssuer = issuer2
			validatingIssuer = issuer1
		case v1alpha2.DeactiveIntent:
			signingIssuer = issuer1
			validatingIssuer = issuer2
		default:
			log.Error().Err(ErrInvalidMRCIntentCombination).Str(errcode.Kind, errcode.GetErrCodeWithMetric(errcode.ErrInvalidMRCIntentCombination)).
				Msgf("invalid combination of %s intent and %s intent", intent1, intent2)
			return ErrInvalidMRCIntentCombination
		}
	case v1alpha2.DeactiveIntent:
		switch intent2 {
		case v1alpha2.ActiveIntent, v1alpha2.PassiveIntent:
			signingIssuer = issuer2
			validatingIssuer = issuer1
		default:
			log.Error().Err(ErrInvalidMRCIntentCombination).Str(errcode.Kind, errcode.GetErrCodeWithMetric(errcode.ErrInvalidMRCIntentCombination)).
				Msgf("invalid combination of %s intent and %s intent", intent1, intent2)
			return ErrInvalidMRCIntentCombination
		}
	case v1alpha2.InactiveIntent:
		switch intent2 {
		case v1alpha2.ActiveIntent:
			signingIssuer = issuer2
			validatingIssuer = issuer2
		default:
			log.Error().Err(ErrInvalidMRCIntentCombination).Str(errcode.Kind, errcode.GetErrCodeWithMetric(errcode.ErrInvalidMRCIntentCombination)).
				Msgf("invalid combination of %s intent and %s intent", intent1, intent2)
			return ErrInvalidMRCIntentCombination
		}
	default:
		// TODO(jaellio): create errcode for ErrInvalidMRCIntent
		// log.Error().Err(ErrInvalidMRCIntent).Str(errcode.Kind, errcode.GetErrCodeWithMetric(errcode.ErrInvalidMRCIntent)).
		//		Msgf("invalid combination of %s intent and %s intent", intent1, intent2)
		return ErrInvalidMRCIntent
	}

	return m.updateIssuers(signingIssuer, validatingIssuer)
}

func (m *Manager) updateIssuers(signing, validating *issuer) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.signingIssuer = signing
	m.validatingIssuer = validating
	return nil
}

func (m *Manager) getCertIssuer(mrc *v1alpha2.MeshRootCertificate) (*issuer, error) {
	client, ca, err := m.mrcClient.GetCertIssuerForMRC(mrc)
	if err != nil {
		return nil, err
	}

	c := &issuer{Issuer: client, ID: mrc.Name, CertificateAuthority: ca, TrustDomain: mrc.Spec.TrustDomain, SpiffeEnabled: mrc.Spec.SpiffeEnabled}
	return c, nil
}

func getNamespacedMRC(mrc *v1alpha2.MeshRootCertificate) string {
	return fmt.Sprintf("%s/%s", mrc.Namespace, mrc.Name)
}
