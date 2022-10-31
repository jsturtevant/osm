package certificate

import (
	"github.com/openservicemesh/osm/pkg/apis/config/v1alpha2"
	"github.com/openservicemesh/osm/pkg/errcode"
)

func (m *Manager) handleMRCEvent(event MRCEvent) error {
	log.Debug().Msgf("handling MRC event for MRC %s", event.MRCName)
	// TODO(#5226): optimize event handling to reduce cost of listing all MRCs for each event
	mrcList, err := m.mrcClient.ListMeshRootCertificates()
	if err != nil {
		return err
	}

	filteredMRCList := filterOutInactiveMRCs(mrcList)

	desiredSigningMRC, desiredValidatingMRC, err := getSigningAndValidatingMRCs(filteredMRCList)
	if err != nil {
		return err
	}

	shouldUpdate, err := m.shouldUpdateIssuers(desiredSigningMRC, desiredValidatingMRC)
	if err != nil {
		return err
	}
	if !shouldUpdate {
		return nil
	}

	desiredSigningIssuer, desiredValidatingIssuer, err := m.getCertIssuers(desiredSigningMRC, desiredValidatingMRC)
	if err != nil {
		return err
	}

	return m.updateIssuers(desiredSigningIssuer, desiredValidatingIssuer)
}

// getSigningAndValidatingMRCs returns the signing and validating MRCs from a list of MRCs
func getSigningAndValidatingMRCs(mrcList []*v1alpha2.MeshRootCertificate) (*v1alpha2.MeshRootCertificate, *v1alpha2.MeshRootCertificate, error) {
	if len(mrcList) == 0 {
		log.Error().Err(ErrNoMRCsFound).Msg("when handling MRC event, found no MRCs in OSM control plane namespace")
		return nil, nil, ErrNoMRCsFound
	}
	if len(mrcList) > 2 {
		log.Error().Err(ErrNumMRCExceedsMaxSupported).Str(errcode.Kind, errcode.GetErrCodeWithMetric(errcode.ErrNumMRCExceedsMaxSupported)).
			Msgf("expected 2 or less MRCs in the OSM control plane namespace, found %d", len(mrcList))
		return nil, nil, ErrNumMRCExceedsMaxSupported
	}

	var mrc1, mrc2 *v1alpha2.MeshRootCertificate
	mrc1 = mrcList[0]
	if len(mrcList) == 2 {
		mrc2 = mrcList[1]
	} else {
		log.Trace().Msgf("found single MRC in the mesh when handling MRC event for MRC %s", mrc1.Name)
		// if there is only one MRC, set mrc2 equal to mrc1
		mrc2 = mrc1
	}

	if mrc1 == nil || mrc2 == nil {
		log.Error().Err(ErrUnexpectedNilMRC).Msg("unexpected nil MRC provided when validating MRC intents")
		return nil, nil, ErrUnexpectedNilMRC
	}

	intent1 := mrc1.Spec.Intent
	intent2 := mrc2.Spec.Intent

	log.Debug().Msgf("validating intent combination of %s and %s", intent1, intent2)
	if mrc1 == mrc2 {
		// if the MRCs are equal then there is only 1 MRC in the mesh
		// and it must have an active intent
		if intent1 == v1alpha2.ActiveIntent {
			// since there is only 1 MRC, it is the signing and validating MRC
			return mrc1, mrc2, nil
		}

		log.Error().Err(ErrExpectedActiveMRC).Msgf("expected single MRC with %s intent, found %s", v1alpha2.ActiveIntent, mrc1.Spec.Intent)
		return nil, nil, ErrExpectedActiveMRC
	}

	// the combination of active and passive intents is deterministic
	// regardless of MRC ordering, the passive MRC is the validating MRC and
	// the active MRC is the signing MRC
	// the combination of active and active intents is non-deterministic
	// depending on the MRC ordering, either MRC could be the validating or
	// signing MRC
	if intent1 == v1alpha2.ActiveIntent && (intent2 == v1alpha2.PassiveIntent || intent2 == v1alpha2.ActiveIntent) {
		return mrc1, mrc2, nil
	}
	if intent1 == v1alpha2.PassiveIntent && intent2 == v1alpha2.ActiveIntent {
		return mrc2, mrc1, nil
	}

	log.Error().Err(ErrInvalidMRCIntentCombination).Str(errcode.Kind, errcode.GetErrCodeWithMetric(errcode.ErrInvalidMRCIntentCombination)).
		Msgf("invalid intent combination of %s and %s", intent1, intent2)
	return nil, nil, ErrInvalidMRCIntentCombination
}

