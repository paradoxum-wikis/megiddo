package certmgr

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"

	"megiddo/internal/megiddo"
)

const (
	notBeforeSkew           = -5 * time.Minute
	defaultCAExpiry         = time.Hour * 24 * 365 * 10
	defaultLeafExpiry       = time.Hour * 24 * 825
	minCARemainingToReuse   = 30 * 24 * time.Hour
	minLeafRemainingToReuse = 7 * 24 * time.Hour
)

func serialNumber() *big.Int {
	buf := make([]byte, 8)
	if _, err := rand.Reader.Read(buf); err != nil {
		panic(err)
	}
	return new(big.Int).SetBytes(buf)
}

func writePEM(path, blockType string, der []byte) error {
	fd, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer fd.Close()
	return pem.Encode(fd, &pem.Block{Type: blockType, Bytes: der})
}

func writeKeyPEM(path string, key *rsa.PrivateKey) error {
	fd, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer fd.Close()
	der := x509.MarshalPKCS1PrivateKey(key)
	return pem.Encode(fd, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: der})
}

func parseCA(certPEM, keyPEM []byte) (*x509.Certificate, *rsa.PrivateKey, error) {
	cBlock, _ := pem.Decode(certPEM)
	if cBlock == nil || !strings.Contains(cBlock.Type, "CERTIFICATE") {
		return nil, nil, errors.New("ca cert PEM missing")
	}
	caCert, err := x509.ParseCertificate(cBlock.Bytes)
	if err != nil {
		return nil, nil, err
	}
	kBlock, _ := pem.Decode(keyPEM)
	if kBlock == nil {
		return nil, nil, errors.New("ca key PEM missing")
	}
	if pkcs1, err := x509.ParsePKCS1PrivateKey(kBlock.Bytes); err == nil {
		return caCert, pkcs1, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(kBlock.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("parse CA key: %w", err)
	}
	rsaKey, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, nil, errors.New("ca key is not RSA")
	}
	return caCert, rsaKey, nil
}

func leafValid(leafCertPath, leafKeyPath string, caCert *x509.Certificate, minLeft time.Duration) (bool, error) {
	cb, err := os.ReadFile(leafCertPath)
	if err != nil {
		return false, err
	}
	kb, err := os.ReadFile(leafKeyPath)
	if err != nil {
		return false, err
	}
	b, _ := pem.Decode(cb)
	if b == nil {
		return false, errors.New("leaf cert PEM")
	}
	leafCert, err := x509.ParseCertificate(b.Bytes)
	if err != nil {
		return false, err
	}
	pub, ok := leafCert.PublicKey.(*rsa.PublicKey)
	if !ok {
		return false, nil
	}
	kbDecoded, _ := pem.Decode(kb)
	if kbDecoded == nil {
		return false, errors.New("leaf key PEM")
	}
	leafKey, err := x509.ParsePKCS1PrivateKey(kbDecoded.Bytes)
	if err != nil {
		return false, err
	}
	if !leafCert.NotAfter.After(time.Now().UTC().Add(minLeft)) {
		return false, nil
	}
	if pub.N.Cmp(leafKey.PublicKey.N) != 0 || pub.E != leafKey.PublicKey.E {
		return false, nil
	}
	if leafCert.Issuer.String() != caCert.Subject.String() {
		return false, nil
	}
	if err := leafCert.CheckSignatureFrom(caCert); err != nil {
		return false, nil
	}
	return true, nil
}

func validPair(caCertPath, caKeyPath string, minLeft time.Duration) (bool, error) {
	cb, err := os.ReadFile(caCertPath)
	if err != nil {
		return false, err
	}
	kb, err := os.ReadFile(caKeyPath)
	if err != nil {
		return false, err
	}
	cert, key, err := parseCA(cb, kb)
	if err != nil {
		return false, err
	}
	if strings.ToLower(cert.Subject.CommonName) != strings.ToLower(megiddo.CanaryCommonName) {
		return false, nil
	}
	if !cert.NotAfter.After(time.Now().UTC().Add(minLeft)) {
		return false, nil
	}
	pub, ok := cert.PublicKey.(*rsa.PublicKey)
	if !ok {
		return false, nil
	}
	if pub.N.Cmp(key.PublicKey.N) != 0 || pub.E != key.PublicKey.E {
		return false, nil
	}
	return cert.CheckSignatureFrom(cert) == nil, nil
}

