{ lib, stdenv, bun, nodejs, version }:
let
  src = lib.cleanSourceWith {
    src = ../ui;
    filter = path: _type:
      !(builtins.elem (baseNameOf path) [ "node_modules" "dist" "test-results" ]);
  };
  # Lockfiles only, so UI source edits don't invalidate the dep hash.
  depsSrc = lib.fileset.toSource {
    root = ../ui;
    fileset = lib.fileset.unions [
      ../ui/package.json
      ../ui/bun.lock
    ];
  };
  bunDeps = stdenv.mkDerivation {
    pname = "teldrive-ui-node_modules";
    inherit version;
    src = depsSrc;
    nativeBuildInputs = [ bun nodejs ];
    # Fixed-output: network allowed here to fetch the lockfile closure.
    # Update with: just update-ui-deps-hash
    outputHashMode = "recursive";
    outputHashAlgo = "sha256";
    outputHash = "sha256-S4arAot+s4I1IhbW08t4UdmxW7KAe1eXhKBuZ3smyyg=";
    # FOD outputs must not reference /nix/store. Keep vendored files
    # byte-identical to upstream: no shebang rewrites, no ELF patching
    # of prebuilt NAPI binaries (tailwind oxide, rolldown, biome).
    dontPatchShebangs = true;
    dontPatchELF = true;
    buildPhase = ''
      runHook preBuild
      export HOME=$TMPDIR
      export BUN_INSTALL_CACHE=$TMPDIR/bun-cache
      bun install --frozen-lockfile --backend=copyfile
      runHook postBuild
    '';
    installPhase = ''
      runHook preInstall
      cp -r node_modules $out
      runHook postInstall
    '';
  };
in
stdenv.mkDerivation {
  pname = "teldrive-ui-dist";
  inherit version;
  inherit src;
  nativeBuildInputs = [ bun nodejs ];
  buildPhase = ''
    runHook preBuild
    export HOME=$TMPDIR
    cp -r ${bunDeps} node_modules
    chmod -R u+w node_modules
    patchShebangs node_modules
    bun run --frozen-lockfile build
    runHook postBuild
  '';
  installPhase = ''
    runHook preInstall
    mkdir -p $out
    cp -r dist/. $out/
    runHook postInstall
  '';
}
