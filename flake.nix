{
  description = "Teldrive server binary";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  inputs.bun2nix.url = "github:nix-community/bun2nix";
  inputs.bun2nix.inputs.nixpkgs.follows = "nixpkgs";
  inputs.nix-pkgs.url = "github:divyam234/nix-pkgs";
  inputs.nix-pkgs.inputs.nixpkgs.follows = "nixpkgs";

  outputs = { self, nixpkgs, bun2nix, ... }@inputs:
    let
      systems = [ "x86_64-linux" "aarch64-linux" ];
      forAllSystems = nixpkgs.lib.genAttrs systems;
      version = "dev";

      bun2nixPrebuilt = final: prev:
        let
          slimHook = final.makeSetupHook
            {
              name = "bun2nix-hook";
              propagatedBuildInputs = [ final.bun final.yq-go ];
              substitutions = {
                resolveCatalogTs = inputs.bun2nix.outPath + "/nix/mk-derivation/resolve-catalog.ts";
                bunDefaultInstallFlags =
                  if final.stdenv.hostPlatform.isDarwin then
                    [
                      "--linker=isolated"
                      "--backend=symlink"
                    ]
                  else
                    [
                      "--linker=isolated"
                    ];
              };
            }
            (inputs.bun2nix.outPath + "/nix/mk-derivation/hook.sh");
        in
        {
          bun2nix = inputs."nix-pkgs".packages.${final.stdenv.hostPlatform.system}.bun2nix
            // {
              hook = slimHook;
              fetchBunDeps = prev.bun2nix.fetchBunDeps;
            };
        };

      mkPkgs = system: import nixpkgs {
        inherit system;
        overlays = [ bun2nix.overlays.default bun2nixPrebuilt ];
      };
    in {
      packages = forAllSystems (system:
        let
          pkgs = mkPkgs system;

          commit = self.shortRev or self.dirtyShortRev or "unknown";
          buildDate = self.lastModifiedDate or "unknown";

          uiDist = pkgs.callPackage ./nix/ui.nix { inherit version; };
          teldrive = pkgs.callPackage ./nix/package.nix {
            inherit version commit buildDate uiDist;
          };
        in {
          teldrive = teldrive;
          default = teldrive;
          bun2nix = pkgs.bun2nix;
        });

      overlays.default = final: prev: {
        teldrive-ui-dist = final.callPackage ./nix/ui.nix { inherit version; };
        teldrive = final.callPackage ./nix/package.nix {
          inherit version;
          commit = "unknown";
          buildDate = "unknown";
          uiDist = final.teldrive-ui-dist;
        };
      };

      nixosModules.default = import ./nix/modules/nixos.nix;
      homeManagerModules.default = import ./nix/modules/home.nix;

      devShells = forAllSystems (system:
        let
          pkgs = mkPkgs system;
        in {
          default = pkgs.mkShell {
            packages = with pkgs; [
              go
              bun
              pkgs.bun2nix
              nodejs
              chromium
              ffmpeg
              sqlc
              just
              git
              podman
              postgresql
            ];
            PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH = "${pkgs.chromium}/bin/chromium";
            PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD = "1";
            shellHook = ''
              echo "teldrive dev shell: $(go version | cut -d' ' -f3), bun $(bun --version), sqlc $(sqlc version 2>/dev/null | head -n1)"
              echo "run 'just --list' for workflows (try: just install-tools)"
            '';
          };
        });
    };
}
