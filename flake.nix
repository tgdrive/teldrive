{
  description = "Teldrive server binary";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs = { self, nixpkgs, ... }:
    let
      systems = [ "x86_64-linux" "aarch64-linux" ];
      forAllSystems = nixpkgs.lib.genAttrs systems;
    in {
      packages = forAllSystems (system:
        let
          pkgs = import nixpkgs { inherit system; };
          lib = pkgs.lib;

          version = "dev";
          commit = self.shortRev or self.dirtyShortRev or "unknown";
          buildDate = self.lastModifiedDate or "unknown";

          uiSrc = lib.cleanSourceWith {
            src = ./ui;
            filter = path: _type:
              !(builtins.elem (baseNameOf path) [ "node_modules" "dist" "test-results" ]);
          };

          goSrc = lib.cleanSourceWith {
            src = ./.;
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

          # Fixed-output derivation: installs UI dependencies (network)
          # and runs the offline Vite build, publishing only dist/.
          # A separate node_modules derivation is not possible because its
          # contents reference store paths, which fixed-output derivations
          # must not contain.
          uiDist = pkgs.stdenv.mkDerivation {
            name = "teldrive-ui-dist";
            src = uiSrc;
            buildInputs = [ pkgs.bun ];
            nativeBuildInputs = [ pkgs.nodejs ];
            buildPhase = ''
              runHook preBuild
              export HOME=$TMPDIR
              export BUN_INSTALL_CACHE_DIR=$TMPDIR/bun-cache
              bun install --frozen-lockfile --no-progress
              # Sandbox has no /usr/bin/env; rewrite shim interpreters to
              # store paths (follows .bin symlinks into package files).
              patchShebangs node_modules/.bin node_modules/vite
              bun run build
              runHook postBuild
            '';
            installPhase = ''
              runHook preInstall
              mkdir -p $out
              cp -r dist/. $out/
              runHook postInstall
            '';
            outputHashMode = "recursive";
            outputHashAlgo = "sha256";
            outputHash = "sha256-jmWxLtRsj+KmRL2vtchJtU0n2wg/ub5EMv1HgtH4aX4=";
          };

          teldrive = pkgs.buildGoModule {
            pname = "teldrive";
            inherit version;
            src = goSrc;
            vendorHash = "sha256-zBKc/TkO8TJTMVk2H/aTlD8f/3IPiey5Apn54Twx8yk=";
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
          };
        in {
          teldrive = teldrive;
          default = teldrive;
        });
    };
}
