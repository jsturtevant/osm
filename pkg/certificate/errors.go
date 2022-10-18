package certificate

import (
	"errors"
)

var errEncodeKey = errors.New("encode key")
var errEncodeCert = errors.New("encode cert")
var errMarshalPrivateKey = errors.New("marshal private key")
var errNoPrivateKeyInPEM = errors.New("no private Key in PEM")

// ErrNoCertificateInPEM is the error for no certificate in PEM
var ErrNoCertificateInPEM = errors.New("no certificate in PEM")

// ErrInvalidMRCIntentCombination is the error that should be returned if the combination of MRC intents is invalid.
var ErrInvalidMRCIntentCombination = errors.New("invalid mrc intent combination")

// ErrNumMRCExceedsMaxSupported is the error that should be returned if there are more than 2 MRCs with active
// and/or passive intent in the mesh.
var ErrNumMRCExceedsMaxSupported = errors.New("found more than the max number of MRCs supported in the control plane namespace")

// ErrExpectedActiveMRC is the error that should be returned when there is only 1 MRC in the mesh and it does not
// have an active intent.
var ErrExpectedActiveMRC = errors.New("found single MRC with non active intent")

// ErrUnknownMRCIntent is the error that should be returned if the intent value is not passive, active, or inactive.
var ErrUnknownMRCIntent = errors.New("found single MRC with non active intent")

// ErrMRCNotFound is the the error that should be returned if the expected MRC is not found or is nil.
var ErrMRCNotFound = errors.New("MRC not found")

// All of the below errors should be returned by the StorageEngine for each described scenario. The errors may be
// wrapped

// ErrInvalidCertSecret is the error that should be returned if the secret is stored incorrectly in the underlying infra
var ErrInvalidCertSecret = errors.New("invalid secret for certificate")

// ErrSecretNotFound should be returned if the secret isn't present in the underlying infra, on a Get
var ErrSecretNotFound = errors.New("secret not found")
