# Home Manager module for Teldrive (user service). Apply the teldrive flake
# overlay so pkgs.teldrive exists, then:
#
#   services.teldrive = {
#     enable = true;
#     settings.database.url = "postgres://...";
#     settings.security.signing-key = "…";
#     settings.security.data-key = "…";
#   };
{ config, lib, pkgs, ... }:
let
  cfg = config.services.teldrive;
  toEnv = import ./to-env.nix { inherit lib; };
in
{
  options.services.teldrive = {
    enable = lib.mkEnableOption "Teldrive Telegram-backed cloud storage server (user service)";

    package = lib.mkOption {
      type = lib.types.package;
      default = pkgs.teldrive;
      defaultText = lib.literalExpression "pkgs.teldrive";
      description = "Teldrive package to run (requires the teldrive flake overlay).";
    };

    settings = lib.mkOption {
      type = lib.types.submodule {
        options = import ./generated-options.nix { inherit lib; };
      };
      default = { };
      description = "Teldrive settings, injected as TELDRIVE_ environment variables. Only explicitly set values are passed; the daemon applies its own defaults otherwise. See `just nix-generate` and internal/config for the full schema.";
    };

    environmentFile = lib.mkOption {
      type = lib.types.nullOr lib.types.path;
      default = null;
      example = "~/.secrets/teldrive.env";
      description = "File with TELDRIVE_ assignments for secrets such as database.url and security keys. Takes precedence over settings.";
    };
  };

  config = lib.mkIf cfg.enable {
    systemd.user.services.teldrive = {
      Unit = {
        Description = "Teldrive Telegram-backed cloud storage server";
        After = [ "network.target" ];
      };
      Install = {
        WantedBy = [ "default.target" ];
      };
      Service = {
        ExecStart = "${cfg.package}/bin/teldrive run";
        Restart = "on-failure";
        Environment = lib.mapAttrsToList (name: value: "${name}=${value}") (toEnv cfg.settings);
      }
      // lib.optionalAttrs (cfg.environmentFile != null) {
        EnvironmentFile = cfg.environmentFile;
      };
    };
  };
}
