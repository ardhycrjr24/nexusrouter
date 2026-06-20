# Auto-updated by release.yml on tag v0.1.18. Do not edit manually.
class Keirouter < Formula
  desc "AI API router — unified gateway for 20+ LLM providers with fallback, caching, and dashboard"
  homepage "https://github.com/ardhycrjr24/nexusrouter"
  version "0.1.18"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/ardhycrjr24/nexusrouter/releases/download/v0.1.18/nexusrouter_v0.1.18_darwin_arm64.tar.gz"
      sha256 "2c5036265dde53934e0e0fabb198ee81b111ffad781ac2f13393418479638a09"
    else
      url "https://github.com/ardhycrjr24/nexusrouter/releases/download/v0.1.18/nexusrouter_v0.1.18_darwin_amd64.tar.gz"
      sha256 "35ef072ff17a3d130635b37a1355dc6e532b3bdf30905047a3984c18ed94bbf0"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/ardhycrjr24/nexusrouter/releases/download/v0.1.18/nexusrouter_v0.1.18_linux_arm64.tar.gz"
      sha256 "e9ffcd8c05d4351e7e87309219ae33538dd22e49fc6f8aca52ae871ad8785e50"
    else
      url "https://github.com/ardhycrjr24/nexusrouter/releases/download/v0.1.18/nexusrouter_v0.1.18_linux_amd64.tar.gz"
      sha256 "d57dd3dbf5976212b63bc17f6e00cb1d4fc13b06e4186884e795da79f1a564fa"
    end
  end

  def install
    bin.install "nexusrouter"
    (share/"nexusrouter").install "frontend"
  end

  def caveats
    <<~EOS
      Quick start:
        nexusrouter -bootstrap    # create your first API key
        nexusrouter               # start server on :20180

      Dashboard: http://localhost:20180  (default password: nexusrouter)
    EOS
  end

  test do
    assert_match "NexusRouter", shell_output("\#{bin}/nexusrouter --help 2>&1", 2)
  end
end
