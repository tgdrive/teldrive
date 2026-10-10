{ lib, stdenv, fetchurl, installShellFiles }:
let
  release = builtins.fromJSON (builtins.readFile ./release.json);
  arch = { x86_64-linux = "amd64"; aarch64-linux = "arm64"; }.${stdenv.hostPlatform.system} or (throw "Unsupported teldrive-bin platform");
in stdenv.mkDerivation {
  pname = "teldrive-bin";
  inherit (release) version;
  src = if release.version == null then throw "Run just update-bin VERSION after publishing a release" else fetchurl {
    url = "https://github.com/tgdrive/teldrive/releases/download/v${release.version}/teldrive-v${release.version}-linux-${arch}.tar.gz";
    hash = release.hashes.${arch};
  };
  sourceRoot = ".";
  nativeBuildInputs = [ installShellFiles ];
  installPhase = ''
    runHook preInstall
    install -Dm755 teldrive $out/bin/teldrive
    installShellCompletion --cmd teldrive --bash completions/teldrive.bash --zsh completions/teldrive.zsh --fish completions/teldrive.fish
    runHook postInstall
  '';
  meta = {
    description = "Telegram-backed cloud storage server (release binary)";
    mainProgram = "teldrive";
    platforms = [ "x86_64-linux" "aarch64-linux" ];
  };
}