func EnsureCA(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("ca dir: %w", err)
	}
	caCertPath := filepath.Join(dir, "ca.crt")
	caKeyPath := filepath.Join(dir, "ca.key")
	if b, err := validPair(caCertPath, caKeyPath, minCARemainingToReuse); err == nil && b {
		return nil
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Add(notBeforeSkew)
	tmpl := &x509.Certificate{
		SerialNumber: serialNumber(),
		Subject: pkix.Name{
			CommonName:   megiddo.CanaryCommonName,
			Organization: []string{"Megiddo"},
		},
		NotBefore:             now,
		NotAfter:              now.Add(defaultCAExpiry),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLenZero:        true,
		SignatureAlgorithm:    x509.SHA256WithRSA,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return err
	}
	tmpCrt := caCertPath + ".tmp"
	tmpKey := caKeyPath + ".tmp"
	if err := writePEM(tmpCrt, "CERTIFICATE", der); err != nil {
		return err
	}
	if err := writeKeyPEM(tmpKey, key); err != nil {
		return err
	}
	if err := os.Rename(tmpCrt, caCertPath); err != nil {
		return err
	}
	return os.Rename(tmpKey, caKeyPath)
}

func leafFilenames(dir, host string) (certPath, keyPath string) {
	safe := strings.ReplaceAll(host, "*", "_wildcard_")
	base := filepath.Join(dir, safe)
	return base + ".crt", base + ".key"
}

func TLSPair(host, dir string) (*tls.Certificate, error) {
	if err := EnsureCA(dir); err != nil {
		return nil, err
	}
	caCertPath := filepath.Join(dir, "ca.crt")
	caKeyPath := filepath.Join(dir, "ca.key")
	caCertPEM, err := os.ReadFile(caCertPath)
	if err != nil {
		return nil, err
	}
	caKeyPEM, err := os.ReadFile(caKeyPath)
	if err != nil {
		return nil, err
	}
	caCert, caKey, err := parseCA(caCertPEM, caKeyPEM)
	if err != nil {
		return nil, err
	}
	lp, lk := leafFilenames(dir, host)
	if ok, err := leafValid(lp, lk, caCert, minLeafRemainingToReuse); err == nil && ok {
		tlsc, err := tls.LoadX509KeyPair(lp, lk)
		if err == nil {
			return &tlsc, nil
		}
	}
	leafKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC().Add(notBeforeSkew)
	tmpl := &x509.Certificate{
		SerialNumber: serialNumber(),
		Subject: pkix.Name{
			CommonName: host,
		},
		NotBefore:          now,
		NotAfter:           now.Add(defaultLeafExpiry),
		KeyUsage:           x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:        []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		SignatureAlgorithm: x509.SHA256WithRSA,
		DNSNames:           []string{host},
	}
	leafDer, err := x509.CreateCertificate(rand.Reader, tmpl, caCert, &leafKey.PublicKey, caKey)
	if err != nil {
		return nil, err
	}
	tmpCrt := lp + ".tmp"
	tmpKey := lk + ".tmp"
	if err := writePEM(tmpCrt, "CERTIFICATE", leafDer); err != nil {
		return nil, err
	}
	if err := writeKeyPEM(tmpKey, leafKey); err != nil {
		return nil, err
	}
	if err := os.Rename(tmpCrt, lp); err != nil {
		return nil, err
	}
	if err := os.Rename(tmpKey, lk); err != nil {
		return nil, err
	}
	tlsc, err := tls.LoadX509KeyPair(lp, lk)
	if err != nil {
		return nil, err
	}
	return &tlsc, nil
}

func ReadCAPEM(dir string) ([]byte, error) {
	path := filepath.Join(dir, "ca.crt")
	return os.ReadFile(path)
}
