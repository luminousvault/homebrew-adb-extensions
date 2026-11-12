# Rendered by .github/formula/render.sh during the release workflow.
# Edit this template, not Formula/ak.rb.
class Ak < Formula
  desc "ADB extensions kit - Essential ADB utilities for Android development"
  homepage "https://github.com/luminousvault/homebrew-adb-extensions"
  version "2.0.0"
  license "MIT"

  # Prebuilt binaries from the GitHub release: no Go toolchain on the user's machine.
  on_macos do
    on_arm do
      url "https://github.com/luminousvault/homebrew-adb-extensions/releases/download/v2.0.0/ak_2.0.0_darwin_arm64.tar.gz"
      sha256 "3a9779a7d34678a08ad0e4d010789d800904c5b5461705040e32d25613a9c2ee"
    end
    on_intel do
      url "https://github.com/luminousvault/homebrew-adb-extensions/releases/download/v2.0.0/ak_2.0.0_darwin_amd64.tar.gz"
      sha256 "9aa3b7074251c72fe45f31dfcbb72fbc437f265e4c714a841cc12a6c3aeeb068"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/luminousvault/homebrew-adb-extensions/releases/download/v2.0.0/ak_2.0.0_linux_arm64.tar.gz"
      sha256 "b3123cecb0a055aecd5e53ddbdacc0342dba35a8afbb6fd66de3f81a5638bca1"
    end
    on_intel do
      url "https://github.com/luminousvault/homebrew-adb-extensions/releases/download/v2.0.0/ak_2.0.0_linux_amd64.tar.gz"
      sha256 "f9685794b140adfe243afd4a86a5d743e17bbd3a6303236fa60cb7559919b598"
    end
  end

  def install
    bin.install "ak"
    zsh_completion.install "completions/_ak"
  end

  def caveats
    <<~EOS
      ak needs adb. If it is not installed yet:
        brew install --cask android-platform-tools
      Run 'ak doctor' to see which adb ak uses.

      To enable tab completion, restart your terminal.
    EOS
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/ak --version")
    assert_match "install", shell_output("#{bin}/ak --help")
  end
end
