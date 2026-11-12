// Package apk reads the package name and signing certificates of an APK
// without Android SDK tools (replaces aapt and apksigner).
package apk

import (
	"archive/zip"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"strings"

	"github.com/avast/apkparser"
	"github.com/avast/apkverifier"
)

// manifestPackage captures the package attribute of <manifest>.
type manifestPackage struct{ name string }

func (m *manifestPackage) EncodeToken(t xml.Token) error {
	if se, ok := t.(xml.StartElement); ok && se.Name.Local == "manifest" && m.name == "" {
		for _, a := range se.Attr {
			if a.Name.Local == "package" {
				m.name = a.Value
			}
		}
	}
	return nil
}

func (m *manifestPackage) Flush() error { return nil }

// PackageName returns the package name declared in the APK manifest.
func PackageName(path string) (string, error) {
	var m manifestPackage
	zipErr, _, manifestErr := apkparser.ParseApk(path, &m)
	if zipErr != nil {
		return "", zipErr
	}
	if m.name == "" {
		if manifestErr != nil {
			return "", manifestErr
		}
		return "", errors.New("package name not found in AndroidManifest.xml")
	}
	return m.name, nil
}

// Signer is one signing certificate with apksigner-style digests.
type Signer struct {
	DN                string
	SHA256, SHA1, MD5 string
}

// Signers returns the signer certificates (first certificate of each signer).
func Signers(path string) ([]Signer, error) {
	chains, err := apkverifier.ExtractCerts(path, nil)
	if err != nil {
		return nil, err
	}
	var signers []Signer
	for _, chain := range chains {
		if len(chain) == 0 {
			continue
		}
		signers = append(signers, newSigner(chain[0]))
	}
	if len(signers) == 0 {
		return nil, errors.New("no signer certificates found")
	}
	return signers, nil
}

func newSigner(c *x509.Certificate) Signer {
	s256 := sha256.Sum256(c.Raw)
	s1 := sha1.Sum(c.Raw)
	m5 := md5.Sum(c.Raw)
	return Signer{
		DN:     distinguishedName(c.RawSubject),
		SHA256: hex.EncodeToString(s256[:]),
		SHA1:   hex.EncodeToString(s1[:]),
		MD5:    hex.EncodeToString(m5[:]),
	}
}

// Attribute names as printed by Java's X500Principal.
var attrNames = map[string]string{
	"2.5.4.3":                    "CN",
	"2.5.4.11":                   "OU",
	"2.5.4.10":                   "O",
	"2.5.4.7":                    "L",
	"2.5.4.8":                    "ST",
	"2.5.4.6":                    "C",
	"2.5.4.9":                    "STREET",
	"2.5.4.5":                    "SERIALNUMBER",
	"1.2.840.113549.1.9.1":       "EMAILADDRESS",
	"0.9.2342.19200300.100.1.25": "DC",
	"0.9.2342.19200300.100.1.1":  "UID",
}

// distinguishedName formats the raw subject like Java's X500Principal
// ("CN=Android Debug, O=Android, C=US"): most specific RDN first, ", " separated,
// values with special characters quoted.
func distinguishedName(rawSubject []byte) string {
	var seq pkix.RDNSequence
	if _, err := asn1.Unmarshal(rawSubject, &seq); err != nil {
		return ""
	}
	parts := make([]string, 0, len(seq))
	for i := len(seq) - 1; i >= 0; i-- {
		var atvs []string
		for _, atv := range seq[i] {
			name, ok := attrNames[atv.Type.String()]
			if !ok {
				name = "OID." + atv.Type.String()
			}
			atvs = append(atvs, name+"="+dnValue(fmt.Sprint(atv.Value)))
		}
		parts = append(parts, strings.Join(atvs, " + "))
	}
	return strings.Join(parts, ", ")
}

// dnValue quotes values containing DN special characters, as Java does.
func dnValue(v string) string {
	if !strings.ContainsAny(v, ",+=<>#;\"\\\n") && strings.TrimSpace(v) == v {
		return v
	}
	v = strings.ReplaceAll(v, `\`, `\\`)
	v = strings.ReplaceAll(v, `"`, `\"`)
	return `"` + v + `"`
}

// HasSourceStamp reports whether the APK carries a Google Play source stamp.
func HasSourceStamp(path string) bool {
	z, err := zip.OpenReader(path)
	if err != nil {
		return false
	}
	defer func() { _ = z.Close() }()
	for _, f := range z.File {
		if f.Name == "stamp-cert-sha256" {
			return true
		}
	}
	return false
}
