package adb

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestParseDevices(t *testing.T) {
	out := "* daemon not running; starting now at tcp:5037\nList of devices attached\nSERIAL0001\tdevice\nSERIAL0002\tunauthorized\n10.0.0.5:5555\toffline\n\n"
	got := ParseDevices(out)
	want := []DeviceEntry{{"SERIAL0001", "device"}, {"SERIAL0002", "unauthorized"}, {"10.0.0.5:5555", "offline"}}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] got %v want %v", i, got[i], want[i])
		}
	}
}

func TestParseGetprop(t *testing.T) {
	out := "[ro.product.brand]: [google]\r\n[ro.product.model]: [Pixel 9 Pro]\n[ro.empty]: []\nnoise\n"
	p := ParseGetprop(out)
	if p["ro.product.brand"] != "google" || p["ro.product.model"] != "Pixel 9 Pro" {
		t.Errorf("got %v", p)
	}
	if v, ok := p["ro.empty"]; !ok || v != "" {
		t.Errorf("empty value: %q %v", v, ok)
	}
}

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"34.0.0", "35.0.1", -1},
		{"35.0.10", "35.0.9", 1},
		{"36.0.0", "36.0.0", 0},
		{"36.0.0-rc1", "36.0.0", 1},
	}
	for _, c := range cases {
		if got := compareVersions(c.a, c.b); got != c.want {
			t.Errorf("compare(%s,%s)=%d want %d", c.a, c.b, got, c.want)
		}
	}
}

func fakeADB(t *testing.T, dir string) string {
	t.Helper()
	p := filepath.Join(dir, exeName("adb"))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestCandidatesOrder(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PATH/home layout differs on Windows")
	}
	root := t.TempDir()
	home := filepath.Join(root, "home")
	t.Setenv("HOME", home)
	custom := fakeADB(t, filepath.Join(root, "custom"))
	sdk := filepath.Join(root, "sdk")
	sdkADB := fakeADB(t, filepath.Join(sdk, "platform-tools"))
	pathADB := fakeADB(t, filepath.Join(root, "bin"))
	cache := fakeADB(t, filepath.Join(home, ".ak", "platform-tools"))

	t.Setenv("AK_ADB", custom)
	t.Setenv("ANDROID_HOME", sdk)
	t.Setenv("ANDROID_SDK_ROOT", sdk) // same file as ANDROID_HOME: listed once
	t.Setenv("PATH", filepath.Join(root, "bin"))

	got := Candidates()
	want := []Candidate{{custom, "AK_ADB"}, {sdkADB, "ANDROID_HOME"}, {pathADB, "PATH"}, {cache, "ak cache"}}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] got %v want %v", i, got[i], want[i])
		}
	}

	t.Setenv("AK_ADB", "")
	if p, _ := Locate(); p != sdkADB {
		t.Errorf("Locate without AK_ADB = %s, want %s", p, sdkADB)
	}
}

func TestLocateNotFound(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LOCALAPPDATA", t.TempDir())
	t.Setenv("AK_ADB", "")
	t.Setenv("ANDROID_HOME", "")
	t.Setenv("ANDROID_SDK_ROOT", "")
	t.Setenv("PATH", t.TempDir())
	if _, err := Locate(); err != ErrNotFound {
		t.Errorf("err = %v", err)
	}
}
