class Ak < Formula
  desc "ADB extensions kit - Essential ADB utilities for Android development"
  homepage "https://github.com/luminousvault/homebrew-adb-extensions"
  url "https://github.com/luminousvault/homebrew-adb-extensions/releases/download/v1.3.0/adb-extensions-v1.3.0.tar.gz"
  sha256 "5126f558bb48899d3fd88c26591523cb91fcf7e4d293bc512adc6a27e109e9d6"
  license "MIT"
  version "1.3.0"

  # depends_on "android-platform-tools"  # adb 의존성

  def install
    # 쉘 스크립트 설치
    bin.install "build/ak" => "ak"
    # Completion 설치
    zsh_completion.install "build/completions/_ak"
  end
  
  def caveats
    <<~EOS
        ⚠️ IMPORTANT: To enable tab completion, restart your terminal
    EOS
  end

  test do
    # 버전 체크 (formula의 version 속성을 참조하므로 릴리즈마다 자동 반영)
    assert_match version.to_s, shell_output("#{bin}/ak --version")

    # 도움말 라우팅 체크
    assert_match "install", shell_output("#{bin}/ak --help")
  end
end
