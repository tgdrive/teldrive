{
  description = "Teldrive server binary";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  inputs.bun2nix.url = "github:nix-community/bun2nix";
  inputs.bun2nix.inputs.nixpkgs.follows = "nixpkgs";

  outputs = { self, nixpkgs, bun2nix, ... }:
    let
      systems = [ "x86_64-linux" "aarch64-linux" ];
      forAllSystems = nixpkgs.lib.genAttrs systems;
      version = "dev";
    in {
      packages = forAllSystems (system:
        let
          pkgs = import nixpkgs {
            inherit system;
            overlays = [ bun2nix.overlays.default ];
          };

          commit = self.shortRev or self.dirtyShortRev or "unknown";
          buildDate = self.lastModifiedDate or "unknown";

          uiDist = pkgs.callPackage ./nix/ui.nix { inherit version; };
          teldrive = pkgs.callPackage ./nix/package.nix {
            inherit version commit buildDate uiDist;
          };
        in {
          teldrive = teldrive;
          default = teldrive;
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
          pkgs = import nixpkgs {
            inherit system;
            overlays = [ bun2nix.overlays.default ];
          };
        in {
          # Toolchain for the just workflows: go/bun/nodejs run the code,
          # sqlc + patchsqlc regenerate the DB layer (must be v1.31.1),
          # just drives justfile, podman backs integration tests,
          # postgresql provides psql for debugging test databases.
          # NOTE: bun2nix is deliberately absent here (its nix-community
          # binary cache is untrusted without --accept-flake-config, so it
          # would compile from source). `just update-bun-nix` fetches it
          # on demand instead.
          default = pkgs.mkShell {
            packages = with pkgs; [
              go
              bun
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
