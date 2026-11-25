# Rendered by .github/formula/render.sh during the release workflow.
# Edit this template, not Formula/ak.rb.
class Ak < Formula
  desc "ADB extensions kit - Essential ADB utilities for Android development"
  homepage "https://github.com/luminousvault/homebrew-adb-extensions"
  version "2.5.0"
  license "MIT"

  # Prebuilt binaries from the GitHub release: no Go toolchain on the user's machine.
  on_macos do
    # Universal binary: Apple Silicon and Intel in one file.
    url "https://github.com/luminousvault/homebrew-adb-extensions/releases/download/v2.5.0/ak_2.5.0_macos.tar.gz"
    sha256 "6722c97da942108207c4538233cf825ecf537ca9c5fd026db175cbf0da0d54ab"
  end

  on_linux do
    on_arm do
      url "https://github.com/luminousvault/homebrew-adb-extensions/releases/download/v2.5.0/ak_2.5.0_linux_arm64.tar.gz"
      sha256 "d3a96ef24b981992fba062b624faf05cb5272c03f11fbf60a8d804dc5068e83f"
    end
    on_intel do
      url "https://github.com/luminousvault/homebrew-adb-extensions/releases/download/v2.5.0/ak_2.5.0_linux_x86_64.tar.gz"
      sha256 "6e5673be56a057af0d481cd50502fd06939924988813ac5996a53efc02393136"
    end
  end

  def install
    bin.install "ak"
    # bash, zsh and fish completion from `ak completion <shell>`
    generate_completions_from_executable(bin/"ak", "completion")
  end

  def caveats
    <<~EOS
      ak needs adb. If it is not installed yet:
        brew install --cask android-platform-tools
      Run 'ak doctor' to see which adb ak uses.

      To enable tab completion, restart your terminal.
      PowerShell and other setups: ak completion --help
    EOS
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/ak --version")
    assert_match "install", shell_output("#{bin}/ak --help")
  end
end
