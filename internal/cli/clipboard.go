package cli

import (
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

var flagCopy = Flag{"-c", "--copy", "", "Copy the image to the clipboard (alone: no file is saved)"}

// copyImageFn puts a PNG file on the system clipboard as an image; tests
// replace it.
var copyImageFn = copyImage

func copyImage(path string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		// PNG plus an LZW-compressed TIFF, like Preview's copy: AppleScript's
		// «class PNGf» pastes fine but clipboard history apps (Raycast) only
		// record images that also come as TIFF, and an uncompressed TIFF is
		// ~8x the PNG. The path goes in as an argument, so it needs no escaping.
		cmd = exec.Command("osascript", "-l", "JavaScript", "-e", macCopyImageJS, path)
	case "windows":
		cmd = exec.Command("powershell", "-NoProfile", "-STA", "-Command",
			"Add-Type -AssemblyName System.Windows.Forms, System.Drawing; "+
				"$img = [System.Drawing.Image]::FromFile($env:AK_CLIPBOARD_FILE); "+
				"[System.Windows.Forms.Clipboard]::SetImage($img); $img.Dispose()")
		cmd.Env = append(os.Environ(), "AK_CLIPBOARD_FILE="+path)
	default:
		tool, args := linuxClipboard()
		if tool == "" {
			return errors.New("no clipboard tool found (install wl-clipboard or xclip)")
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		cmd = exec.Command(tool, args...)
		cmd.Stdin = f
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		if msg := strings.TrimSpace(string(out)); msg != "" {
			// osascript: "execution error: Error: Error: cannot read the image (-2700)"
			msg = strings.TrimPrefix(msg, "execution error: ")
			for strings.HasPrefix(msg, "Error: ") {
				msg = strings.TrimPrefix(msg, "Error: ")
			}
			return errors.New(strings.TrimSuffix(msg, " (-2700)"))
		}
		return err
	}
	return nil
}

// macCopyImageJS writes one pasteboard item with public.png (the original
// bytes) and public.tiff (NSTIFFCompressionLZW = 5).
const macCopyImageJS = `function run(argv) {
  ObjC.import("AppKit");
  var png = $.NSData.dataWithContentsOfFile(argv[0]);
  var img = png.isNil() ? $() : $.NSImage.alloc.initWithData(png);
  if (img.isNil()) throw new Error("cannot read the image");
  var item = $.NSPasteboardItem.alloc.init;
  item.setDataForType(png, "public.png");
  item.setDataForType(img.TIFFRepresentationUsingCompressionFactor(5, 0), "public.tiff");
  var pb = $.NSPasteboard.generalPasteboard;
  pb.clearContents;
  if (!pb.writeObjects($([item]))) throw new Error("the clipboard did not accept the image");
}`

// linuxClipboard picks wl-copy on Wayland, else xclip.
func linuxClipboard() (string, []string) {
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		if _, err := exec.LookPath("wl-copy"); err == nil {
			return "wl-copy", []string{"--type", "image/png"}
		}
	}
	if _, err := exec.LookPath("xclip"); err == nil {
		return "xclip", []string{"-selection", "clipboard", "-t", "image/png", "-i"}
	}
	return "", nil
}
