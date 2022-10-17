package certificate

import (
	"fmt"

	"github.com/openservicemesh/osm/pkg/apis/config/v1alpha2"
	"github.com/openservicemesh/osm/pkg/errcode"
)

func (m *Manager) handleMRCEvent(event MRCEvent) error {
	log.Debug().Msgf("handling MRC event for MRC %s", event.MRCName)
	mrcList, err := m.mrcClient.ListMeshRootCertificates()
	if err != nil {
		return err
	}

	if len(mrcList) == 0 {
		msg := fmt.Sprintf("when handling MRC event for MRC %s, found no MRCs in OSM control plane namespace", event.MRCName)
		log.Error().Msg(msg)
		return fmt.Errorf(msg)
	}

	filteredMRCList := filterOutInactiveMRCs(mrcList)
	if len(filteredMRCList) > 2 {
		log.Error().Err(ErrNumMRCExceedsMaxSupported).Str(errcode.Kind, errcode.GetErrCodeWithMetric(errcode.ErrNumMRCExceedsMaxSupported)).
			Msgf("expected 2 or less MRCs in the OSM control plane namespace, found %d", len(mrcList))
		return ErrNumMRCExceedsMaxSupported
	}

	var mrc1, mrc2 *v1alpha2.MeshRootCertificate
	mrc1 = mrcList[0]
	if len(filteredMRCList) == 2 {
		mrc2 = mrcList[1]
	}

	log.Debug().Msg("validating MRC intent combination")
	if err = ValidateMRCIntents(mrc1, mrc2); err != nil {
		return err
	}

	if m.shouldSetIssuers(mrc1, mrc2) {
		return m.setIssuers(mrc1, mrc2)
	}

	return nil
}

var validMRCIntentCombinations = map[v1alpha2.MeshRootCertificateIntent][]v1alpha2.MeshRootCertificateIntent{
	v1alpha2.ActiveIntent: {
		v1alpha2.PassiveIntent,
		v1alpha2.ActiveIntent,
	},
	v1alpha2.PassiveIntent: {
		v1alpha2.ActiveIntent,
	},
}

// ValidateMRCIntents validates the intent combination of MRCs
func ValidateMRCIntents(mrc1, mrc2 *v1alpha2.MeshRootCertificate) error {
	if mrc1 == nil && mrc2 == nil {
		log.Error().Err(ErrUnexpectedNilMRC).Msg("unexpected nil MRC provided when validating MRC intents")
		return ErrUnexpectedNilMRC
	}
	if (mrc1 != nil && mrc2 == nil) || (mrc1 == nil && mrc2 != nil) {
		mrc := mrc1
		if mrc == nil {
			mrc = mrc2
		}

		if mrc.Spec.Intent != v1alpha2.ActiveIntent {
			log.Error().Err(ErrExpectedActiveMRC).Msgf("expected single MRC with %s intent, found %s", v1alpha2.ActiveIntent, mrc.Spec.Intent)
			return ErrExpectedActiveMRC
		}

		return nil
	}

	intent1 := mrc1.Spec.Intent
	intent2 := mrc2.Spec.Intent

	validIntents, ok := validMRCIntentCombinations[intent1]
	if !ok {
		log.Error().Err(ErrUnknownMRCIntent).Msgf("unable to find %s intent in set of valid intents. Invalid combination of %s intent and %s intent", intent1, intent1, intent2)
		return ErrUnknownMRCIntent
	}

	for _, intent := range validIntents {
		if intent2 == intent {
			log.Debug().Msgf("verified valid intent combination of %s intent and %s intent", intent1, intent2)
			return nil
		}
	}

	log.Error().Err(ErrInvalidMRCIntentCombination).Str(errcode.Kind, errcode.GetErrCodeWithMetric(errcode.ErrInvalidMRCIntentCombination)).
		Msgf("invalid combination of %s intent and %s intent", intent1, intent2)
	return ErrInvalidMRCIntentCombination
}

