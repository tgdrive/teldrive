{
  description = "Teldrive development shell";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs = { nixpkgs, ... }:
    let
      systems = [ "x86_64-linux" "aarch64-linux" ];
      forAllSystems = nixpkgs.lib.genAttrs systems;
    in {
      packages = forAllSystems (system:
        let
          pkgs = import nixpkgs { inherit system; };
          playwrightLibs = with pkgs; [
            alsa-lib
            atk
            cairo
            cups
            dbus
            expat
            glib
            gtk3
            libdrm
            libgbm
            libxkbcommon
            mesa
            nspr
            nss
            pango
            systemd
            xorg.libX11
            xorg.libXcomposite
            xorg.libXdamage
            xorg.libXext
            xorg.libXfixes
            xorg.libXrandr
            xorg.libxcb
          ];
          uiTest = pkgs.writeShellApplication {
            name = "teldrive-ui-test";
            runtimeInputs = with pkgs; [ bun chromium ];
            text = ''
              export LD_LIBRARY_PATH="${pkgs.lib.makeLibraryPath playwrightLibs}''${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}"
              export PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH="${pkgs.chromium}/bin/chromium"
              export PLAYWRIGHT_REUSE_EXISTING_SERVER="false"
              exec bun run --cwd ui test "$@"
            '';
          };
        in {
          ui-test = uiTest;
          default = uiTest;
        });
    };
}
