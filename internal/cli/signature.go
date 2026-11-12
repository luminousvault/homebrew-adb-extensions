package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/luminousvault/homebrew-adb-extensions/internal/adb"
	"github.com/luminousvault/homebrew-adb-extensions/internal/apk"
	"github.com/luminousvault/homebrew-adb-extensions/internal/debuglog"
	. "github.com/luminousvault/homebrew-adb-extensions/internal/style"
	"github.com/luminousvault/homebrew-adb-extensions/internal/tui"
)

var cmdSignature = &Command{
	Name: "signature", Desc: "Extract signature hash",
	Usage: "signature [package|apk]", Help: "Show signature",
	Flags: helpFlags, Run: runSignature,
}

func showHelpSignature() {
	say("%s%sUsage:%s ak signature [-h|--help] [packageName|/path/to/app.apk]", Cyan, Bold, NC)
	say("")
	say("Description: Extract the SHA-256 certificate digest of an APK.")
	say("You can provide either a package name or a local APK file path.")
	say("")
	say("Options:")
	say("  -h, --help       Show this help message")
	say("")
	say("Arguments:")
	say("  packageName      - Package name installed on the device (e.g., com.example.app)")
	say("  /path/to/app.apk - Local APK file path (must end with .apk)")
	say("  (no argument)    - Interactive selection: foreground apps from all devices + APK files")
	say("")
	say("Note: No Android SDK is required. If the APK cannot be parsed, apksigner from")
	say("      Android SDK build-tools is used as a fallback.")
	say("")
}

// signatureTarget is a package on a device or a local APK file.
type signatureTarget struct {
	pkg, serial string // package target
	apkPath     string // local APK target
}

func selectSignatureTarget() signatureTarget {
	var items []string
	var targets []signatureTarget
	for _, serial := range adb.ReadySerials() {
		pkg := foregroundPackage(serial)
		if pkg == "" || pkg == "null" {
			continue
		}
		items = append(items, fmt.Sprintf("📱 %s (%s)", pkg, prettyDevice(serial, "minimal")))
		targets = append(targets, signatureTarget{pkg: pkg, serial: serial})
	}
	say("")
	say("%s %sScanning APK files in the current directory...%s", BArrow, Bold, NC)
	for _, f := range listAPKs(".", sortByName) {
		items = append(items, "📦 "+filepath.Base(f))
		targets = append(targets, signatureTarget{apkPath: f})
	}
	switch len(items) {
	case 0:
		say("")
		say("%s No options available:", Error)
		say("  - No foreground app detected on any connected device")
		say("  - No APK files found in current directory")
		say("")
		say("Please specify a package name or APK file path explicitly.")
		exit(1)
	case 1:
		t := targets[0]
		if t.pkg != "" {
			say("%s Auto-selected: %s%s%s on %s%s%s", BArrow, Yellow, t.pkg, NC, Dim, t.serial, NC)
		} else {
			say("%s Auto-selected: %s%s%s", BArrow, Yellow, filepath.Base(t.apkPath), NC)
		}
		return t
	}
	say("")
	r := pick(tui.Options{Prompt: "Select target for signature extraction", Items: items})
	return targets[r.Index]
}

func absPath(p string) string {
	if a, err := filepath.Abs(p); err == nil {
		if r, err := filepath.EvalSymlinks(a); err == nil {
			return r
		}
		return a
	}
	return p
}

