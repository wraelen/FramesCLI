# FramesCLI Homebrew Formula
#
# To use this tap:
#   brew tap wraelen/framescli
#   brew install framescli
#
# Or install directly from this repo:
#   brew install wraelen/framescli/framescli

class Framescli < Formula
  desc "Turn screen recordings into agent-ready artifacts: frames, transcripts, metadata"
  homepage "https://github.com/wraelen/framescli"
  version "0.1.0"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/wraelen/framescli/releases/download/v0.1.0/framescli_0.1.0_darwin_arm64.tar.gz"
      sha256 "REPLACE_WITH_ACTUAL_SHA256_FOR_DARWIN_ARM64"
    else
      url "https://github.com/wraelen/framescli/releases/download/v0.1.0/framescli_0.1.0_darwin_amd64.tar.gz"
      sha256 "REPLACE_WITH_ACTUAL_SHA256_FOR_DARWIN_AMD64"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/wraelen/framescli/releases/download/v0.1.0/framescli_0.1.0_linux_arm64.tar.gz"
      sha256 "REPLACE_WITH_ACTUAL_SHA256_FOR_LINUX_ARM64"
    else
      url "https://github.com/wraelen/framescli/releases/download/v0.1.0/framescli_0.1.0_linux_amd64.tar.gz"
      sha256 "REPLACE_WITH_ACTUAL_SHA256_FOR_LINUX_AMD64"
    end
  end

  depends_on "ffmpeg"

  def install
    bin.install "framescli"

    # Generate shell completions
    generate_completions_from_executable(bin/"framescli", "completion")
  end

  def caveats
    <<~EOS
      FramesCLI has been installed!

      Optional dependencies for transcription:
        pip install openai-whisper
        # or
        pip install faster-whisper

      Quick start:
        framescli doctor
        framescli setup

      For AI agents (MCP integration):
        See: https://github.com/wraelen/framescli/blob/main/docs/AGENT_INTEGRATION.md
    EOS
  end

  test do
    # Test that the binary runs and shows version/help
    assert_match "FramesCLI", shell_output("#{bin}/framescli --help")

    # Test doctor command (should work even without whisper)
    system "#{bin}/framescli", "doctor", "--json"
  end
end
