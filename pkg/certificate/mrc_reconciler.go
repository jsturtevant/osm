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

	err = validateMRCIntents(mrcList)
	if err != nil {
		// TODO(jaellio): set error on MRC?
		return err
	}

	err = m.setIssuers(mrcList)
	if err != nil {
		// TODO(jaellio): set error on MRC?
		return err
	}
	return nil
}

func validateMRCIntents(mrcList []*v1alpha2.MeshRootCertificate) error {
	// TODO(jaellio): check is list is empty, return error?
	mrc1Name := mrcList[0].Name
	mrc1Intent := mrcList[0].Spec.Intent
	if len(mrcList) == 1 {
		if mrc1Intent != "Active" {
			return fmt.Errorf("")
		}
		return nil
	}

	mrc2Name := mrcList[1].Name
	mrc2Intent := mrcList[1].Spec.Intent

	log.Info().Msgf("validating intent %s for MRC %s and intent %s for MRC %s", mrc1Intent, mrc1Name, mrc2Intent, mrc2Name)
	switch mrc1Intent {
	case "Active":
		switch mrc2Intent {
		case "Passive", "Inactive", "Deactive":
			return nil
		default:
			return ErrInvalidMRCIntentCombination
		}
	case "Passive":
		switch mrc2Intent {
		case "Active", "Deactive":
			return nil
		default:
			return ErrInvalidMRCIntentCombination
		}
	case "Deactive":
		switch mrc2Intent {
		case "Active", "Passive":
			return nil
		default:
			return ErrInvalidMRCIntentCombination
		}
	case "Inactive":
		switch mrc2Intent {
		case "Active":
			return nil
		default:
			return ErrInvalidMRCIntentCombination
		}
	default:
		return ErrInvalidMRCIntent
	}

	log.Error().Err(ErrInvalidMRCIntentCombination).Str(errcode.Kind, errcode.GetErrCodeWithMetric(errcode.ErrInvalidMRCIntentCombination)).
		Msgf("mrc %s with intent %s and mrc % with intent %s", mrc1Name, mrc1Intent, mrc2Name, mrc2Intent)
	return nil
}

// TODO(jeallio): simplify to different data structure rather than user switch case
func (m *Manager) setIssuers(mrcList []*v1alpha2.MeshRootCertificate) error {
	mrc1 := mrcList[0]
	mrc1Intent := mrc1.Spec.Intent
	var mrc1Issuer, mrc2Issuer *issuer

	if len(mrcList) == 1 {
		if mrc1Intent != "Active" {
			return fmt.Errorf("")
		}
		mrc1Issuer, err := m.getCertIssuer(mrc1)
		if err != nil {
			return err
		}
		return m.updateIssuers(mrc1Issuer, nil)
	}

	mrc2 := mrcList[1]
	mrc2Intent := mrc2.Spec.Intent
	mrc1Issuer, err := m.getCertIssuer(mrc2)
	if err != nil {
		return err
	}
	// TODO(jaellio): create errors
	switch mrc1Intent {
	case "Active":
		switch mrc2Intent {
		case "Passive":
			return m.updateIssuers(mrc1Issuer, mrc2Issuer)
		case "Inactive":
			return m.updateIssuers(mrc1Issuer, mrc1Issuer)
		case "Deactive":
			return m.updateIssuers(mrc2Issuer, mrc1Issuer)
		default:
			return fmt.Errorf("I")
		}
	case "Passive":
		switch mrc2Intent {
		case "Active":
			return m.updateIssuers(mrc2Issuer, mrc1Issuer)
		case "Deactive":
			return m.updateIssuers(mrc1Issuer, mrc2Issuer)
		default:
			return fmt.Errorf("%s", mrc2Intent)
		}
	case "Deactive":
		switch mrc2Intent {
		case "Active":
			return m.updateIssuers(mrc2Issuer, mrc1Issuer)
		case "Passive":
			return m.updateIssuers(mrc2Issuer, mrc1Issuer)
		default:
			return fmt.Errorf("%s", mrc2Intent)
		}
	case "Inactive":
		switch mrc2Intent {
		case "Active":
			return m.updateIssuers(mrc2Issuer, mrc2Issuer)
		default:
			return fmt.Errorf("%s", mrc2Intent)
		}
	default:
		return fmt.Errorf("%s", mrc2Intent)
	}

	return nil
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
