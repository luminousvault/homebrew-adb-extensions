# Rendered by .github/formula/render.sh during the release workflow.
# Edit this template, not Formula/ak.rb.
class Ak < Formula
  desc "ADB extensions kit - Essential ADB utilities for Android development"
  homepage "https://github.com/luminousvault/homebrew-adb-extensions"
  version "2.1.1"
  license "MIT"

  # Prebuilt binaries from the GitHub release: no Go toolchain on the user's machine.
  on_macos do
    # Universal binary: Apple Silicon and Intel in one file.
    url "https://github.com/luminousvault/homebrew-adb-extensions/releases/download/v2.1.1/ak_2.1.1_macos.tar.gz"
    sha256 "5114ce8b75f95253e270fa4ae4f0be5482d9fbdab5ba2fda9281aa73c45f3035"
  end

  on_linux do
    on_arm do
      url "https://github.com/luminousvault/homebrew-adb-extensions/releases/download/v2.1.1/ak_2.1.1_linux_arm64.tar.gz"
      sha256 "60d67d4a2536dbecd3abba0e53b041d713b4b8fdb3195f81f2306fb9dfa57080"
    end
    on_intel do
      url "https://github.com/luminousvault/homebrew-adb-extensions/releases/download/v2.1.1/ak_2.1.1_linux_x86_64.tar.gz"
      sha256 "c072171652963fbea5f25a516c4c5dd016309a42ed42bc72dce5593890a881d1"
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
