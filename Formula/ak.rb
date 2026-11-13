# Rendered by .github/formula/render.sh during the release workflow.
# Edit this template, not Formula/ak.rb.
class Ak < Formula
  desc "ADB extensions kit - Essential ADB utilities for Android development"
  homepage "https://github.com/luminousvault/homebrew-adb-extensions"
  version "2.2.0"
  license "MIT"

  # Prebuilt binaries from the GitHub release: no Go toolchain on the user's machine.
  on_macos do
    # Universal binary: Apple Silicon and Intel in one file.
    url "https://github.com/luminousvault/homebrew-adb-extensions/releases/download/v2.2.0/ak_2.2.0_macos.tar.gz"
    sha256 "345f1a7e1ed3925ad10b67088e0bd4d015a821013a9fb409ea022afb3be5b7c8"
  end

  on_linux do
    on_arm do
      url "https://github.com/luminousvault/homebrew-adb-extensions/releases/download/v2.2.0/ak_2.2.0_linux_arm64.tar.gz"
      sha256 "e8cb25dad71f169b2a9cf47fdbff2564932ef643ecea07f11de558498b7defed"
    end
    on_intel do
      url "https://github.com/luminousvault/homebrew-adb-extensions/releases/download/v2.2.0/ak_2.2.0_linux_x86_64.tar.gz"
      sha256 "508c351f97322bacec53ee0d26812443a8c20224d080665c47afce4a01d2ca4b"
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
