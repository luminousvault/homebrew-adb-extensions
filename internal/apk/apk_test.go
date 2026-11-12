package apk

import (
	"crypto/x509/pkix"
	"encoding/asn1"
	"testing"
)

// testdata/aktest.apk is a minimal APK (com.example.aktest) built with aapt2 and
// signed by a throwaway test key. Expected values come from aapt and apksigner.
const testAPK = "testdata/aktest.apk"

func TestPackageName(t *testing.T) {
	got, err := PackageName(testAPK)
	if err != nil || got != "com.example.aktest" {
		t.Errorf("got %q, %v", got, err)
	}
}

func TestSigners(t *testing.T) {
	ss, err := Signers(testAPK)
	if err != nil || len(ss) != 1 {
		t.Fatalf("signers %v, %v", ss, err)
	}
	want := Signer{
		DN:     "CN=ak test A, O=ak, C=KR",
		SHA256: "558e574176d03df75f403b854a339f2f88330387c16d96177c849984e26e68ed",
		SHA1:   "f21d32d77fea779f377ec7c961a6dafb0f8009e3",
		MD5:    "69412f0557c0dc8b897be4f6f3389105",
	}
	if ss[0] != want {
		t.Errorf("got %+v\nwant %+v", ss[0], want)
	}
}

func TestNotAnAPK(t *testing.T) {
	if _, err := PackageName("apk_test.go"); err == nil {
		t.Error("expected error for non-APK")
	}
	if _, err := Signers("apk_test.go"); err == nil {
		t.Error("expected error for non-APK")
	}
}

func TestDistinguishedNameJavaStyle(t *testing.T) {
	email := asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 1}
	name := pkix.Name{
		Country:      []string{"KR"},
		Organization: []string{"ONE store Co., Ltd."},
		CommonName:   "Demo",
		ExtraNames:   []pkix.AttributeTypeAndValue{{Type: email, Value: "a@b.c"}},
	}
	raw, err := asn1.Marshal(name.ToRDNSequence())
	if err != nil {
		t.Fatal(err)
	}
	want := `EMAILADDRESS=a@b.c, CN=Demo, O="ONE store Co., Ltd.", C=KR`
	if got := distinguishedName(raw); got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}