func (m *Manager) shouldSetIssuers(mrc1, mrc2 *v1alpha2.MeshRootCertificate) bool {
	if mrc1 == nil && mrc2 == nil {
		log.Error().Err(ErrUnexpectedNilMRC).Msg("unexpected nil MRC provided when validating MRC intents")
		return false
	}
	var signingIssuer, validatingIssuer *issuer
	// handle single MRC in control plane
	if (mrc1 != nil && mrc2 == nil) || (mrc1 == nil && mrc2 != nil) {
		mrc := mrc1
		if mrc == nil {
			mrc = mrc2
		}

		// single MRC must have an active intent
		if mrc.Spec.Intent != v1alpha2.ActiveIntent {
			log.Error().Err(ErrExpectedActiveMRC).Msgf("expected single MRC with %s intent, found %s", v1alpha2.ActiveIntent, mrc.Spec.Intent)
			return false
		}
		m.mu.Lock()
		signingIssuer = m.signingIssuer
		validatingIssuer = m.validatingIssuer
		m.mu.Unlock()

		if mrc.Name == signingIssuer.ID && mrc.Name == validatingIssuer.ID {
			log.Debug().Msgf("issuers already set to expected values. Should not update")
			return false
		}

		return true
	}

	intent1 := mrc1.Spec.Intent
	intent2 := mrc2.Spec.Intent
	m.mu.Lock()
	signingIssuer = m.signingIssuer
	validatingIssuer = m.validatingIssuer
	m.mu.Unlock()

	switch intent1 {
	case v1alpha2.ActiveIntent:
		switch intent2 {
		case v1alpha2.PassiveIntent:
			if mrc1.Name == signingIssuer.ID && mrc2.Name == validatingIssuer.ID {
				log.Debug().Msgf("issuers already set to expected values: validating[%s] and signing[%s]. Should not update", validatingIssuer.ID, signingIssuer.ID)
				return false
			}
			return true
		case v1alpha2.ActiveIntent:
			// When both MRCs have active intents, their state is non deterministic.
			// To avoid continuously resetting the issuers when both MRCs are active,
			// accept either of the following cases. Only update the issuers if the
			// issuers are the same (signingIssuer == validatingIssuer).
			if (mrc1.Name == signingIssuer.ID && mrc2.Name == validatingIssuer.ID) ||
				(mrc1.Name == validatingIssuer.ID && mrc2.Name == signingIssuer.ID) {
				log.Debug().Msgf("issuers already set to expected values: validating[%s] and signing[%s]. Should not update", validatingIssuer.ID, signingIssuer.ID)
				return false
			}
			return true
		default:
			log.Error().Err(ErrInvalidMRCIntentCombination).Str(errcode.Kind, errcode.GetErrCodeWithMetric(errcode.ErrInvalidMRCIntentCombination)).
				Msgf("invalid combination of %s intent and %s intent", intent1, intent2)
			return false
		}
	case v1alpha2.PassiveIntent:
		switch intent2 {
		case v1alpha2.ActiveIntent:
			if mrc1.Name == validatingIssuer.ID && mrc2.Name == signingIssuer.ID {
				log.Debug().Msgf("issuers already set to expected values: validating[%s] and signing[%s]. Should not update", validatingIssuer.ID, signingIssuer.ID)
				return false
			}
			return true
		default:
			log.Error().Err(ErrInvalidMRCIntentCombination).Str(errcode.Kind, errcode.GetErrCodeWithMetric(errcode.ErrInvalidMRCIntentCombination)).
				Msgf("invalid combination of %s intent and %s intent", intent1, intent2)
			return false
		}
	default:
		log.Error().Err(ErrUnknownMRCIntent).Msgf("invalid combination of %s intent and %s intent", intent1, intent2)
		return false
	}
}

func (m *Manager) setIssuers(mrc1, mrc2 *v1alpha2.MeshRootCertificate) error {
	if mrc1 == nil && mrc2 == nil {
		log.Error().Err(ErrUnexpectedNilMRC).Msg("unexpected nil MRC provided when validating MRC intents")
		return ErrUnexpectedNilMRC
	}
	// handle single MRC in control plane
	if (mrc1 != nil && mrc2 == nil) || (mrc1 == nil && mrc2 != nil) {
		mrc := mrc1
		if mrc == nil {
			mrc = mrc2
		}

		issuer, err := m.getCertIssuer(mrc)
		if err != nil {
			return err
		}

		return m.updateIssuers(issuer, issuer)
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
		case v1alpha2.ActiveIntent:
			signingIssuer = issuer1
			validatingIssuer = issuer2
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
		default:
			log.Error().Err(ErrInvalidMRCIntentCombination).Str(errcode.Kind, errcode.GetErrCodeWithMetric(errcode.ErrInvalidMRCIntentCombination)).
				Msgf("invalid combination of %s intent and %s intent", intent1, intent2)
			return ErrInvalidMRCIntentCombination
		}
	default:
		// TODO(jaellio): create errcode for ErrUnknownMRCIntent
		log.Error().Err(ErrUnknownMRCIntent).Msgf("invalid combination of %s intent and %s intent", intent1, intent2)
		return ErrUnknownMRCIntent
	}

	return m.updateIssuers(signingIssuer, validatingIssuer)
}

func (m *Manager) updateIssuers(signing, validating *issuer) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.signingIssuer = signing
	m.validatingIssuer = validating
	log.Trace().Msgf("setting issuers for validating[%s] and signing[%s]", validating.ID, signing.ID)
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

func filterOutInactiveMRCs(mrcList []*v1alpha2.MeshRootCertificate) []*v1alpha2.MeshRootCertificate {
	n := 0
	for _, mrc := range mrcList {
		if mrc.Spec.Intent != v1alpha2.InactiveIntent {
			mrcList[n] = mrc
			n++
		}
	}
	return mrcList[:n]
}
