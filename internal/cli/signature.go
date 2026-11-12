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
	"github.com/luminousvault/homebrew-adb-extensions/internal/tui"
	"github.com/luminousvault/homebrew-adb-extensions/internal/ui"
)

var cmdSignature = &Command{
	Name: "signature", Summary: "Extract signature hash", Args: "[package|file.apk]",
	Long: "Show the signing certificate (DN, SHA-256, SHA-1, MD5) of an installed package or a local APK.\n" +
		"No Android SDK is needed; apksigner is used only if the APK cannot be parsed.\n" +
		"Without an argument, pick from the foreground apps of connected devices and APKs in the current directory.",
	ArgHelp: [][2]string{{"package", "Package installed on the device"}, {"file.apk", "Local APK file"}},
	Examples: []string{"ak signature", "ak signature com.example.app", "ak signature app-release.apk",
		"ak signature app.apk --json | jq -r '.signers[0].sha256'"},
	Device: oneDevice, JSON: true, Run: runSignature,
}

type signatureTarget struct {
	pkg, serial string
	apkPath     string
}

func (t signatureTarget) describe() string {
	if t.pkg != "" {
		return q(t.pkg) + " on " + deviceName(t.serial)
	}
	return q(filepath.Base(t.apkPath))
}

// pickSignatureTarget lists foreground apps of devices and local APKs.
func pickSignatureTarget(c *Ctx) (signatureTarget, error) {
	serials := c.Strings("--serial")
	if len(serials) == 0 {
		if env := os.Getenv("ANDROID_SERIAL"); env != "" {
			serials = []string{env}
		} else {
			serials = adb.ReadySerials()
		}
	}
	var items []string
	var targets []signatureTarget
	for _, serial := range serials {
		if pkg := foregroundPackage(serial); pkg != "" && pkg != "null" {
			items = append(items, fmt.Sprintf("%s\t(foreground app on %s)", pkg, deviceName(serial)))
			targets = append(targets, signatureTarget{pkg: pkg, serial: serial})
		}
	}
	for _, f := range listAPKs(".", sortByName) {
		items = append(items, filepath.Base(f)+"\t(local APK)")
		targets = append(targets, signatureTarget{apkPath: f})
	}
	switch len(items) {
	case 0:
		return signatureTarget{}, failErr("Pass a package name or an APK file",
			"No foreground app on connected devices and no APK files in the current directory")
	case 1:
		return targets[0], nil
	}
	r, err := pick(tui.Options{Prompt: "Select a package or APK", Items: items}, "Pass a package name or an APK file")
	if err != nil {
		return signatureTarget{}, err
	}
	return targets[r.Index], nil
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

func resolveSignatureTarget(c *Ctx) (signatureTarget, error) {
	if len(c.Args) == 0 {
		return pickSignatureTarget(c)
	}
	input := c.Args[0]
	if strings.HasSuffix(input, ".apk") {
		if st, err := os.Stat(input); err != nil || st.IsDir() {
			return signatureTarget{}, failErr("", "APK file not found: %s", q(input))
		}
		return signatureTarget{apkPath: absPath(input)}, nil
	}
	serial, err := c.Device()
	if err != nil {
		return signatureTarget{}, err
	}
	return signatureTarget{pkg: input, serial: serial}, nil
}

type signerJSON struct {
	DN     string `json:"dn"`
	SHA256 string `json:"sha256"`
	SHA1   string `json:"sha1"`
	MD5    string `json:"md5"`
}

func runSignature(c *Ctx) error {
	if err := tooMany(c, 1); err != nil {
		return err
	}
	t, err := resolveSignatureTarget(c)
	if err != nil {
		return err
	}
	if t.pkg != "" {
		if err := requirePackage(t.serial, t.pkg); err != nil {
			return err
		}
	}

	apkPath := t.apkPath
	if t.pkg != "" {
		remote, err := apkPathForPackage(t.serial, t.pkg)
		if err != nil {
			return err
		}
		tmp, err := os.CreateTemp("", "ak_signature_*.apk")
		if err != nil {
			return err
		}
		_ = tmp.Close()
		defer func() { _ = os.Remove(tmp.Name()) }()
		sp := ui.Spin("Pulling the APK of %s", t.describe())
		out, err := adb.Combined("-s", t.serial, "pull", remote, tmp.Name())
		sp.Stop()
		if err != nil {
			ui.Errorf("Couldn't pull the APK of %s", q(t.pkg))
			ui.Detail("%s", out)
			return errReported
		}
		apkPath = tmp.Name()
	}

	signers, err := apk.Signers(apkPath)
	if err != nil {
		debuglog.Logf("apk.Signers failed: %v", err)
		if c.JSON() {
			return failErr("", "Could not read the signature of %s: %v", t.describe(), err)
		}
		return signatureWithAPKSigner(apkPath, t.describe(), err)
	}
	stamp := apk.HasSourceStamp(apkPath)
	if c.JSON() {
		list := make([]signerJSON, len(signers))
		for i, s := range signers {
			list[i] = signerJSON{s.DN, s.SHA256, s.SHA1, s.MD5}
		}
		src := map[string]any{"signers": list, "sourceStamp": stamp}
		if t.pkg != "" {
			src["package"], src["serial"] = t.pkg, t.serial
		} else {
			src["file"] = t.apkPath
		}
		return ui.JSON(src)
	}
	ui.Step("Signature of %s", t.describe())
	for i, s := range signers {
		if i > 0 {
			ui.Out.Println("")
		}
		ui.Out.Println(ui.Out.Bold(fmt.Sprintf("Signer #%d", i+1)))
		ui.KeyValues([][2]string{{"DN", s.DN}, {"SHA-256", s.SHA256}, {"SHA-1", s.SHA1}, {"MD5", s.MD5}})
	}
	if stamp {
		ui.Detail("The Source Stamp signer added by Google Play is not shown")
	}
	return nil
}

// signatureWithAPKSigner falls back to the SDK's apksigner when the Go parser fails.
func signatureWithAPKSigner(apkPath, label string, parseErr error) error {
	tool := adb.FindBuildTool("apksigner")
	if tool == "" {
		return failErr("Install Android SDK build-tools to use apksigner as a fallback",
			"Could not read the signature of %s: %v", label, parseErr)
	}
	ui.Warn("Could not parse the APK (%v); using apksigner", parseErr)
	run := func(extra ...string) string {
		out, _ := exec.Command(tool, append(append([]string{"verify", "--print-certs"}, extra...), apkPath)...).CombinedOutput()
		return string(out)
	}
	out := run()
	if !strings.Contains(out, "certificate") {
		out = run("--min-sdk-version", "21") // legacy algorithms (MD5/SHA-1) need an older min SDK
	}
	if !strings.Contains(out, "certificate") {
		ui.Errorf("apksigner could not read the signature of %s", label)
		ui.Detail("%s", out)
		return errReported
	}
	ui.Step("Signature of %s (apksigner)", label)
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if !strings.HasPrefix(line, "WARNING:") {
			ui.Out.Println(line)
		}
	}
	if strings.Contains(out, "DOES NOT VERIFY") {
		ui.Warn("The signature was extracted but does not verify (common for system apps or APKs without a v1 signature)")
	}
	return nil
}
