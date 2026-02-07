# typed: false
# frozen_string_literal: true

class Pier < Formula
  desc "Clean .dock domains for Docker containers and local processes"
  homepage "https://github.com/eshe-huli/pier"
  url "https://github.com/eshe-huli/pier/archive/refs/tags/v0.1.0.tar.gz"
  sha256 "a02919a38cff853859446b5c8aedb2e40e305dae32f279950c90393dd61d2e2b"
  license "MIT"
  head "https://github.com/eshe-huli/pier.git", branch: "main"

  depends_on "go" => :build
  depends_on "dnsmasq"
  depends_on "nginx"

  def install
    ldflags = "-X github.com/eshe-huli/pier/internal/cli.Version=#{version}"
    system "go", "build", *std_go_args(ldflags:), "./cmd/pier"
  end

  def post_install
    ohai "Run 'pier init' to complete setup (configures dnsmasq, nginx, Traefik)"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/pier version")
  end
end