func runSignature(args []string) {
	args = parseLeading("signature", args, showHelpSignature, nil, true)

	var t signatureTarget
	if len(args) > 0 && args[0] != "" {
		input := args[0]
		isLocal := strings.HasSuffix(input, ".apk")
		if !isLocal {
			t.serial = selectDevice()
		}
		say("")
		if isLocal {
			say("%sUsing local APK file:%s %s", Blue, NC, input)
			if st, err := os.Stat(input); err != nil || st.IsDir() {
				say("%sERROR: Local APK file not found:%s %s", Red, NC, input)
				say("")
				exit(1)
			}
			t.apkPath = absPath(input)
			say("%s==> Using APK file:%s %s", Green, NC, t.apkPath)
		} else {
			t.pkg = input
			say("%sUsing specified package:%s %s", Blue, NC, input)
			requirePackage(t.serial, input)
		}
	} else {
		t = selectSignatureTarget()
		say("")
		if t.pkg != "" {
			say("%sSelected package:%s %s", Blue, NC, t.pkg)
			say("%sDevice:%s %s", Dim, NC, prettyDevice(t.serial, "short"))
			requirePackage(t.serial, t.pkg)
		} else {
			say("%sSelected APK file:%s %s", Blue, NC, filepath.Base(t.apkPath))
			t.apkPath = absPath(t.apkPath)
			say("%s==> Using APK file:%s %s", Green, NC, t.apkPath)
		}
	}
	say("")

	apkPath := t.apkPath
	if t.pkg != "" {
		remote, ok := apkPathForPackage(t.serial, t.pkg)
		if !ok {
			exit(1)
		}
		tmp, err := os.CreateTemp("", "ak_signature_*.apk")
		if err != nil {
			say("%s %v", Error, err)
			exit(1)
		}
		_ = tmp.Close()
		defer func() { _ = os.Remove(tmp.Name()) }()
		say("%s==> Pulling APK from device...%s", Blue, NC)
		if _, err := adb.Combined("-s", t.serial, "pull", remote, tmp.Name()); err != nil {
			say("")
			say("%sERROR: Failed to pull APK from device. Check device connection and permissions.%s", Red, NC)
			exit(1)
		}
		apkPath = tmp.Name()
		say("")
	}

	say("%s==> Extracting signature...%s", Blue, NC)
	if signers, err := apk.Signers(apkPath); err == nil {
		for i, s := range signers {
			n := i + 1
			say("Signer #%d certificate DN: %s", n, s.DN)
			say("%s%sSigner #%d certificate SHA-256 digest: %s%s", Green, Bold, n, s.SHA256, NC)
			say("%s%sSigner #%d certificate SHA-1 digest: %s%s", Yellow, Bold, n, s.SHA1, NC)
			say("Signer #%d certificate MD5 digest: %s", n, s.MD5)
		}
		if apk.HasSourceStamp(apkPath) {
			say("%sNote: the Source Stamp signer (added by Google Play) is not shown.%s", Dim, NC)
		}
	} else {
		debuglog.Logf("apk.Signers failed: %v", err)
		signatureWithAPKSigner(apkPath, err)
	}
	say("")
	say("%sSignature extraction complete.%s", Green, NC)
}

// signatureWithAPKSigner falls back to the SDK's apksigner when the Go parser fails.
func signatureWithAPKSigner(apkPath string, parseErr error) {
	tool := adb.FindBuildTool("apksigner")
	if tool == "" {
		say("%s Could not read the APK signature: %v", Error, parseErr)
		say("")
		say("%sapksigner (Android SDK build-tools) was not found for a fallback.%s", Yellow, NC)
		say("")
		exit(1)
	}
	say("%s   Falling back to apksigner...%s", Dim, NC)
	run := func(extra ...string) string {
		out, _ := exec.Command(tool, append(append([]string{"verify", "--print-certs"}, extra...), apkPath)...).CombinedOutput()
		return string(out)
	}
	out := run()
	switch {
	case strings.Contains(out, "not supported on API Level"):
		say("%s   Detected legacy signing algorithm, retrying...%s", Dim, NC)
		out = run("--min-sdk-version", "21")
	case (strings.Contains(out, "DOES NOT VERIFY") || strings.Contains(out, "ERROR")) && !strings.Contains(out, "certificate"):
		say("%s   Retrying with --min-sdk-version 21...%s", Dim, NC)
		out = run("--min-sdk-version", "21")
	}
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		switch {
		case strings.HasPrefix(line, "WARNING:"):
		case strings.Contains(line, "SHA-256"):
			say("%s%s%s%s", Green, Bold, line, NC)
		case strings.Contains(line, "SHA-1"):
			say("%s%s%s%s", Yellow, Bold, line, NC)
		default:
			say("%s", line)
		}
	}
	if strings.Contains(out, "DOES NOT VERIFY") && strings.Contains(out, "certificate") {
		say("")
		say("%sNote: Signature extracted but verification failed.%s", Yellow, NC)
		say("%sThis may occur with system apps or APKs missing v1 signature.%s", Dim, NC)
	}
}