func (m *Manager) shouldUpdateIssuers(desiredSigningMRC, desiredValidatingMRC *v1alpha2.MeshRootCertificate) (bool, error) {
	m.mu.Lock()
	signingIssuer := m.signingIssuer
	validatingIssuer := m.validatingIssuer
	m.mu.Unlock()

	if signingIssuer == nil || validatingIssuer == nil {
		return true, nil
	}

	// no update required if the issuers are already set to the desired value
	if signingIssuer.ID == desiredSigningMRC.Name && validatingIssuer.ID == desiredValidatingMRC.Name {
		log.Debug().Msgf("issuers already set to the desired value. Will not update issuers: validating[%s] and signing[%s]", validatingIssuer.ID, signingIssuer.ID)
		return false, nil
	}

	// if desiredSigningMRC != desiredValidatingMRC and both MRCs have active intents, their state is non
	// deterministic. No update required if the current signing and validating issuers correspond to the
	// existing MRCs. This check is necessary to avoid continuously resetting the issuers on start up
	if desiredSigningMRC != desiredValidatingMRC && desiredSigningMRC.Spec.Intent == v1alpha2.ActiveIntent &&
		desiredValidatingMRC.Spec.Intent == v1alpha2.ActiveIntent &&
		(desiredSigningMRC.Name == signingIssuer.ID || desiredSigningMRC.Name == validatingIssuer.ID) &&
		(desiredValidatingMRC.Name == signingIssuer.ID || desiredValidatingMRC.Name == validatingIssuer.ID) {
		log.Debug().Msgf("Will not update issuers to avoid repeated updates; validating[%s] and signing[%s]", validatingIssuer.ID, signingIssuer.ID)
		return false, nil
	}

	return true, nil
}

func (m *Manager) updateIssuers(signingIssuer, validatingIssuer *issuer) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.signingIssuer = signingIssuer
	m.validatingIssuer = validatingIssuer
	log.Trace().Msgf("setting issuers for validating[%s] and signing[%s]", validatingIssuer.ID, signingIssuer.ID)
	return nil
}

func (m *Manager) getCertIssuers(desiredSigningMRC, desiredValidatingMRC *v1alpha2.MeshRootCertificate) (*issuer, *issuer, error) {
	var desiredSigningIssuer, desiredValidatingIssuer *issuer
	desiredSigningIssuer, err := m.getCertIssuer(desiredSigningMRC)
	if err != nil {
		return nil, nil, err
	}
	// don't get the issuer again if there is a single MRC in the control plane
	if desiredSigningMRC == desiredValidatingMRC {
		desiredValidatingIssuer = desiredSigningIssuer
	} else {
		desiredValidatingIssuer, err = m.getCertIssuer(desiredValidatingMRC)
		if err != nil {
			return nil, nil, err
		}
	}

	return desiredSigningIssuer, desiredValidatingIssuer, nil
}

func (m *Manager) getCertIssuer(mrc *v1alpha2.MeshRootCertificate) (*issuer, error) {
	m.mu.Lock()
	signingIssuer := m.signingIssuer
	validatingIssuer := m.validatingIssuer
	m.mu.Unlock()

	// if the issuer has already been created for the specified MRC,
	// return the existing issuer
	if signingIssuer != nil && mrc.Name == signingIssuer.ID {
		return signingIssuer, nil
	}
	if validatingIssuer != nil && mrc.Name == validatingIssuer.ID {
		return validatingIssuer, nil
	}

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
