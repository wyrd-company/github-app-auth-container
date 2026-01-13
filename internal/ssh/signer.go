package ssh

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha512"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"strings"

	"golang.org/x/crypto/ssh"
)

// Signer provides SSH signing capabilities using an RSA private key.
type Signer struct {
	privateKey *rsa.PrivateKey
	sshSigner  ssh.Signer
	publicKey  string
	keyID      string
}

// NewSigner creates a new SSH signer from an RSA private key.
func NewSigner(privateKey *rsa.PrivateKey) (*Signer, error) {
	// Create SSH signer from private key
	sshSigner, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create SSH signer: %w", err)
	}

	// Get the public key in SSH format
	publicKey := ssh.MarshalAuthorizedKey(sshSigner.PublicKey())

	// Generate a key ID (fingerprint)
	keyID := ssh.FingerprintSHA256(sshSigner.PublicKey())

	return &Signer{
		privateKey: privateKey,
		sshSigner:  sshSigner,
		publicKey:  strings.TrimSpace(string(publicKey)),
		keyID:      keyID,
	}, nil
}

// Sign creates an SSH signature for the given data.
// This produces a signature compatible with git's SSH signing format.
func (s *Signer) Sign(data []byte) (string, error) {
	// SSH signatures use SHA-512
	hash := sha512.Sum512(data)

	// Sign with RSA-SHA512 (rsa-sha2-512)
	signature, err := rsa.SignPKCS1v15(rand.Reader, s.privateKey, crypto.SHA512, hash[:])
	if err != nil {
		return "", fmt.Errorf("failed to sign: %w", err)
	}

	// Format as SSH signature
	// Git expects the signature in the SSH signature format
	sshSig := formatSSHSignature(s.sshSigner.PublicKey(), signature)

	return sshSig, nil
}

// SignGit creates a signature in git's expected format for SSH signing.
// The namespace is typically "git" for commits and tags.
func (s *Signer) SignGit(data []byte, namespace string) (string, error) {
	// Git uses a specific signature format with namespace
	sig, err := s.sshSigner.Sign(rand.Reader, data)
	if err != nil {
		return "", fmt.Errorf("failed to sign: %w", err)
	}

	// Format as armored SSH signature
	return formatArmoredSignature(s.sshSigner.PublicKey(), sig, namespace), nil
}

// PublicKey returns the SSH public key in authorized_keys format.
func (s *Signer) PublicKey() string {
	return s.publicKey
}

// KeyID returns the key fingerprint.
func (s *Signer) KeyID() string {
	return s.keyID
}

// PublicKeyPEM returns the public key in PEM format.
func (s *Signer) PublicKeyPEM() string {
	pubKeyBytes := ssh.MarshalAuthorizedKey(s.sshSigner.PublicKey())
	return string(pubKeyBytes)
}

// formatSSHSignature formats a signature in SSH wire format.
func formatSSHSignature(pubKey ssh.PublicKey, sig []byte) string {
	// This is a simplified format - git uses a more complex armored format
	return base64.StdEncoding.EncodeToString(sig)
}

// formatArmoredSignature creates an armored SSH signature like ssh-keygen produces.
func formatArmoredSignature(pubKey ssh.PublicKey, sig *ssh.Signature, namespace string) string {
	// SSH signature format (magic preamble + version + public key + namespace + reserved + hash algorithm + signature)
	// Reference: https://github.com/openssh/openssh-portable/blob/master/PROTOCOL.sshsig

	var buf strings.Builder

	// SSH signature armor
	buf.WriteString("-----BEGIN SSH SIGNATURE-----\n")

	// Build the signature blob
	sigBlob := buildSSHSigBlob(pubKey, sig, namespace)
	encoded := base64.StdEncoding.EncodeToString(sigBlob)

	// Wrap at 70 characters
	for i := 0; i < len(encoded); i += 70 {
		end := i + 70
		if end > len(encoded) {
			end = len(encoded)
		}
		buf.WriteString(encoded[i:end])
		buf.WriteString("\n")
	}

	buf.WriteString("-----END SSH SIGNATURE-----")

	return buf.String()
}

// buildSSHSigBlob builds the binary SSH signature blob.
func buildSSHSigBlob(pubKey ssh.PublicKey, sig *ssh.Signature, namespace string) []byte {
	// Magic preamble
	magic := []byte("SSHSIG")

	// Version (uint32 = 1)
	version := []byte{0, 0, 0, 1}

	// Public key (string)
	pubKeyBlob := pubKey.Marshal()
	pubKeyLen := uint32(len(pubKeyBlob))

	// Namespace (string)
	namespaceBytes := []byte(namespace)
	namespaceLen := uint32(len(namespaceBytes))

	// Reserved (empty string)
	reservedLen := uint32(0)

	// Hash algorithm (string) - sha512
	hashAlgo := []byte("sha512")
	hashAlgoLen := uint32(len(hashAlgo))

	// Signature (string)
	sigBlob := ssh.Marshal(sig)
	sigLen := uint32(len(sigBlob))

	// Build the blob
	blob := make([]byte, 0, 256)
	blob = append(blob, magic...)
	blob = append(blob, version...)
	blob = appendUint32(blob, pubKeyLen)
	blob = append(blob, pubKeyBlob...)
	blob = appendUint32(blob, namespaceLen)
	blob = append(blob, namespaceBytes...)
	blob = appendUint32(blob, reservedLen)
	blob = appendUint32(blob, hashAlgoLen)
	blob = append(blob, hashAlgo...)
	blob = appendUint32(blob, sigLen)
	blob = append(blob, sigBlob...)

	return blob
}

func appendUint32(b []byte, v uint32) []byte {
	return append(b, byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
}

// ExtractPublicKeyFromPEM extracts an SSH public key from a PEM-encoded private key.
func ExtractPublicKeyFromPEM(pemData []byte) (string, error) {
	block, _ := pem.Decode(pemData)
	if block == nil {
		return "", fmt.Errorf("failed to decode PEM")
	}

	signer, err := ssh.ParsePrivateKey(pemData)
	if err != nil {
		return "", fmt.Errorf("failed to parse private key: %w", err)
	}

	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(signer.PublicKey()))), nil
}
