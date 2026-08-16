class Memos2txt < Formula
  desc "Audio Files to Cloud ASR (Groq / Deepgram / AssemblyAI) to Deterministic JSON"
  homepage "https://github.com/Maolon/memo2txt"
  version "0.1.0"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/Maolon/memo2txt/releases/latest/download/memos2txt-darwin-arm64.tar.gz"
    else
      url "https://github.com/Maolon/memo2txt/releases/latest/download/memos2txt-darwin-amd64.tar.gz"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/Maolon/memo2txt/releases/latest/download/memos2txt-linux-arm64.tar.gz"
    else
      url "https://github.com/Maolon/memo2txt/releases/latest/download/memos2txt-linux-amd64.tar.gz"
    end
  end

  head "https://github.com/Maolon/memo2txt.git", branch: "main"

  depends_on "go" => :build if build.head?

  def install
    if build.head?
      system "go", "build", "-trimpath", "-ldflags", "-s -w", "-o", bin/"memos2txt", "./cmd/memos2txt"
    else
      bin.install "memos2txt"
    end
  end

  test do
    output = shell_output("#{bin}/memos2txt auth --list")
    assert_match '"ok":true', output
    assert_match '"mode":"auth"', output
  end
end
