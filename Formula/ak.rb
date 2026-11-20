# Rendered by .github/formula/render.sh during the release workflow.
# Edit this template, not Formula/ak.rb.
class Ak < Formula
  desc "ADB extensions kit - Essential ADB utilities for Android development"
  homepage "https://github.com/luminousvault/homebrew-adb-extensions"
  version "2.4.1"
  license "MIT"

  # Prebuilt binaries from the GitHub release: no Go toolchain on the user's machine.
  on_macos do
    # Universal binary: Apple Silicon and Intel in one file.
    url "https://github.com/luminousvault/homebrew-adb-extensions/releases/download/v2.4.1/ak_2.4.1_macos.tar.gz"
    sha256 "00214487195f7ef936926da7986b81d72c49f20a6d5569193de61cbf6e1dab2d"
  end

  on_linux do
    on_arm do
      url "https://github.com/luminousvault/homebrew-adb-extensions/releases/download/v2.4.1/ak_2.4.1_linux_arm64.tar.gz"
      sha256 "29ccb4f784a60444e429d02c857c98458a17d650880d6e48c968f7c28440632e"
    end
    on_intel do
      url "https://github.com/luminousvault/homebrew-adb-extensions/releases/download/v2.4.1/ak_2.4.1_linux_x86_64.tar.gz"
      sha256 "23c6869e03bcf4c7e70ebf69c9308d6d83c97eb60d4b61055c7fe5bb68ce727e"
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
