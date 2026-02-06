package ubt

import "github.com/jam-duna/jamduna/storage"

type (
	Hasher     = storage.Hasher
	Profile    = storage.Profile
	Stem       = storage.Stem
	TreeKey    = storage.TreeKey
	Config     = storage.Config
	KeyValue   = storage.KeyValue
	Proof      = storage.Proof
	MultiProof = storage.MultiProof
)

const (
	EIPProfile = storage.EIPProfile
	JAMProfile = storage.JAMProfile
)

var (
	ErrKeyNotFound  = storage.ErrKeyNotFound
	ErrInvalidProof = storage.ErrInvalidProof
)

func NewBlake3Hasher(profile Profile) *storage.Blake3Hasher {
	return storage.NewBlake3Hasher(profile)
}

func NewUnifiedBinaryTree(cfg Config) *storage.UnifiedBinaryTree {
	return storage.NewUnifiedBinaryTree(cfg)
}

func TreeKeyFromBytes(b [32]byte) TreeKey {
	return storage.TreeKeyFromBytes(b)
}

func VerifyMultiProof(mp *MultiProof, hasher Hasher, expectedRoot [32]byte) error {
	return storage.VerifyMultiProof(mp, hasher, expectedRoot)
}
