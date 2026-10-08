# Teldrive server binary (embeds the web UI from uiDist).
{ lib, buildGoModule, version, commit, buildDate, uiDist }:
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
  # Build-only flake: tests run via just/test harnesses instead.
  doCheck = false;
}
