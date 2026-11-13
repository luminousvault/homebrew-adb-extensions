# Rendered by .github/formula/render.sh during the release workflow.
# Edit this template, not Formula/ak.rb.
class Ak < Formula
  desc "ADB extensions kit - Essential ADB utilities for Android development"
  homepage "https://github.com/luminousvault/homebrew-adb-extensions"
  version "2.1.0"
  license "MIT"

  # Prebuilt binaries from the GitHub release: no Go toolchain on the user's machine.
  on_macos do
    # Universal binary: Apple Silicon and Intel in one file.
    url "https://github.com/luminousvault/homebrew-adb-extensions/releases/download/v2.1.0/ak_2.1.0_macos.tar.gz"
    sha256 "853732a3559b56f59f6f2e922754bb65d3d1fe58133bae49844d7caeaa8cb34d"
  end

  on_linux do
    on_arm do
      url "https://github.com/luminousvault/homebrew-adb-extensions/releases/download/v2.1.0/ak_2.1.0_linux_arm64.tar.gz"
      sha256 "7cadcdcdd06b0f1edb69056c7b52f461ef47112ee74f519ea293467d15b346d0"
    end
    on_intel do
      url "https://github.com/luminousvault/homebrew-adb-extensions/releases/download/v2.1.0/ak_2.1.0_linux_x86_64.tar.gz"
      sha256 "a40db59c8611432c6949fd0fcebcd16ebb5e1e93f51ca9982c4a327d1beca73a"
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
