{
  description = "Teldrive server binary";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs = { self, nixpkgs, ... }:
    let
      systems = [ "x86_64-linux" "aarch64-linux" ];
      forAllSystems = nixpkgs.lib.genAttrs systems;
      version = "dev";

      mkPkgs = system: import nixpkgs {
        inherit system;
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
          teldrive-bin = pkgs.callPackage ./nix/package-bin.nix { };
          default = teldrive;
        });

      overlays.default = final: prev: {
        teldrive-bin = final.callPackage ./nix/package-bin.nix { };
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
              goreleaser
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
