{ lib, stdenv, installShellFiles, buildGoModule, version, commit, buildDate, uiDist }:
buildGoModule {
  pname = "teldrive";
  inherit version;
  src = lib.cleanSourceWith {
    src = ../.;
    filter = path: _type:
      !(builtins.elem (baseNameOf path) [
        ".git"
        "bin"
        "result"
        "dist"
        "node_modules"
        "test-results"
      ]);
  };
  vendorHash = "sha256-0eX6BtyEzJ30jnYEOaZPzV5etPWdMRX353CsJAA9bHQ=";
  subPackages = [ "cmd/teldrive" ];
  env.CGO_ENABLED = "0";
  ldflags = [
    "-s"
    "-w"
    "-X main.version=${version}"
    "-X main.commit=${commit}"
    "-X main.date=${buildDate}"
  ];
  preBuild = ''
    cp -r ${uiDist} ui/dist
    chmod -R u+w ui/dist
  '';
  doCheck = false;
  nativeBuildInputs = [ installShellFiles ];
  postInstall = lib.optionalString (stdenv.buildPlatform.canExecute stdenv.hostPlatform) ''
    installShellCompletion --cmd teldrive \
      --bash <($out/bin/teldrive completion bash) \
      --zsh <($out/bin/teldrive completion zsh) \
      --fish <($out/bin/teldrive completion fish)
  '';
}
